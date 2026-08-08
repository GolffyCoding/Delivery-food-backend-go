package notification

import (
	"strconv"

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

func (h *Handler) List(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	items, total, err := h.service.ListByUser(c.Context(), userID, page, perPage)
	if err != nil {
		h.logger.Error("list notifications failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Paginated(c, items, response.Meta{Page: page, PerPage: perPage, Total: total})
}

func (h *Handler) UnreadCount(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	count, err := h.service.UnreadCount(c.Context(), userID)
	if err != nil {
		h.logger.Error("unread count failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, fiber.Map{"unread_count": count})
}

func (h *Handler) MarkRead(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid notification ID")
	}
	if err := h.service.MarkRead(c.Context(), id, userID); err != nil {
		h.logger.Error("mark notification read failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, fiber.Map{"message": "marked as read"})
}

func (h *Handler) Send(c fiber.Ctx) error {
	var req SendRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	n, err := h.service.Send(c.Context(), req)
	if err != nil {
		h.logger.Error("send notification failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, n)
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	api := router.Group("/api/v1/notifications", authMW.RequireAuth())
	api.Get("/", h.List)
	api.Get("/unread-count", h.UnreadCount)
	api.Post("/:id/read", h.MarkRead)

	admin := router.Group("/api/v1/admin/notifications", authMW.RequireAuth(), authMW.RequireRole(string(auth.RoleAdmin)))
	admin.Post("/", h.Send)
}
