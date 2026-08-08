package coupon

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

func (h *Handler) Create(c fiber.Ctx) error {
	var req CreateCouponRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	coup, err := h.service.Create(c.Context(), req)
	if err != nil {
		h.logger.Error("create coupon failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, coup)
}

func (h *Handler) ListActive(c fiber.Ctx) error {
	coupons, err := h.service.ListActive(c.Context())
	if err != nil {
		h.logger.Error("list active coupons failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, coupons)
}

func (h *Handler) Validate(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	var req struct {
		Code     string  `json:"code" validate:"required"`
		Subtotal float64 `json:"subtotal" validate:"required,gt=0"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	result, err := h.service.Preview(c.Context(), req.Code, userID, req.Subtotal)
	if err != nil {
		if err == ErrCouponNotFound {
			return response.NotFound(c, "Coupon not found")
		}
		h.logger.Error("preview coupon failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, result)
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	admin := router.Group("/api/v1/admin/coupons", authMW.RequireAuth(), authMW.RequireRole(string(auth.RoleAdmin)))
	admin.Post("/", h.Create)

	customer := router.Group("/api/v1/coupons", authMW.RequireAuth(), authMW.RequireRole(string(auth.RoleCustomer)))
	customer.Get("/", h.ListActive)
	customer.Post("/validate", h.Validate)
}
