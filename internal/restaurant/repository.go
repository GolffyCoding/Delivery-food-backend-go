package restaurant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var ErrNotFound = errors.New("restaurant not found")

type postgresRepository struct {
	db *bun.DB
}

func NewPostgresRepository(db *bun.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Create(ctx context.Context, rest *Restaurant) error {
	_, err := r.db.NewInsert().Model(rest).Exec(ctx)
	return err
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Restaurant, error) {
	rest := &Restaurant{}
	err := r.db.NewSelect().
		Model(rest).
		Where("id = ? AND deleted_at IS NULL", id).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get restaurant: %w", err)
	}
	return rest, nil
}

func (r *postgresRepository) Update(ctx context.Context, rest *Restaurant) error {
	rest.UpdatedAt = time.Now()
	_, err := r.db.NewUpdate().Model(rest).WherePK().Exec(ctx)
	return err
}

func (r *postgresRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.NewUpdate().
		Model((*Restaurant)(nil)).
		Set("deleted_at = NOW()").
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *postgresRepository) List(ctx context.Context, filter ListFilter) ([]Restaurant, int64, error) {
	var restaurants []Restaurant
	query := r.db.NewSelect().Model(&restaurants).Where("deleted_at IS NULL")

	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.Cuisine != "" {
		query = query.Where("? = ANY(cuisine_types)", filter.Cuisine)
	}
	if filter.MinRating > 0 {
		query = query.Where("rating >= ?", filter.MinRating)
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	orderCol := "created_at"
	if filter.SortBy == "rating" {
		orderCol = "rating"
	}
	dir := "DESC"
	if filter.SortDir == "asc" {
		dir = "ASC"
	}
	query = query.OrderExpr(orderCol + " " + dir)

	if filter.PerPage > 0 {
		offset := (filter.Page - 1) * filter.PerPage
		query = query.Limit(filter.PerPage).Offset(offset)
	}

	if err := query.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return restaurants, int64(total), nil
}

func (r *postgresRepository) ListByMerchant(ctx context.Context, merchantID uuid.UUID) ([]Restaurant, error) {
	var restaurants []Restaurant
	err := r.db.NewSelect().
		Model(&restaurants).
		Where("merchant_id = ? AND deleted_at IS NULL", merchantID).
		Order("created_at DESC").
		Scan(ctx)
	return restaurants, err
}

func (r *postgresRepository) ListFeatured(ctx context.Context) ([]Restaurant, error) {
	var restaurants []Restaurant
	err := r.db.NewSelect().
		Model(&restaurants).
		Where("is_featured = true AND status = ? AND deleted_at IS NULL", StatusActive).
		Order("rating DESC").
		Limit(20).
		Scan(ctx)
	return restaurants, err
}

// ListNearby uses the Haversine formula directly in SQL (no PostGIS dependency required).
func (r *postgresRepository) ListNearby(ctx context.Context, lat, lng, radiusKm float64, limit int) ([]Restaurant, error) {
	var restaurants []Restaurant
	distExpr := fmt.Sprintf(
		"(6371 * acos(LEAST(1, GREATEST(-1, cos(radians(%f)) * cos(radians(latitude)) * cos(radians(longitude) - radians(%f)) + sin(radians(%f)) * sin(radians(latitude))))))",
		lat, lng, lat,
	)

	err := r.db.NewSelect().
		Model(&restaurants).
		Where("deleted_at IS NULL AND status = ?", StatusActive).
		Where(distExpr+" <= ?", radiusKm).
		OrderExpr(distExpr + " ASC").
		Limit(limit).
		Scan(ctx)
	return restaurants, err
}

func (r *postgresRepository) Search(ctx context.Context, query string) ([]Restaurant, error) {
	var restaurants []Restaurant
	pattern := "%" + query + "%"
	err := r.db.NewSelect().
		Model(&restaurants).
		Where("deleted_at IS NULL AND status = ? AND (name ILIKE ? OR description ILIKE ?)",
			StatusActive, pattern, pattern).
		Order("rating DESC").
		Limit(20).
		Scan(ctx)
	return restaurants, err
}

func (r *postgresRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	return r.db.NewSelect().
		Model((*Restaurant)(nil)).
		Where("id = ? AND deleted_at IS NULL", id).
		Exists(ctx)
}

func (r *postgresRepository) IsOpen(ctx context.Context, id uuid.UUID) (bool, error) {
	rest, err := r.GetByID(ctx, id)
	if err != nil {
		return false, err
	}

	now := time.Now()
	dayOfWeek := int(now.Weekday())
	if dayOfWeek == 0 {
		dayOfWeek = 7
	}
	currentTime := now.Format("15:04")

	for _, oh := range rest.OpeningHours {
		if oh.DayOfWeek == dayOfWeek {
			if oh.IsClosed {
				return false, nil
			}
			return currentTime >= oh.OpenTime && currentTime <= oh.CloseTime, nil
		}
	}

	return false, nil
}
