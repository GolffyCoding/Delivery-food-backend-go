package payment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func (r *postgresRepository) Create(ctx context.Context, p *Payment) error {
	_, err := r.db.NewInsert().Model(p).Exec(ctx)
	return err
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Payment, error) {
	p := &Payment{}
	err := r.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("get payment: %w", err)
	}
	return p, nil
}

func (r *postgresRepository) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*Payment, error) {
	p := &Payment{}
	err := r.db.NewSelect().Model(p).Where("order_id = ?", orderID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("get payment by order: %w", err)
	}
	return p, nil
}

func (r *postgresRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status Status) error {
	_, err := r.db.NewUpdate().
		Model((*Payment)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}
