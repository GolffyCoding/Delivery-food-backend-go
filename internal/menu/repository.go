package menu

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

func (r *postgresRepository) ListCategories(ctx context.Context, restaurantID uuid.UUID) ([]Category, error) {
	var categories []Category
	err := r.db.NewSelect().
		Model(&categories).
		Where("restaurant_id = ? AND is_active = true", restaurantID).
		Order("display_order ASC").
		Scan(ctx)
	return categories, err
}

func (r *postgresRepository) CreateCategory(ctx context.Context, cat *Category) error {
	_, err := r.db.NewInsert().Model(cat).Exec(ctx)
	return err
}

func (r *postgresRepository) UpdateCategory(ctx context.Context, cat *Category) error {
	cat.UpdatedAt = time.Now()
	_, err := r.db.NewUpdate().Model(cat).WherePK().Exec(ctx)
	return err
}

func (r *postgresRepository) DeleteCategory(ctx context.Context, id, restaurantID uuid.UUID) error {
	_, err := r.db.NewDelete().
		Model((*Category)(nil)).
		Where("id = ? AND restaurant_id = ?", id, restaurantID).
		Exec(ctx)
	return err
}

func (r *postgresRepository) ListItems(ctx context.Context, restaurantID uuid.UUID, activeOnly bool) ([]Item, error) {
	var items []Item
	query := r.db.NewSelect().
		Model(&items).
		Where("restaurant_id = ? AND deleted_at IS NULL", restaurantID)
	if activeOnly {
		query = query.Where("is_active = true")
	}
	err := query.Order("created_at DESC").Scan(ctx)
	return items, err
}

func (r *postgresRepository) GetItem(ctx context.Context, id uuid.UUID) (*Item, error) {
	item := &Item{}
	err := r.db.NewSelect().
		Model(item).
		Where("id = ? AND deleted_at IS NULL", id).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get menu item: %w", err)
	}
	return item, nil
}

func (r *postgresRepository) CreateItem(ctx context.Context, item *Item) error {
	_, err := r.db.NewInsert().Model(item).Exec(ctx)
	return err
}

func (r *postgresRepository) UpdateItem(ctx context.Context, item *Item) error {
	item.UpdatedAt = time.Now()
	_, err := r.db.NewUpdate().Model(item).WherePK().Exec(ctx)
	return err
}

func (r *postgresRepository) DeleteItem(ctx context.Context, id, restaurantID uuid.UUID) error {
	_, err := r.db.NewUpdate().
		Model((*Item)(nil)).
		Set("deleted_at = NOW()").
		Where("id = ? AND restaurant_id = ?", id, restaurantID).
		Exec(ctx)
	return err
}

func (r *postgresRepository) ListVariants(ctx context.Context, itemID uuid.UUID) ([]Variant, error) {
	var variants []Variant
	err := r.db.NewSelect().Model(&variants).Where("item_id = ?", itemID).Scan(ctx)
	return variants, err
}

func (r *postgresRepository) CreateVariant(ctx context.Context, v *Variant) error {
	_, err := r.db.NewInsert().Model(v).Exec(ctx)
	return err
}

func (r *postgresRepository) DeleteVariant(ctx context.Context, id, itemID uuid.UUID) error {
	_, err := r.db.NewDelete().
		Model((*Variant)(nil)).
		Where("id = ? AND item_id = ?", id, itemID).
		Exec(ctx)
	return err
}

func (r *postgresRepository) ListAddOns(ctx context.Context, itemID uuid.UUID) ([]AddOn, error) {
	var addons []AddOn
	err := r.db.NewSelect().Model(&addons).Where("item_id = ?", itemID).Scan(ctx)
	return addons, err
}

func (r *postgresRepository) CreateAddOn(ctx context.Context, a *AddOn) error {
	_, err := r.db.NewInsert().Model(a).Exec(ctx)
	return err
}

func (r *postgresRepository) DeleteAddOn(ctx context.Context, id, itemID uuid.UUID) error {
	_, err := r.db.NewDelete().
		Model((*AddOn)(nil)).
		Where("id = ? AND item_id = ?", id, itemID).
		Exec(ctx)
	return err
}
