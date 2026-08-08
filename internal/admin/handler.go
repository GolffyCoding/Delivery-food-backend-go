package admin

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/opendelivery/opendelivery/pkg/middleware"
	"github.com/opendelivery/opendelivery/pkg/response"
	"github.com/uptrace/bun"
)

type Handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) *Handler {
	return &Handler{db: db}
}

type dashboardStats struct {
	TotalUsers         int     `json:"total_users"`
	TotalOrders        int     `json:"total_orders"`
	TotalRevenue       float64 `json:"total_revenue"`
	ActiveRestaurants  int     `json:"active_restaurants"`
	OnlineDrivers      int     `json:"online_drivers"`
	PendingOrders      int     `json:"pending_orders"`
	OrdersToday        int     `json:"orders_today"`
	RevenueToday       float64 `json:"revenue_today"`
}

func (h *Handler) Dashboard(c fiber.Ctx) error {
	ctx := context.Background()
	var stats dashboardStats

	stats.TotalUsers, _ = h.db.NewSelect().Table("users").Where("deleted_at IS NULL").Count(ctx)
	stats.TotalOrders, _ = h.db.NewSelect().Table("orders").Where("deleted_at IS NULL").Count(ctx)
	stats.ActiveRestaurants, _ = h.db.NewSelect().Table("restaurants").Where("status = 'active' AND deleted_at IS NULL").Count(ctx)
	stats.OnlineDrivers, _ = h.db.NewSelect().Table("drivers").Where("status = 'online'").Count(ctx)
	stats.PendingOrders, _ = h.db.NewSelect().Table("orders").Where("status = 'pending' AND deleted_at IS NULL").Count(ctx)
	stats.OrdersToday, _ = h.db.NewSelect().Table("orders").Where("created_at >= CURRENT_DATE AND deleted_at IS NULL").Count(ctx)

	_ = h.db.NewSelect().Table("orders").
		ColumnExpr("COALESCE(SUM(total_amount), 0)").
		Where("status = 'completed' AND deleted_at IS NULL").
		Scan(ctx, &stats.TotalRevenue)

	_ = h.db.NewSelect().Table("orders").
		ColumnExpr("COALESCE(SUM(total_amount), 0)").
		Where("status = 'completed' AND created_at >= CURRENT_DATE AND deleted_at IS NULL").
		Scan(ctx, &stats.RevenueToday)

	return response.Success(c, stats)
}

func (h *Handler) UserList(c fiber.Ctx) error {
	ctx := context.Background()
	type userRow struct {
		ID    string `bun:"id" json:"id"`
		Email string `bun:"email" json:"email"`
		Role  string `bun:"role" json:"role"`
	}
	var users []userRow
	err := h.db.NewSelect().Table("users").
		Column("id", "email", "role").
		Where("deleted_at IS NULL").
		OrderExpr("created_at DESC").
		Limit(50).
		Scan(ctx, &users)
	if err != nil {
		return response.InternalError(c)
	}
	return response.Success(c, users)
}

func (h *Handler) SystemHealth(c fiber.Ctx) error {
	ctx := context.Background()
	dbStatus := "healthy"
	if err := h.db.PingContext(ctx); err != nil {
		dbStatus = "unhealthy"
	}
	return response.Success(c, fiber.Map{"database": dbStatus})
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	api := router.Group("/api/v1/admin", authMW.RequireAuth(), authMW.RequireRole("admin"))
	api.Get("/dashboard", h.Dashboard)
	api.Get("/users", h.UserList)
	api.Get("/system/health", h.SystemHealth)
}
