package support

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusResolved   Status = "resolved"
	StatusClosed     Status = "closed"
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

type Category string

const (
	CategoryMissingFood    Category = "missing_food"
	CategoryWrongOrder     Category = "wrong_order"
	CategoryDamagedFood    Category = "damaged_food"
	CategoryLateDelivery   Category = "late_delivery"
	CategoryPayment        Category = "payment"
	CategoryDriverBehavior Category = "driver_behavior"
	CategoryOther          Category = "other"
)

// Ticket represents a customer support request. When it is filed against an order,
// OrderSnapshot is captured at creation time so the assigned agent immediately has the
// order's status/restaurant/driver/total context without cross-referencing another
// screen or asking the customer to repeat themselves — the single most-repeated
// complaint in the reviews was agents who clearly had no context and made the customer
// re-explain everything.
type Ticket struct {
	bun.BaseModel `bun:"table:support_tickets"`

	ID              uuid.UUID              `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	UserID          uuid.UUID              `json:"user_id" bun:",type:uuid,notnull"`
	OrderID         *uuid.UUID             `json:"order_id" bun:",type:uuid"`
	OrderSnapshot   map[string]interface{} `json:"order_snapshot" bun:"type:jsonb"`
	Category        Category               `json:"category" bun:",notnull"`
	Subject         string                 `json:"subject" bun:",notnull"`
	Description     string                 `json:"description" bun:",notnull"`
	Status          Status                 `json:"status" bun:",notnull,default:'open'"`
	Priority        Priority               `json:"priority" bun:",notnull,default:'normal'"`
	AssignedAdminID *uuid.UUID             `json:"assigned_admin_id" bun:",type:uuid"`
	Resolution      string                 `json:"resolution"`
	CreatedAt       time.Time              `json:"created_at" bun:",nullzero,default:now()"`
	UpdatedAt       time.Time              `json:"updated_at" bun:",nullzero,default:now()"`
	ResolvedAt      *time.Time             `json:"resolved_at"`
}

var (
	ErrTicketNotFound  = errors.New("support ticket not found")
	ErrNotYourTicket   = errors.New("not your support ticket")
	ErrAlreadyResolved = errors.New("ticket is already resolved or closed")
)

type Repository interface {
	Create(ctx context.Context, t *Ticket) error
	GetByID(ctx context.Context, id uuid.UUID) (*Ticket, error)
	Update(ctx context.Context, t *Ticket) error
	ListByUser(ctx context.Context, userID uuid.UUID, page, perPage int) ([]Ticket, int64, error)
	// ListOpen returns tickets an admin queue should work through: open or
	// in_progress, oldest (and highest priority) first.
	ListOpen(ctx context.Context, page, perPage int) ([]Ticket, int64, error)
}
