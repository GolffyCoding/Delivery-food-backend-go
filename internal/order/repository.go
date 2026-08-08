package order

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

func (r *postgresRepository) Create(ctx context.Context, order *Order) error {
	return r.CreateWithHook(ctx, order, nil)
}

func (r *postgresRepository) CreateWithHook(ctx context.Context, order *Order, hook TxHook) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(order).Exec(ctx); err != nil {
			return fmt.Errorf("insert order: %w", err)
		}

		for i := range order.Items {
			order.Items[i].OrderID = order.ID
			if order.Items[i].ID == uuid.Nil {
				order.Items[i].ID = uuid.New()
			}
			if _, err := tx.NewInsert().Model(&order.Items[i]).Exec(ctx); err != nil {
				return fmt.Errorf("insert order item %d: %w", i, err)
			}
		}

		if hook != nil {
			if err := hook(ctx, tx); err != nil {
				return fmt.Errorf("outbox hook: %w", err)
			}
		}
		return nil
	})
}

func (r *postgresRepository) loadItems(ctx context.Context, order *Order) error {
	var items []OrderItem
	if err := r.db.NewSelect().
		Model(&items).
		Where("order_id = ?", order.ID).
		Order("name ASC").
		Scan(ctx); err != nil {
		return err
	}
	order.Items = items
	return nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Order, error) {
	order := &Order{}
	err := r.db.NewSelect().
		Model(order).
		Where("id = ? AND deleted_at IS NULL", id).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("get order: %w", err)
	}
	_ = r.loadItems(ctx, order)
	return order, nil
}

func (r *postgresRepository) GetByOrderNumber(ctx context.Context, number string) (*Order, error) {
	order := &Order{}
	err := r.db.NewSelect().
		Model(order).
		Where("order_number = ? AND deleted_at IS NULL", number).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("get order by number: %w", err)
	}
	_ = r.loadItems(ctx, order)
	return order, nil
}

// UpdateWithOptimisticLock updates an order guarded by its version column, incrementing
// the version atomically. Returns ErrOptimisticLock if the row was modified concurrently.
func (r *postgresRepository) UpdateWithOptimisticLock(ctx context.Context, order *Order) error {
	order.UpdatedAt = time.Now()
	expectedVersion := order.Version

	res, err := r.db.NewUpdate().
		Model(order).
		Set("driver_id = ?", order.DriverID).
		Set("status = ?", order.Status).
		Set("payment_status = ?", order.PaymentStatus).
		Set("accepted_at = ?", order.AcceptedAt).
		Set("prepared_at = ?", order.PreparedAt).
		Set("picked_up_at = ?", order.PickedUpAt).
		Set("delivered_at = ?", order.DeliveredAt).
		Set("delivery_photo_url = ?", order.DeliveryPhotoURL).
		Set("completed_at = ?", order.CompletedAt).
		Set("cancelled_at = ?", order.CancelledAt).
		Set("cancel_reason = ?", order.CancelReason).
		Set("updated_at = ?", order.UpdatedAt).
		Set("version = version + 1").
		Where("id = ? AND version = ?", order.ID, expectedVersion).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("update order: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrOptimisticLock
	}
	order.Version++
	return nil
}

func (r *postgresRepository) ListByCustomer(ctx context.Context, customerID uuid.UUID, page, perPage int) ([]Order, int64, error) {
	var orders []Order
	query := r.db.NewSelect().Model(&orders).Where("customer_id = ? AND deleted_at IS NULL", customerID)

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Order("created_at DESC").Limit(perPage).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, err
	}
	return orders, int64(total), nil
}

func (r *postgresRepository) ListByRestaurant(ctx context.Context, restaurantID uuid.UUID, status *Status, page, perPage int) ([]Order, int64, error) {
	var orders []Order
	query := r.db.NewSelect().Model(&orders).Where("restaurant_id = ? AND deleted_at IS NULL", restaurantID)
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Order("created_at DESC").Limit(perPage).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, err
	}
	return orders, int64(total), nil
}

func (r *postgresRepository) ListByDriver(ctx context.Context, driverID uuid.UUID, page, perPage int) ([]Order, int64, error) {
	var orders []Order
	query := r.db.NewSelect().Model(&orders).Where("driver_id = ? AND deleted_at IS NULL", driverID)

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Order("created_at DESC").Limit(perPage).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, err
	}
	return orders, int64(total), nil
}

func (r *postgresRepository) GetPendingOrders(ctx context.Context, restaurantID uuid.UUID) ([]Order, error) {
	var orders []Order
	err := r.db.NewSelect().
		Model(&orders).
		Where("restaurant_id = ? AND status = ? AND deleted_at IS NULL", restaurantID, StatusPending).
		Order("created_at ASC").
		Scan(ctx)
	return orders, err
}

func (r *postgresRepository) GetStalePending(ctx context.Context, cutoff time.Time) ([]Order, error) {
	var orders []Order
	err := r.db.NewSelect().
		Model(&orders).
		Where("status = ? AND created_at < ? AND deleted_at IS NULL", StatusPending, cutoff).
		Scan(ctx)
	return orders, err
}

func (r *postgresRepository) GetStaleReady(ctx context.Context, cutoff time.Time) ([]Order, error) {
	var orders []Order
	err := r.db.NewSelect().
		Model(&orders).
		Where("status = ? AND driver_id IS NULL AND prepared_at < ? AND deleted_at IS NULL", StatusReady, cutoff).
		Scan(ctx)
	return orders, err
}

func (r *postgresRepository) GetSalesReport(ctx context.Context, restaurantID uuid.UUID, from, to time.Time) (*SalesReport, error) {
	report := &SalesReport{From: from, To: to}

	type statusCount struct {
		Status Status `bun:"status"`
		Count  int64  `bun:"count"`
	}
	var statusRows []statusCount
	if err := r.db.NewSelect().
		Model((*Order)(nil)).
		ColumnExpr("status, COUNT(*) AS count").
		Where("restaurant_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ?", restaurantID, from, to).
		Group("status").
		Scan(ctx, &statusRows); err != nil {
		return nil, fmt.Errorf("sales report status counts: %w", err)
	}
	for _, row := range statusRows {
		report.TotalOrders += row.Count
		switch row.Status {
		case StatusCompleted:
			report.CompletedOrders = row.Count
		case StatusCancelled:
			report.CancelledOrders = row.Count
		}
	}

	if err := r.db.NewSelect().
		Model((*Order)(nil)).
		ColumnExpr("COALESCE(SUM(total_amount), 0)").
		Where("restaurant_id = ? AND deleted_at IS NULL AND status = ? AND created_at >= ? AND created_at < ?",
			restaurantID, StatusCompleted, from, to).
		Scan(ctx, &report.TotalRevenue); err != nil {
		return nil, fmt.Errorf("sales report revenue: %w", err)
	}
	if report.CompletedOrders > 0 {
		report.AverageOrderValue = report.TotalRevenue / float64(report.CompletedOrders)
	}

	var daily []DailyRevenue
	if err := r.db.NewSelect().
		Model((*Order)(nil)).
		ColumnExpr("TO_CHAR(created_at, 'YYYY-MM-DD') AS date").
		ColumnExpr("COUNT(*) AS orders").
		ColumnExpr("COALESCE(SUM(total_amount), 0) AS revenue").
		Where("restaurant_id = ? AND deleted_at IS NULL AND status = ? AND created_at >= ? AND created_at < ?",
			restaurantID, StatusCompleted, from, to).
		GroupExpr("TO_CHAR(created_at, 'YYYY-MM-DD')").
		OrderExpr("date ASC").
		Scan(ctx, &daily); err != nil {
		return nil, fmt.Errorf("sales report daily revenue: %w", err)
	}
	report.RevenueByDay = daily

	var topItems []ItemSales
	if err := r.db.NewSelect().
		TableExpr("order_items AS oi").
		Join("JOIN orders AS o ON o.id = oi.order_id").
		ColumnExpr("oi.menu_item_id AS menu_item_id").
		ColumnExpr("oi.name AS name").
		ColumnExpr("SUM(oi.quantity) AS quantity").
		ColumnExpr("SUM(oi.total_price) AS revenue").
		Where("o.restaurant_id = ? AND o.deleted_at IS NULL AND o.status = ? AND o.created_at >= ? AND o.created_at < ?",
			restaurantID, StatusCompleted, from, to).
		GroupExpr("oi.menu_item_id, oi.name").
		OrderExpr("revenue DESC").
		Limit(10).
		Scan(ctx, &topItems); err != nil {
		return nil, fmt.Errorf("sales report top items: %w", err)
	}
	report.TopItems = topItems

	return report, nil
}

func (r *postgresRepository) CountByStatus(ctx context.Context, restaurantID uuid.UUID) (map[Status]int64, error) {
	type countRow struct {
		Status Status `bun:"status"`
		Count  int64  `bun:"count"`
	}
	var rows []countRow
	err := r.db.NewSelect().
		Model((*Order)(nil)).
		ColumnExpr("status, COUNT(*) AS count").
		Where("restaurant_id = ? AND deleted_at IS NULL", restaurantID).
		Group("status").
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}

	counts := make(map[Status]int64)
	for _, row := range rows {
		counts[row.Status] = row.Count
	}
	return counts, nil
}
