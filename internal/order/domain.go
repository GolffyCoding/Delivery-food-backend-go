package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusAccepted   Status = "accepted"
	StatusPreparing  Status = "preparing"
	StatusReady      Status = "ready"
	StatusPickedUp   Status = "picked_up"
	StatusDelivering Status = "delivering"
	StatusDelivered  Status = "delivered"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"
)

type AddOnItem struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Qty   int     `json:"qty"`
}

type OrderItem struct {
	bun.BaseModel `bun:"table:order_items"`

	ID           uuid.UUID   `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	OrderID      uuid.UUID   `json:"order_id" bun:",type:uuid,notnull"`
	MenuItemID   uuid.UUID   `json:"menu_item_id" bun:",type:uuid,notnull"`
	Name         string      `json:"name" bun:",notnull"`
	Quantity     int         `json:"quantity" bun:",notnull"`
	UnitPrice    float64     `json:"unit_price" bun:",notnull"`
	VariantName  string      `json:"variant_name"`
	VariantPrice float64     `json:"variant_price"`
	AddOns       []AddOnItem `json:"addons" bun:"addons,type:jsonb"`
	TotalPrice   float64     `json:"total_price" bun:",notnull"`
	SpecialNotes string      `json:"special_notes"`
}

type Order struct {
	bun.BaseModel `bun:"table:orders"`

	ID                uuid.UUID   `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	OrderNumber       string      `json:"order_number" bun:",unique,notnull"`
	CustomerID        uuid.UUID   `json:"customer_id" bun:",type:uuid,notnull"`
	RestaurantID      uuid.UUID   `json:"restaurant_id" bun:",type:uuid,notnull"`
	DriverID          *uuid.UUID  `json:"driver_id" bun:",type:uuid"`
	Status            Status      `json:"status" bun:",notnull,default:'pending'"`
	Items             []OrderItem `json:"items" bun:"-"`
	Subtotal          float64     `json:"subtotal" bun:",notnull"`
	TaxAmount         float64     `json:"tax_amount" bun:",notnull"`
	DeliveryFee       float64     `json:"delivery_fee" bun:",notnull"`
	DiscountAmount    float64     `json:"discount_amount"`
	CouponCode        string      `json:"coupon_code"`
	TotalAmount       float64     `json:"total_amount" bun:",notnull"`
	PaymentMethod     string      `json:"payment_method" bun:",notnull,default:'cash'"`
	PaymentStatus     string      `json:"payment_status" bun:",default:'pending'"`
	DeliveryAddress   string      `json:"delivery_address" bun:",notnull"`
	DeliveryLat       float64     `json:"delivery_lat"`
	DeliveryLng       float64     `json:"delivery_lng"`
	SpecialNotes      string      `json:"special_notes"`
	EstimatedDelivery *time.Time  `json:"estimated_delivery"`
	AcceptedAt        *time.Time  `json:"accepted_at"`
	PreparedAt        *time.Time  `json:"prepared_at"`
	PickedUpAt        *time.Time  `json:"picked_up_at"`
	DeliveredAt       *time.Time  `json:"delivered_at"`
	CompletedAt       *time.Time  `json:"completed_at"`
	CancelledAt       *time.Time  `json:"cancelled_at"`
	CancelReason      string      `json:"cancel_reason"`
	// PickupCode is a short code the driver must present to the restaurant (or read
	// off the packed order) and submit when claiming the delivery. Reviews describe
	// riders grabbing whichever order looks like the best payout without checking it
	// matches their assignment, causing mixed-up handoffs; requiring this code forces
	// a physical match check instead of trusting the driver's word.
	PickupCode string `json:"pickup_code" bun:",notnull"`
	// DeliveryPhotoURL is the proof-of-delivery photo the driver submits when marking
	// an order delivered, giving the customer/restaurant something concrete to check
	// against a "food arrived cold/spilled/never arrived" complaint instead of it
	// being one party's word against another's.
	DeliveryPhotoURL string     `json:"delivery_photo_url"`
	Version          int        `json:"version" bun:",notnull,default:1"`
	CreatedAt        time.Time  `json:"created_at" bun:",nullzero,default:now()"`
	UpdatedAt        time.Time  `json:"updated_at" bun:",nullzero,default:now()"`
	DeletedAt        *time.Time `json:"-" bun:",soft_delete"`
}

// validTransitions encodes the order state machine.
var validTransitions = map[Status][]Status{
	StatusPending:    {StatusAccepted, StatusCancelled},
	StatusAccepted:   {StatusPreparing, StatusCancelled},
	StatusPreparing:  {StatusReady, StatusCancelled},
	StatusReady:      {StatusPickedUp},
	StatusPickedUp:   {StatusDelivering},
	StatusDelivering: {StatusDelivered},
	StatusDelivered:  {StatusCompleted},
	StatusCompleted:  {},
	StatusCancelled:  {},
}

func (s Status) CanTransitionTo(target Status) bool {
	allowed, ok := validTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == target {
			return true
		}
	}
	return false
}

func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusCancelled
}

func ValidateTransition(current, target Status) error {
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: cannot go from %s to %s", ErrInvalidTransition, current, target)
	}
	return nil
}

var (
	ErrOrderNotFound      = errors.New("order not found")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrRestaurantClosed   = errors.New("restaurant is closed or unavailable")
	ErrBelowMinimumOrder  = errors.New("order total below minimum order amount")
	ErrEmptyOrder         = errors.New("order must have at least one item")
	ErrOptimisticLock     = errors.New("order was modified by another process")
	ErrNotYourOrder       = errors.New("not your order")
	ErrInvalidCoupon      = errors.New("coupon code is invalid or cannot be applied")
	ErrPickupCodeMismatch = errors.New("pickup code does not match this order")
)

// TxHook runs inside the same DB transaction as Order creation, used to append
// an outbox message atomically with the order insert (transactional outbox pattern).
type TxHook func(ctx context.Context, tx bun.Tx) error

type Repository interface {
	Create(ctx context.Context, order *Order) error
	CreateWithHook(ctx context.Context, order *Order, hook TxHook) error
	GetByID(ctx context.Context, id uuid.UUID) (*Order, error)
	GetByOrderNumber(ctx context.Context, number string) (*Order, error)
	UpdateWithOptimisticLock(ctx context.Context, order *Order) error

	ListByCustomer(ctx context.Context, customerID uuid.UUID, page, perPage int) ([]Order, int64, error)
	ListByRestaurant(ctx context.Context, restaurantID uuid.UUID, status *Status, page, perPage int) ([]Order, int64, error)
	ListByDriver(ctx context.Context, driverID uuid.UUID, page, perPage int) ([]Order, int64, error)

	GetPendingOrders(ctx context.Context, restaurantID uuid.UUID) ([]Order, error)
	CountByStatus(ctx context.Context, restaurantID uuid.UUID) (map[Status]int64, error)

	// GetStalePending returns pending orders created before the cutoff, used by
	// the background worker to auto-cancel orders no restaurant ever accepted.
	GetStalePending(ctx context.Context, cutoff time.Time) ([]Order, error)

	// GetStaleReady returns orders that have been ready-for-pickup since before the
	// cutoff but still have no driver assigned, used by the background worker to
	// re-broadcast the delivery and alert the customer/restaurant of the delay.
	GetStaleReady(ctx context.Context, cutoff time.Time) ([]Order, error)

	// GetSalesReport aggregates completed-order revenue for a restaurant over
	// [from, to), used by the merchant sales report endpoint.
	GetSalesReport(ctx context.Context, restaurantID uuid.UUID, from, to time.Time) (*SalesReport, error)
}

type DailyRevenue struct {
	Date    string  `json:"date" bun:"date"`
	Orders  int64   `json:"orders" bun:"orders"`
	Revenue float64 `json:"revenue" bun:"revenue"`
}

type ItemSales struct {
	MenuItemID uuid.UUID `json:"menu_item_id" bun:"menu_item_id"`
	Name       string    `json:"name" bun:"name"`
	Quantity   int64     `json:"quantity" bun:"quantity"`
	Revenue    float64   `json:"revenue" bun:"revenue"`
}

type SalesReport struct {
	From              time.Time      `json:"from"`
	To                time.Time      `json:"to"`
	TotalOrders       int64          `json:"total_orders"`
	CompletedOrders   int64          `json:"completed_orders"`
	CancelledOrders   int64          `json:"cancelled_orders"`
	TotalRevenue      float64        `json:"total_revenue"`
	AverageOrderValue float64        `json:"average_order_value"`
	RevenueByDay      []DailyRevenue `json:"revenue_by_day"`
	TopItems          []ItemSales    `json:"top_items"`
}
