package coupon

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type DiscountType string

const (
	DiscountFixed      DiscountType = "fixed"
	DiscountPercentage DiscountType = "percentage"
)

type Coupon struct {
	bun.BaseModel `bun:"table:coupons"`

	ID             uuid.UUID    `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	Code           string       `json:"code" bun:",unique,notnull"`
	DiscountType   DiscountType `json:"discount_type" bun:",notnull"`
	DiscountValue  float64      `json:"discount_value" bun:",notnull"`
	MinPurchase    float64      `json:"min_purchase"`
	MaxDiscount    float64      `json:"max_discount"`
	UsageLimit     int          `json:"usage_limit"`
	UsedCount      int          `json:"used_count"`
	PerUserLimit   int          `json:"per_user_limit" bun:",default:1"`
	StartDate      time.Time    `json:"start_date" bun:",notnull"`
	ExpiresAt      time.Time    `json:"expires_at" bun:",notnull"`
	IsActive       bool         `json:"is_active" bun:",default:true"`
	CreatedAt      time.Time    `json:"created_at" bun:",nullzero,default:now()"`
}

type Usage struct {
	bun.BaseModel `bun:"table:coupon_usages"`

	ID        uuid.UUID `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	CouponID  uuid.UUID `json:"coupon_id" bun:",type:uuid,notnull"`
	UserID    uuid.UUID `json:"user_id" bun:",type:uuid,notnull"`
	OrderID   uuid.UUID `json:"order_id" bun:",type:uuid,notnull"`
	CreatedAt time.Time `json:"created_at" bun:",nullzero,default:now()"`
}

type CouponError struct {
	Code    string
	Message string
}

func (e *CouponError) Error() string { return e.Message }

var (
	ErrCouponNotFound         = &CouponError{Code: "COUPON_NOT_FOUND", Message: "Coupon not found"}
	ErrCouponInactive         = &CouponError{Code: "COUPON_INACTIVE", Message: "Coupon is not active"}
	ErrCouponExpired          = &CouponError{Code: "COUPON_EXPIRED", Message: "Coupon has expired"}
	ErrCouponLimitReached     = &CouponError{Code: "COUPON_LIMIT_REACHED", Message: "Coupon usage limit reached"}
	ErrCouponUserLimitReached = &CouponError{Code: "COUPON_USER_LIMIT_REACHED", Message: "You have reached the usage limit for this coupon"}
	ErrCouponBelowMinPurchase = &CouponError{Code: "COUPON_BELOW_MIN_PURCHASE", Message: "Order total is below the minimum purchase amount"}
)

type Repository interface {
	Create(ctx context.Context, c *Coupon) error
	FindByCode(ctx context.Context, code string) (*Coupon, error)
	IncrementUsedCount(ctx context.Context, id uuid.UUID) error
	CountUserUsage(ctx context.Context, couponID, userID uuid.UUID) (int, error)
	CreateUsage(ctx context.Context, usage *Usage) error
	// ListActive returns coupons that are currently redeemable in principle
	// (active flag set, within their date window, under their global usage
	// cap) so customers can browse what's available before checkout instead
	// of only discovering terms by pasting a code in and failing.
	ListActive(ctx context.Context) ([]Coupon, error)
}

// Evaluate applies the coupon's business rules and returns the discount amount to apply.
func Evaluate(c *Coupon, subtotal float64, userUsageCount int) (float64, error) {
	now := time.Now()
	if !c.IsActive {
		return 0, ErrCouponInactive
	}
	if now.Before(c.StartDate) || now.After(c.ExpiresAt) {
		return 0, ErrCouponExpired
	}
	if c.UsageLimit > 0 && c.UsedCount >= c.UsageLimit {
		return 0, ErrCouponLimitReached
	}
	if c.PerUserLimit > 0 && userUsageCount >= c.PerUserLimit {
		return 0, ErrCouponUserLimitReached
	}
	if subtotal < c.MinPurchase {
		return 0, ErrCouponBelowMinPurchase
	}

	var discount float64
	switch c.DiscountType {
	case DiscountFixed:
		discount = c.DiscountValue
	case DiscountPercentage:
		discount = subtotal * (c.DiscountValue / 100)
		if c.MaxDiscount > 0 && discount > c.MaxDiscount {
			discount = c.MaxDiscount
		}
	}

	if discount > subtotal {
		discount = subtotal
	}
	return discount, nil
}
