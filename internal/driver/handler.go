package driver

import (
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

func (h *Handler) Register(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	var req RegisterDriverRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	d, err := h.service.Register(c.Context(), userID, req)
	if err != nil {
		h.logger.Error("register driver failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, d)
}

func (h *Handler) Me(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	d, err := h.service.GetProfile(c.Context(), userID)
	if err != nil {
		return response.NotFound(c, "Driver")
	}
	return response.Success(c, d)
}

func (h *Handler) GoOnline(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	if err := h.service.GoOnline(c.Context(), userID); err != nil {
		return response.NotFound(c, "Driver")
	}
	return response.Success(c, fiber.Map{"status": "online"})
}

func (h *Handler) GoOffline(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	if err := h.service.GoOffline(c.Context(), userID); err != nil {
		return response.NotFound(c, "Driver")
	}
	return response.Success(c, fiber.Map{"status": "offline"})
}

func (h *Handler) UpdateLocation(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	var req LocationUpdateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	if err := h.service.UpdateLocation(c.Context(), userID, req); err != nil {
		return response.NotFound(c, "Driver")
	}
	return response.Success(c, fiber.Map{"message": "location updated"})
}

func (h *Handler) Earnings(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	earnings, total, err := h.service.GetEarnings(c.Context(), userID)
	if err != nil {
		return response.NotFound(c, "Driver")
	}
	return response.Success(c, fiber.Map{"earnings": earnings, "total": total})
}

func (h *Handler) FindNearest(c fiber.Ctx) error {
	var req struct {
		Latitude  float64 `json:"latitude" validate:"required"`
		Longitude float64 `json:"longitude" validate:"required"`
		RadiusKm  float64 `json:"radius_km" validate:"required,gte=0.5,lte=50"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	result, err := h.service.FindNearestDriver(c.Context(), req.Latitude, req.Longitude, req.RadiusKm)
	if err != nil {
		return response.NotFound(c, "Driver")
	}
	return response.Success(c, result)
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	// Per-route middleware (not nested Group()s) to avoid Fiber's Use()-based
	// group middleware bleeding across sibling routes on the same prefix.
	auth1, driverRole := authMW.RequireAuth(), authMW.RequireRole(string(auth.RoleDriver))
	dispatcherRole := authMW.RequireRole(string(auth.RoleAdmin), string(auth.RoleMerchant))

	drivers := router.Group("/api/v1/drivers")
	drivers.Post("/register", auth1, driverRole, h.Register)
	drivers.Get("/me", auth1, driverRole, h.Me)
	drivers.Post("/online", auth1, driverRole, h.GoOnline)
	drivers.Post("/offline", auth1, driverRole, h.GoOffline)
	drivers.Post("/location", auth1, driverRole, h.UpdateLocation)
	drivers.Get("/earnings", auth1, driverRole, h.Earnings)
	drivers.Post("/nearest", auth1, dispatcherRole, h.FindNearest)
}
