package notification

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type postgresRepository struct {
	db *bun.DB
}

func NewPostgresRepository(db *bun.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Create(ctx context.Context, n *Notification) error {
	_, err := r.db.NewInsert().Model(n).Exec(ctx)
	return err
}

func (r *postgresRepository) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	_, err := r.db.NewUpdate().
		Model((*Notification)(nil)).
		Set("read = true").
		Where("id = ? AND user_id = ?", id, userID).
		Exec(ctx)
	return err
}

func (r *postgresRepository) ListByUser(ctx context.Context, userID uuid.UUID, page, perPage int) ([]Notification, int64, error) {
	var items []Notification
	query := r.db.NewSelect().Model(&items).Where("user_id = ?", userID)

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Order("created_at DESC").Limit(perPage).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, err
	}
	return items, int64(total), nil
}

func (r *postgresRepository) GetUnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	count, err := r.db.NewSelect().
		Model((*Notification)(nil)).
		Where("user_id = ? AND read = false", userID).
		Count(ctx)
	return int64(count), err
}

func (r *postgresRepository) MarkSent(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	_, err := r.db.NewUpdate().
		Model((*Notification)(nil)).
		Set("sent_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *postgresRepository) IncrementRetry(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.NewUpdate().
		Model((*Notification)(nil)).
		Set("retry_count = retry_count + 1").
		Where("id = ?", id).
		Exec(ctx)
	return err
}
