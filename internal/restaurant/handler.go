package restaurant

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

func (h *Handler) Create(c fiber.Ctx) error {
	var req CreateRestaurantRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	merchantID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	result, err := h.service.Create(c.Context(), merchantID, req)
	if err != nil {
		h.logger.Error("create restaurant failed", zap.Error(err))
		return response.InternalError(c)
	}

	return response.Created(c, result)
}

func (h *Handler) GetByID(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}

	result, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return response.NotFound(c, "Restaurant")
	}

	return response.Success(c, result)
}

func (h *Handler) Update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}

	merchantID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	var req UpdateRestaurantRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	result, err := h.service.Update(c.Context(), id, merchantID, req)
	if err != nil {
		switch err {
		case ErrRestaurantNotFound:
			return response.NotFound(c, "Restaurant")
		case ErrNotOwner:
			return response.Forbidden(c, "You do not own this restaurant")
		default:
			h.logger.Error("update restaurant failed", zap.Error(err))
			return response.InternalError(c)
		}
	}

	return response.Success(c, result)
}

func (h *Handler) Delete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}

	merchantID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	if err := h.service.Delete(c.Context(), id, merchantID); err != nil {
		switch err {
		case ErrRestaurantNotFound:
			return response.NotFound(c, "Restaurant")
		case ErrNotOwner:
			return response.Forbidden(c, "You do not own this restaurant")
		default:
			h.logger.Error("delete restaurant failed", zap.Error(err))
			return response.InternalError(c)
		}
	}

	return response.NoContent(c)
}

func (h *Handler) List(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	filter := ListFilter{
		Page:    page,
		PerPage: perPage,
		SortBy:  c.Query("sort_by", "created_at"),
		SortDir: c.Query("sort_dir", "desc"),
	}

	if status := c.Query("status"); status != "" {
		s := Status(status)
		filter.Status = &s
	}
	if cuisine := c.Query("cuisine"); cuisine != "" {
		filter.Cuisine = cuisine
	}
	if minRating := c.Query("min_rating"); minRating != "" {
		if r, err := strconv.ParseFloat(minRating, 64); err == nil {
			filter.MinRating = r
		}
	}

	restaurants, meta, err := h.service.List(c.Context(), filter)
	if err != nil {
		h.logger.Error("list restaurants failed", zap.Error(err))
		return response.InternalError(c)
	}

	return response.Paginated(c, restaurants, *meta)
}

func (h *Handler) ListByMerchant(c fiber.Ctx) error {
	merchantID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}

	restaurants, err := h.service.ListByMerchant(c.Context(), merchantID)
	if err != nil {
		h.logger.Error("list merchant restaurants failed", zap.Error(err))
		return response.InternalError(c)
	}

	return response.Success(c, restaurants)
}

func (h *Handler) ListFeatured(c fiber.Ctx) error {
	restaurants, err := h.service.ListFeatured(c.Context())
	if err != nil {
		h.logger.Error("list featured restaurants failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, restaurants)
}

func (h *Handler) ListNearby(c fiber.Ctx) error {
	var req NearbyRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	restaurants, err := h.service.ListNearby(c.Context(), req)
	if err != nil {
		h.logger.Error("list nearby restaurants failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, restaurants)
}

func (h *Handler) Search(c fiber.Ctx) error {
	var req SearchRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	restaurants, err := h.service.Search(c.Context(), req)
	if err != nil {
		h.logger.Error("search restaurants failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, restaurants)
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	// NOTE: all routes share the "/api/v1/restaurants" prefix, so they are
	// registered on a single group and auth/role middleware is attached
	// per-route rather than via nested Group() calls. Fiber mounts a group's
	// middleware with Use() on the shared prefix, which would otherwise apply
	// to every sibling route under that prefix regardless of method.
	api := router.Group("/api/v1/restaurants")
	auth1, role1 := authMW.RequireAuth(), authMW.RequireRole(string(auth.RoleMerchant))

	api.Get("/", h.List)
	api.Get("/featured", h.ListFeatured)
	api.Post("/nearby", h.ListNearby)
	api.Post("/search", h.Search)

	api.Post("/", auth1, role1, h.Create)
	api.Get("/my", auth1, role1, h.ListByMerchant)
	api.Put("/:id", auth1, role1, h.Update)
	api.Delete("/:id", auth1, role1, h.Delete)

	api.Get("/:id", h.GetByID)
}
