package coupon

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

type CreateCouponRequest struct {
	Code          string       `json:"code" validate:"required,min=3,max=50"`
	DiscountType  DiscountType `json:"discount_type" validate:"required,oneof=fixed percentage"`
	DiscountValue float64      `json:"discount_value" validate:"required,gt=0"`
	MinPurchase   float64      `json:"min_purchase" validate:"gte=0"`
	MaxDiscount   float64      `json:"max_discount" validate:"gte=0"`
	UsageLimit    int          `json:"usage_limit" validate:"gte=0"`
	PerUserLimit  int          `json:"per_user_limit" validate:"gte=0"`
	StartDate     time.Time    `json:"start_date" validate:"required"`
	ExpiresAt     time.Time    `json:"expires_at" validate:"required,gtfield=StartDate"`
}

func (s *Service) Create(ctx context.Context, req CreateCouponRequest) (*Coupon, error) {
	perUserLimit := req.PerUserLimit
	if perUserLimit == 0 {
		perUserLimit = 1
	}
	c := &Coupon{
		ID:            uuid.New(),
		Code:          req.Code,
		DiscountType:  req.DiscountType,
		DiscountValue: req.DiscountValue,
		MinPurchase:   req.MinPurchase,
		MaxDiscount:   req.MaxDiscount,
		UsageLimit:    req.UsageLimit,
		PerUserLimit:  perUserLimit,
		StartDate:     req.StartDate,
		ExpiresAt:     req.ExpiresAt,
		IsActive:      true,
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("create coupon: %w", err)
	}
	return c, nil
}

// Validate checks whether a coupon can be applied and returns the discount amount.
// It implements order.CouponValidator.
func (s *Service) Validate(ctx context.Context, code string, userID uuid.UUID, subtotal float64) (float64, error) {
	c, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return 0, err
	}
	userUsage, err := s.repo.CountUserUsage(ctx, c.ID, userID)
	if err != nil {
		return 0, err
	}
	return Evaluate(c, subtotal, userUsage)
}

func (s *Service) ListActive(ctx context.Context) ([]Coupon, error) {
	return s.repo.ListActive(ctx)
}

// PreviewResult tells the client exactly why a coupon can or can't be
// applied right now, including the minimum purchase amount on failure —
// a real complaint about the app this backend is modeled on is coupon
// minimums only surfacing at the payment screen, after the customer has
// already built a cart around the wrong assumption.
type PreviewResult struct {
	Code        string  `json:"code"`
	Eligible    bool    `json:"eligible"`
	Discount    float64 `json:"discount"`
	MinPurchase float64 `json:"min_purchase"`
	Reason      string  `json:"reason,omitempty"`
}

func (s *Service) Preview(ctx context.Context, code string, userID uuid.UUID, subtotal float64) (*PreviewResult, error) {
	c, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	userUsage, err := s.repo.CountUserUsage(ctx, c.ID, userID)
	if err != nil {
		return nil, err
	}

	discount, evalErr := Evaluate(c, subtotal, userUsage)
	result := &PreviewResult{Code: c.Code, MinPurchase: c.MinPurchase}
	if evalErr != nil {
		result.Eligible = false
		result.Reason = evalErr.Error()
		return result, nil
	}
	result.Eligible = true
	result.Discount = discount
	return result, nil
}

// Redeem records a coupon usage after an order has been created successfully.
func (s *Service) Redeem(ctx context.Context, code string, userID, orderID uuid.UUID) error {
	c, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return err
	}
	if err := s.repo.CreateUsage(ctx, &Usage{
		ID:       uuid.New(),
		CouponID: c.ID,
		UserID:   userID,
		OrderID:  orderID,
	}); err != nil {
		return err
	}
	return s.repo.IncrementUsedCount(ctx, c.ID)
}
