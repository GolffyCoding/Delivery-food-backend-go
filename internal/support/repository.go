package support

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

func (r *postgresRepository) Create(ctx context.Context, t *Ticket) error {
	_, err := r.db.NewInsert().Model(t).Exec(ctx)
	return err
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Ticket, error) {
	t := &Ticket{}
	err := r.db.NewSelect().Model(t).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTicketNotFound
		}
		return nil, fmt.Errorf("get ticket: %w", err)
	}
	return t, nil
}

func (r *postgresRepository) Update(ctx context.Context, t *Ticket) error {
	_, err := r.db.NewUpdate().
		Model(t).
		Column("status", "priority", "assigned_admin_id", "resolution", "resolved_at", "updated_at").
		Where("id = ?", t.ID).
		Exec(ctx)
	return err
}

func (r *postgresRepository) ListByUser(ctx context.Context, userID uuid.UUID, page, perPage int) ([]Ticket, int64, error) {
	var tickets []Ticket
	query := r.db.NewSelect().Model(&tickets).Where("user_id = ?", userID)

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Order("created_at DESC").Limit(perPage).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, err
	}
	return tickets, int64(total), nil
}

func (r *postgresRepository) ListOpen(ctx context.Context, page, perPage int) ([]Ticket, int64, error) {
	var tickets []Ticket
	query := r.db.NewSelect().Model(&tickets).Where("status IN (?)", bun.In([]Status{StatusOpen, StatusInProgress}))

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.
		OrderExpr("CASE priority WHEN 'high' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END ASC").
		Order("created_at ASC").
		Limit(perPage).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, err
	}
	return tickets, int64(total), nil
}
