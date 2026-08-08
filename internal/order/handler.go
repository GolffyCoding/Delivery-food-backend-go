package order

import (
	"errors"
	"strconv"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/opendelivery/opendelivery/internal/auth"
	"github.com/opendelivery/opendelivery/pkg/middleware"
	"github.com/opendelivery/opendelivery/pkg/response"
	"go.uber.org/zap"
)

type Handler struct {
	service  *Service
	validate *validator.Validate
	logger   *zap.Logger
}

func NewHandler(service *Service, validate *validator.Validate, logger *zap.Logger) *Handler {
	return &Handler{service: service, validate: validate, logger: logger}
}

func (h *Handler) Create(c fiber.Ctx) error {
	var req CreateOrderRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	customerID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	ord, err := h.service.Create(c.Context(), customerID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrRestaurantClosed):
			return response.BadRequest(c, "Restaurant is currently closed or unavailable")
		case errors.Is(err, ErrEmptyOrder):
			return response.BadRequest(c, "Order must have at least one item")
		case errors.Is(err, ErrBelowMinimumOrder):
			return response.BadRequest(c, "Order total is below the minimum order amount")
		case errors.Is(err, ErrInvalidCoupon):
			return response.BadRequest(c, "Coupon code is invalid, expired, or cannot be applied to this order")
		default:
			h.logger.Error("create order failed", zap.Error(err))
			return response.InternalError(c)
		}
	}

	return response.Created(c, ord)
}

func (h *Handler) GetByID(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid order ID")
	}
	ord, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return response.NotFound(c, "Order")
	}
	return response.Success(c, ord)
}

func (h *Handler) GetByOrderNumber(c fiber.Ctx) error {
	number := c.Params("number")
	ord, err := h.service.GetByOrderNumber(c.Context(), number)
	if err != nil {
		return response.NotFound(c, "Order")
	}
	return response.Success(c, ord)
}

func (h *Handler) UpdateStatus(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid order ID")
	}

	var req UpdateStatusRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	ord, err := h.service.UpdateStatus(c.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrOrderNotFound):
			return response.NotFound(c, "Order")
		case errors.Is(err, ErrInvalidTransition):
			return response.BadRequest(c, err.Error())
		case errors.Is(err, ErrOptimisticLock):
			return response.Conflict(c, "Order was modified by another process, please retry")
		default:
			h.logger.Error("update order status failed", zap.Error(err))
			return response.InternalError(c)
		}
	}

	return response.Success(c, ord)
}

func (h *Handler) AcceptOrder(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid order ID")
	}
	driverID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	var req struct {
		PickupCode string `json:"pickup_code" validate:"required,len=4"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	ord, err := h.service.AcceptOrder(c.Context(), id, driverID, req.PickupCode)
	if err != nil {
		switch {
		case errors.Is(err, ErrOrderNotFound):
			return response.NotFound(c, "Order")
		case errors.Is(err, ErrInvalidTransition):
			return response.BadRequest(c, "Order is not ready for pickup")
		case errors.Is(err, ErrPickupCodeMismatch):
			return response.BadRequest(c, "Pickup code does not match this order — double-check you have the right order")
		default:
			return response.Conflict(c, err.Error())
		}
	}
	return response.Success(c, ord)
}

func (h *Handler) Cancel(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid order ID")
	}
	customerID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	var req struct {
		Reason string `json:"reason" validate:"max=500"`
	}
	_ = c.Bind().JSON(&req)

	ord, err := h.service.Cancel(c.Context(), id, req.Reason, customerID)
	if err != nil {
		switch {
		case errors.Is(err, ErrOrderNotFound):
			return response.NotFound(c, "Order")
		case errors.Is(err, ErrNotYourOrder):
			return response.Forbidden(c, "This is not your order")
		case errors.Is(err, ErrInvalidTransition):
			return response.BadRequest(c, "This order can no longer be cancelled")
		default:
			h.logger.Error("cancel order failed", zap.Error(err))
			return response.InternalError(c)
		}
	}
	return response.Success(c, ord)
}

func (h *Handler) ListByCustomer(c fiber.Ctx) error {
	customerID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	orders, meta, err := h.service.ListByCustomer(c.Context(), customerID, page, perPage)
	if err != nil {
		h.logger.Error("list customer orders failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Paginated(c, orders, *meta)
}

func (h *Handler) ListByRestaurant(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	var statusPtr *Status
	if s := c.Query("status"); s != "" {
		st := Status(s)
		statusPtr = &st
	}

	orders, meta, err := h.service.ListByRestaurant(c.Context(), restaurantID, statusPtr, page, perPage)
	if err != nil {
		h.logger.Error("list restaurant orders failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Paginated(c, orders, *meta)
}

func (h *Handler) ListByDriver(c fiber.Ctx) error {
	driverID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	orders, meta, err := h.service.ListByDriver(c.Context(), driverID, page, perPage)
	if err != nil {
		h.logger.Error("list driver orders failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Paginated(c, orders, *meta)
}

func (h *Handler) GetPendingOrders(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	orders, err := h.service.GetPendingOrders(c.Context(), restaurantID)
	if err != nil {
		h.logger.Error("get pending orders failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, orders)
}

func (h *Handler) CountByStatus(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	counts, err := h.service.CountByStatus(c.Context(), restaurantID)
	if err != nil {
		h.logger.Error("count orders by status failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, counts)
}

// SalesReport handles GET /restaurants/:restaurant_id/orders/sales-report.
// Optional query params `from` and `to` accept dates as YYYY-MM-DD; without
// them it reports the trailing 30 days.
func (h *Handler) SalesReport(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}

	var from, to time.Time
	if raw := c.Query("from"); raw != "" {
		from, err = time.Parse("2006-01-02", raw)
		if err != nil {
			return response.BadRequest(c, "Invalid 'from' date, expected YYYY-MM-DD")
		}
	}
	if raw := c.Query("to"); raw != "" {
		to, err = time.Parse("2006-01-02", raw)
		if err != nil {
			return response.BadRequest(c, "Invalid 'to' date, expected YYYY-MM-DD")
		}
	}

	report, err := h.service.GetSalesReport(c.Context(), restaurantID, from, to)
	if err != nil {
		h.logger.Error("sales report failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, report)
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	// All of these share the "/api/v1/orders" prefix with different role
	// requirements per route, so middleware is attached per-route rather than
	// via nested Group() calls (see restaurant.RegisterRoutes for why that
	// matters: Fiber mounts a group's middleware with Use() on the shared
	// prefix, which bleeds across every sibling route under it).
	auth1 := authMW.RequireAuth()
	customerRole := authMW.RequireRole(string(auth.RoleCustomer))
	merchantRole := authMW.RequireRole(string(auth.RoleMerchant))
	driverRole := authMW.RequireRole(string(auth.RoleDriver))
	adminRole := authMW.RequireRole(string(auth.RoleAdmin))
	merchantOrAdminRole := authMW.RequireRole(string(auth.RoleMerchant), string(auth.RoleAdmin))

	orders := router.Group("/api/v1/orders")
	orders.Post("/", auth1, customerRole, h.Create)
	orders.Get("/", auth1, customerRole, h.ListByCustomer)
	orders.Post("/:id/cancel", auth1, customerRole, h.Cancel)
	orders.Put("/:id/status", auth1, merchantOrAdminRole, h.UpdateStatus)
	orders.Get("/number/:number", auth1, h.GetByOrderNumber)
	orders.Get("/:id", auth1, h.GetByID)

	restOrders := router.Group("/api/v1/restaurants/:restaurant_id/orders")
	restOrders.Get("/", auth1, merchantRole, h.ListByRestaurant)
	restOrders.Get("/pending", auth1, merchantRole, h.GetPendingOrders)
	restOrders.Get("/count", auth1, merchantRole, h.CountByStatus)
	restOrders.Get("/sales-report", auth1, merchantRole, h.SalesReport)

	driverOrders := router.Group("/api/v1/driver/orders")
	driverOrders.Get("/", auth1, driverRole, h.ListByDriver)
	driverOrders.Post("/:id/accept", auth1, driverRole, h.AcceptOrder)
	driverOrders.Put("/:id/status", auth1, driverRole, h.UpdateStatus)

	adminOrders := router.Group("/api/v1/admin/orders")
	adminOrders.Put("/:id/status", auth1, adminRole, h.UpdateStatus)
}
