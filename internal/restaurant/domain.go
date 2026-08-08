package restaurant

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusInactive  Status = "inactive"
	StatusSuspended Status = "suspended"
)

type OpeningHours struct {
	DayOfWeek int    `json:"day_of_week"`
	OpenTime  string `json:"open_time"`
	CloseTime string `json:"close_time"`
	IsClosed  bool   `json:"is_closed"`
}

// OpeningHoursList implements sql.Scanner/driver.Valuer so it can be stored as JSONB.
type OpeningHoursList []OpeningHours

type Restaurant struct {
	bun.BaseModel `bun:"table:restaurants"`

	ID               uuid.UUID        `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	MerchantID       uuid.UUID        `json:"merchant_id" bun:",type:uuid,notnull"`
	Name             string           `json:"name" bun:",notnull"`
	Description      string           `json:"description"`
	CoverImageURL    string           `json:"cover_image_url"`
	LogoURL          string           `json:"logo_url"`
	Phone            string           `json:"phone" bun:",notnull"`
	Address          string           `json:"address" bun:",notnull"`
	Latitude         float64          `json:"latitude"`
	Longitude        float64          `json:"longitude"`
	DeliveryRadiusKm float64          `json:"delivery_radius_km" bun:",default:5.0"`
	MinimumOrder     float64          `json:"minimum_order"`
	DeliveryFee      float64          `json:"delivery_fee"`
	Rating           float64          `json:"rating"`
	RatingCount      int              `json:"rating_count"`
	Status           Status           `json:"status" bun:",default:'active'"`
	OpeningHours     OpeningHoursList `json:"opening_hours" bun:"type:jsonb"`
	CuisineTypes     []string         `json:"cuisine_types" bun:"type:text[],array"`
	IsFeatured       bool             `json:"is_featured"`
	CreatedAt        time.Time        `json:"created_at" bun:",nullzero,default:now()"`
	UpdatedAt        time.Time        `json:"updated_at" bun:",nullzero,default:now()"`
	DeletedAt        *time.Time       `json:"-" bun:",soft_delete"`
}

type Repository interface {
	Create(ctx context.Context, r *Restaurant) error
	GetByID(ctx context.Context, id uuid.UUID) (*Restaurant, error)
	Update(ctx context.Context, r *Restaurant) error
	SoftDelete(ctx context.Context, id uuid.UUID) error

	List(ctx context.Context, filter ListFilter) ([]Restaurant, int64, error)
	ListByMerchant(ctx context.Context, merchantID uuid.UUID) ([]Restaurant, error)
	ListFeatured(ctx context.Context) ([]Restaurant, error)
	ListNearby(ctx context.Context, lat, lng, radiusKm float64, limit int) ([]Restaurant, error)
	Search(ctx context.Context, query string) ([]Restaurant, error)

	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
	IsOpen(ctx context.Context, id uuid.UUID) (bool, error)
}

type ListFilter struct {
	Page      int
	PerPage   int
	Status    *Status
	Cuisine   string
	MinRating float64
	SortBy    string
	SortDir   string
}
