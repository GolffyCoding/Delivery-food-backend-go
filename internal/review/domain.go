package review

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Review struct {
	bun.BaseModel `bun:"table:reviews"`

	ID              uuid.UUID  `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	UserID          uuid.UUID  `json:"user_id" bun:",type:uuid,notnull"`
	OrderID         uuid.UUID  `json:"order_id" bun:",type:uuid,unique,notnull"`
	RestaurantID    uuid.UUID  `json:"restaurant_id" bun:",type:uuid,notnull"`
	DriverID        *uuid.UUID `json:"driver_id" bun:",type:uuid"`
	Rating          int        `json:"rating" bun:",notnull"`
	Comment         string     `json:"comment"`
	PhotoURLs       []string   `json:"photo_urls" bun:"photo_urls,type:text[],array"`
	RestaurantReply string     `json:"restaurant_reply"`
	CreatedAt       time.Time  `json:"created_at" bun:",nullzero,default:now()"`
	UpdatedAt       time.Time  `json:"updated_at" bun:",nullzero,default:now()"`
}

type RatingSummary struct {
	Average      float64     `json:"average"`
	Count        int         `json:"count"`
	Distribution map[int]int `json:"distribution"`
}

type Repository interface {
	Create(ctx context.Context, r *Review) error
	GetByID(ctx context.Context, id uuid.UUID) (*Review, error)
	ListByRestaurant(ctx context.Context, restaurantID uuid.UUID, page, perPage int) ([]Review, int64, error)
	GetRestaurantSummary(ctx context.Context, restaurantID uuid.UUID) (*RatingSummary, error)
	HasReviewed(ctx context.Context, orderID uuid.UUID) (bool, error)
	AddReply(ctx context.Context, reviewID, restaurantID uuid.UUID, reply string) error
}
