package support

import (
	"errors"
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

func (h *Handler) Create(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	var req CreateTicketRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	t, err := h.service.Create(c.Context(), userID, req)
	if err != nil {
		h.logger.Error("create support ticket failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, t)
}

func (h *Handler) ListMine(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	tickets, total, err := h.service.ListByUser(c.Context(), userID, page, perPage)
	if err != nil {
		h.logger.Error("list support tickets failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Paginated(c, tickets, response.Meta{Page: page, PerPage: perPage, Total: total})
}

func (h *Handler) GetByID(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid ticket ID")
	}
	t, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return response.NotFound(c, "Support ticket")
	}

	// Owners can see their own ticket; anyone else must be staff (route-level
	// RequireRole already keeps non-staff off the admin variant of this handler, but
	// the shared GetByID above is also reachable by the customer's own route).
	if role := middleware.Role(c); role != string(auth.RoleAdmin) {
		userID, err := uuid.Parse(middleware.UserID(c))
		if err != nil || t.UserID != userID {
			return response.Forbidden(c, "This is not your support ticket")
		}
	}
	return response.Success(c, t)
}

func (h *Handler) ListOpen(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	tickets, total, err := h.service.ListOpen(c.Context(), page, perPage)
	if err != nil {
		h.logger.Error("list open support tickets failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Paginated(c, tickets, response.Meta{Page: page, PerPage: perPage, Total: total})
}

func (h *Handler) Assign(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid ticket ID")
	}
	adminID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	t, err := h.service.Assign(c.Context(), id, adminID)
	if err != nil {
		switch {
		case errors.Is(err, ErrTicketNotFound):
			return response.NotFound(c, "Support ticket")
		case errors.Is(err, ErrAlreadyResolved):
			return response.BadRequest(c, "Ticket is already resolved or closed")
		default:
			h.logger.Error("assign support ticket failed", zap.Error(err))
			return response.InternalError(c)
		}
	}
	return response.Success(c, t)
}

func (h *Handler) Resolve(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid ticket ID")
	}
	adminID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	var req ResolveRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	t, err := h.service.Resolve(c.Context(), id, adminID, req.Resolution)
	if err != nil {
		switch {
		case errors.Is(err, ErrTicketNotFound):
			return response.NotFound(c, "Support ticket")
		case errors.Is(err, ErrAlreadyResolved):
			return response.BadRequest(c, "Ticket is already resolved or closed")
		default:
			h.logger.Error("resolve support ticket failed", zap.Error(err))
			return response.InternalError(c)
		}
	}
	return response.Success(c, t)
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	auth1 := authMW.RequireAuth()
	adminRole := authMW.RequireRole(string(auth.RoleAdmin))

	tickets := router.Group("/api/v1/support/tickets", auth1)
	tickets.Post("/", h.Create)
	tickets.Get("/", h.ListMine)
	tickets.Get("/:id", h.GetByID)

	admin := router.Group("/api/v1/admin/support/tickets", auth1, adminRole)
	admin.Get("/", h.ListOpen)
	admin.Put("/:id/assign", h.Assign)
	admin.Put("/:id/resolve", h.Resolve)
}
