package coupon

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

func (r *postgresRepository) Create(ctx context.Context, c *Coupon) error {
	_, err := r.db.NewInsert().Model(c).Exec(ctx)
	return err
}

func (r *postgresRepository) FindByCode(ctx context.Context, code string) (*Coupon, error) {
	c := &Coupon{}
	err := r.db.NewSelect().Model(c).Where("code = ?", code).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCouponNotFound
		}
		return nil, fmt.Errorf("find coupon: %w", err)
	}
	return c, nil
}

func (r *postgresRepository) IncrementUsedCount(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.NewUpdate().
		Model((*Coupon)(nil)).
		Set("used_count = used_count + 1").
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *postgresRepository) CountUserUsage(ctx context.Context, couponID, userID uuid.UUID) (int, error) {
	count, err := r.db.NewSelect().
		Model((*Usage)(nil)).
		Where("coupon_id = ? AND user_id = ?", couponID, userID).
		Count(ctx)
	return count, err
}

func (r *postgresRepository) CreateUsage(ctx context.Context, usage *Usage) error {
	_, err := r.db.NewInsert().Model(usage).Exec(ctx)
	return err
}

func (r *postgresRepository) ListActive(ctx context.Context) ([]Coupon, error) {
	var coupons []Coupon
	err := r.db.NewSelect().
		Model(&coupons).
		Where("is_active = ?", true).
		Where("start_date <= NOW() AND expires_at >= NOW()").
		Where("usage_limit = 0 OR used_count < usage_limit").
		Order("expires_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active coupons: %w", err)
	}
	return coupons, nil
}
