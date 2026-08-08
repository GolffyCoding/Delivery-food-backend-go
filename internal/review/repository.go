package review

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var ErrNotFound = errors.New("review not found")

type postgresRepository struct {
	db *bun.DB
}

func NewPostgresRepository(db *bun.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Create(ctx context.Context, rv *Review) error {
	_, err := r.db.NewInsert().Model(rv).Exec(ctx)
	return err
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Review, error) {
	rv := &Review{}
	err := r.db.NewSelect().Model(rv).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get review: %w", err)
	}
	return rv, nil
}

func (r *postgresRepository) ListByRestaurant(ctx context.Context, restaurantID uuid.UUID, page, perPage int) ([]Review, int64, error) {
	var reviews []Review
	query := r.db.NewSelect().Model(&reviews).Where("restaurant_id = ?", restaurantID)

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Order("created_at DESC").Limit(perPage).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, err
	}
	return reviews, int64(total), nil
}

func (r *postgresRepository) GetRestaurantSummary(ctx context.Context, restaurantID uuid.UUID) (*RatingSummary, error) {
	var reviews []Review
	if err := r.db.NewSelect().Model(&reviews).Where("restaurant_id = ?", restaurantID).Scan(ctx); err != nil {
		return nil, err
	}

	summary := &RatingSummary{Distribution: map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}}
	var sum int
	for _, rv := range reviews {
		sum += rv.Rating
		summary.Distribution[rv.Rating]++
	}
	summary.Count = len(reviews)
	if summary.Count > 0 {
		summary.Average = float64(sum) / float64(summary.Count)
	}
	return summary, nil
}

func (r *postgresRepository) HasReviewed(ctx context.Context, orderID uuid.UUID) (bool, error) {
	return r.db.NewSelect().Model((*Review)(nil)).Where("order_id = ?", orderID).Exists(ctx)
}

func (r *postgresRepository) AddReply(ctx context.Context, reviewID, restaurantID uuid.UUID, reply string) error {
	_, err := r.db.NewUpdate().
		Model((*Review)(nil)).
		Set("restaurant_reply = ?", reply).
		Set("updated_at = NOW()").
		Where("id = ? AND restaurant_id = ?", reviewID, restaurantID).
		Exec(ctx)
	return err
}
