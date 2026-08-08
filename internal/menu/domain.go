package menu

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var ErrNotFound = errors.New("menu item not found")

type Category struct {
	bun.BaseModel `bun:"table:menu_categories"`

	ID           uuid.UUID `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	RestaurantID uuid.UUID `json:"restaurant_id" bun:",type:uuid,notnull"`
	Name         string    `json:"name" bun:",notnull"`
	DisplayOrder int       `json:"display_order"`
	IsActive     bool      `json:"is_active" bun:",default:true"`
	CreatedAt    time.Time `json:"created_at" bun:",nullzero,default:now()"`
	UpdatedAt    time.Time `json:"updated_at" bun:",nullzero,default:now()"`
}

type Item struct {
	bun.BaseModel `bun:"table:menu_items"`

	ID           uuid.UUID  `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	RestaurantID uuid.UUID  `json:"restaurant_id" bun:",type:uuid,notnull"`
	CategoryID   *uuid.UUID `json:"category_id" bun:",type:uuid"`
	Name         string     `json:"name" bun:",notnull"`
	Description  string     `json:"description"`
	BasePrice    float64    `json:"base_price" bun:",notnull"`
	ImageURL     string     `json:"image_url"`
	IsActive     bool       `json:"is_active" bun:",default:true"`
	PrepTimeMin  int        `json:"prep_time_min" bun:",default:10"`
	PrepTimeMax  int        `json:"prep_time_max" bun:",default:20"`
	CreatedAt    time.Time  `json:"created_at" bun:",nullzero,default:now()"`
	UpdatedAt    time.Time  `json:"updated_at" bun:",nullzero,default:now()"`
	DeletedAt    *time.Time `json:"-" bun:",soft_delete"`
}

type Variant struct {
	bun.BaseModel `bun:"table:menu_item_variants"`

	ID     uuid.UUID `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	ItemID uuid.UUID `json:"item_id" bun:",type:uuid,notnull"`
	Name   string    `json:"name" bun:",notnull"`
	Price  float64   `json:"price" bun:",notnull"`
}

type AddOn struct {
	bun.BaseModel `bun:"table:menu_item_addons"`

	ID          uuid.UUID `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	ItemID      uuid.UUID `json:"item_id" bun:",type:uuid,notnull"`
	Name        string    `json:"name" bun:",notnull"`
	Price       float64   `json:"price" bun:",notnull"`
	MaxQuantity int       `json:"max_quantity" bun:",default:5"`
}

type Repository interface {
	ListCategories(ctx context.Context, restaurantID uuid.UUID) ([]Category, error)
	CreateCategory(ctx context.Context, cat *Category) error
	UpdateCategory(ctx context.Context, cat *Category) error
	DeleteCategory(ctx context.Context, id, restaurantID uuid.UUID) error

	ListItems(ctx context.Context, restaurantID uuid.UUID, activeOnly bool) ([]Item, error)
	GetItem(ctx context.Context, id uuid.UUID) (*Item, error)
	CreateItem(ctx context.Context, item *Item) error
	UpdateItem(ctx context.Context, item *Item) error
	DeleteItem(ctx context.Context, id, restaurantID uuid.UUID) error

	ListVariants(ctx context.Context, itemID uuid.UUID) ([]Variant, error)
	CreateVariant(ctx context.Context, v *Variant) error
	DeleteVariant(ctx context.Context, id, itemID uuid.UUID) error

	ListAddOns(ctx context.Context, itemID uuid.UUID) ([]AddOn, error)
	CreateAddOn(ctx context.Context, a *AddOn) error
	DeleteAddOn(ctx context.Context, id, itemID uuid.UUID) error
}
