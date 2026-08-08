package driver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type postgresRepository struct {
	db *bun.DB
}

func NewPostgresRepository(db *bun.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Create(ctx context.Context, d *Driver) error {
	_, err := r.db.NewInsert().Model(d).Exec(ctx)
	return err
}

func (r *postgresRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*Driver, error) {
	d := &Driver{}
	err := r.db.NewSelect().Model(d).Where("user_id = ?", userID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get driver by user id: %w", err)
	}
	return d, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Driver, error) {
	d := &Driver{}
	err := r.db.NewSelect().Model(d).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get driver: %w", err)
	}
	return d, nil
}

func (r *postgresRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status Status) error {
	_, err := r.db.NewUpdate().
		Model((*Driver)(nil)).
		Set("status = ?", status).
		Set("updated_at = NOW()").
		Where("id = ? AND is_active = true", id).
		Exec(ctx)
	return err
}

func (r *postgresRepository) UpdateLocation(ctx context.Context, id uuid.UUID, lat, lng float64) error {
	_, err := r.db.NewUpdate().
		Model((*Driver)(nil)).
		Set("current_lat = ?", lat).
		Set("current_lng = ?", lng).
		Set("last_location_at = NOW()").
		Set("updated_at = NOW()").
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *postgresRepository) ListOnline(ctx context.Context) ([]Driver, error) {
	var drivers []Driver
	err := r.db.NewSelect().
		Model(&drivers).
		Where("status = ? AND is_active = true", StatusOnline).
		Scan(ctx)
	return drivers, err
}

func (r *postgresRepository) GetEarnings(ctx context.Context, driverID uuid.UUID) ([]Earning, float64, error) {
	var earnings []Earning
	if err := r.db.NewSelect().Model(&earnings).Where("driver_id = ?", driverID).Order("created_at DESC").Scan(ctx); err != nil {
		return nil, 0, err
	}
	var total float64
	for _, e := range earnings {
		total += e.Amount
	}
	return earnings, total, nil
}

func (r *postgresRepository) AddEarning(ctx context.Context, e *Earning) error {
	_, err := r.db.NewInsert().Model(e).Exec(ctx)
	return err
}
