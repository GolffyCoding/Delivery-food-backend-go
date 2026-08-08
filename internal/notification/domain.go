package notification

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Type string

const (
	TypePush  Type = "push"
	TypeEmail Type = "email"
	TypeSMS   Type = "sms"
	TypeInApp Type = "in_app"
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

type Notification struct {
	bun.BaseModel `bun:"table:notifications"`

	ID          uuid.UUID              `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	UserID      uuid.UUID              `json:"user_id" bun:",type:uuid,notnull"`
	Type        Type                   `json:"type" bun:",notnull"`
	Title       string                 `json:"title" bun:",notnull"`
	Body        string                 `json:"body" bun:",notnull"`
	Data        map[string]interface{} `json:"data" bun:"type:jsonb"`
	Priority    Priority               `json:"priority" bun:",default:'normal'"`
	Read        bool                   `json:"read"`
	RetryCount  int                    `json:"retry_count"`
	MaxRetries  int                    `json:"max_retries" bun:",default:3"`
	SentAt      *time.Time             `json:"sent_at"`
	CreatedAt   time.Time              `json:"created_at" bun:",nullzero,default:now()"`
}

// Sender delivers a notification through a specific channel (push, email, sms, ...).
type Sender interface {
	Send(ctx context.Context, n *Notification) error
	CanHandle(t Type) bool
}

type Repository interface {
	Create(ctx context.Context, n *Notification) error
	MarkRead(ctx context.Context, id, userID uuid.UUID) error
	ListByUser(ctx context.Context, userID uuid.UUID, page, perPage int) ([]Notification, int64, error)
	GetUnreadCount(ctx context.Context, userID uuid.UUID) (int64, error)
	MarkSent(ctx context.Context, id uuid.UUID) error
	IncrementRetry(ctx context.Context, id uuid.UUID) error
}
