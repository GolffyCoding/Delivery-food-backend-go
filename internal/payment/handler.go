package payment

import (
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
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

func (h *Handler) Process(c fiber.Ctx) error {
	var req Request
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	p, err := h.service.ProcessPayment(c.Context(), req)
	if err != nil {
		if pe, ok := err.(*PaymentError); ok {
			return response.BadRequest(c, pe.Message)
		}
		h.logger.Error("process payment failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, p)
}

func (h *Handler) Authorize(c fiber.Ctx) error {
	var req Request
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	p, err := h.service.AuthorizePayment(c.Context(), req)
	if err != nil {
		if pe, ok := err.(*PaymentError); ok {
			return response.BadRequest(c, pe.Message)
		}
		h.logger.Error("authorize payment failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, p)
}

func (h *Handler) Confirm(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid payment ID")
	}
	p, err := h.service.ConfirmPayment(c.Context(), id)
	if err != nil {
		if pe, ok := err.(*PaymentError); ok {
			return response.BadRequest(c, pe.Message)
		}
		h.logger.Error("confirm payment failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, p)
}

func (h *Handler) Void(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid payment ID")
	}
	p, err := h.service.VoidPayment(c.Context(), id)
	if err != nil {
		if pe, ok := err.(*PaymentError); ok {
			return response.BadRequest(c, pe.Message)
		}
		h.logger.Error("void payment failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, p)
}

func (h *Handler) GetByOrderID(c fiber.Ctx) error {
	orderID, err := uuid.Parse(c.Params("order_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid order ID")
	}
	p, err := h.service.GetByOrderID(c.Context(), orderID)
	if err != nil {
		return response.NotFound(c, "Payment")
	}
	return response.Success(c, p)
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	api := router.Group("/api/v1/payments", authMW.RequireAuth())
	api.Post("/", h.Process)
	api.Post("/authorize", h.Authorize)
	api.Post("/:id/confirm", h.Confirm)
	api.Post("/:id/void", h.Void)
	api.Get("/order/:order_id", h.GetByOrderID)
}
