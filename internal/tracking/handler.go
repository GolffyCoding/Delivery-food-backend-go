package tracking

import (
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

func (h *Handler) UpdateLocation(c fiber.Ctx) error {
	driverID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	var req struct {
		Latitude  float64 `json:"latitude" validate:"required"`
		Longitude float64 `json:"longitude" validate:"required"`
		Heading   float64 `json:"heading"`
		Speed     float64 `json:"speed"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	err = h.service.UpdateDriverLocation(c.Context(), LocationUpdate{
		DriverID:  driverID,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		Heading:   req.Heading,
		Speed:     req.Speed,
		Timestamp: time.Now(),
	})
	if err != nil {
		h.logger.Error("update driver location failed", zap.Error(err))
		return response.InternalError(c)
	}

	return response.Success(c, fiber.Map{"message": "location updated"})
}

func (h *Handler) ETA(c fiber.Ctx) error {
	var req struct {
		DriverLat float64 `json:"driver_lat" validate:"required"`
		DriverLng float64 `json:"driver_lng" validate:"required"`
		DestLat   float64 `json:"dest_lat" validate:"required"`
		DestLng   float64 `json:"dest_lng" validate:"required"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	result := h.service.CalculateETA(req.DriverLat, req.DriverLng, req.DestLat, req.DestLng, 25)
	return response.Success(c, result)
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	api := router.Group("/api/v1/tracking")
	api.Post("/eta", h.ETA)
	api.Post("/location", authMW.RequireAuth(), authMW.RequireRole(string(auth.RoleDriver)), h.UpdateLocation)
}
