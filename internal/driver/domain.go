package driver

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Status string

const (
	StatusOnline  Status = "online"
	StatusOffline Status = "offline"
	StatusBusy    Status = "busy"
)

type VehicleType string

const (
	VehicleMotorcycle VehicleType = "motorcycle"
	VehicleCar        VehicleType = "car"
	VehicleBicycle    VehicleType = "bicycle"
)

type Driver struct {
	bun.BaseModel `bun:"table:drivers"`

	ID              uuid.UUID   `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	UserID          uuid.UUID   `json:"user_id" bun:",type:uuid,unique,notnull"`
	VehicleType     VehicleType `json:"vehicle_type" bun:",notnull"`
	LicensePlate    string      `json:"license_plate"`
	VehicleColor    string      `json:"vehicle_color"`
	Status          Status      `json:"status" bun:",default:'offline'"`
	CurrentLat      float64     `json:"current_lat"`
	CurrentLng      float64     `json:"current_lng"`
	LastLocationAt  *time.Time  `json:"last_location_at"`
	Rating          float64     `json:"rating" bun:",default:5.0"`
	TotalDeliveries int         `json:"total_deliveries"`
	TotalEarnings   float64     `json:"total_earnings"`
	IsActive        bool        `json:"is_active" bun:",default:true"`
	CreatedAt       time.Time   `json:"created_at" bun:",nullzero,default:now()"`
	UpdatedAt       time.Time   `json:"updated_at" bun:",nullzero,default:now()"`
}

type Earning struct {
	bun.BaseModel `bun:"table:driver_earnings"`

	ID        uuid.UUID `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	DriverID  uuid.UUID `json:"driver_id" bun:",type:uuid,notnull"`
	OrderID   uuid.UUID `json:"order_id" bun:",type:uuid,notnull"`
	Amount    float64   `json:"amount" bun:",notnull"`
	Type      string    `json:"type" bun:",notnull"`
	CreatedAt time.Time `json:"created_at" bun:",nullzero,default:now()"`
}

var ErrNotFound = errors.New("driver not found")

type Repository interface {
	Create(ctx context.Context, d *Driver) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*Driver, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Driver, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status Status) error
	UpdateLocation(ctx context.Context, id uuid.UUID, lat, lng float64) error
	ListOnline(ctx context.Context) ([]Driver, error)
	GetEarnings(ctx context.Context, driverID uuid.UUID) ([]Earning, float64, error)
	AddEarning(ctx context.Context, e *Earning) error
}
