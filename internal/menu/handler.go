package menu

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

func (h *Handler) ListCategories(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	categories, err := h.service.ListCategories(c.Context(), restaurantID)
	if err != nil {
		h.logger.Error("list categories failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, categories)
}

func (h *Handler) CreateCategory(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	var req CreateCategoryRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	cat, err := h.service.CreateCategory(c.Context(), restaurantID, req)
	if err != nil {
		h.logger.Error("create category failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, cat)
}

func (h *Handler) DeleteCategory(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid category ID")
	}
	if err := h.service.DeleteCategory(c.Context(), id, restaurantID); err != nil {
		h.logger.Error("delete category failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.NoContent(c)
}

func (h *Handler) ListItems(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	activeOnly := c.Query("active_only", "true") == "true"
	items, err := h.service.ListItems(c.Context(), restaurantID, activeOnly)
	if err != nil {
		h.logger.Error("list items failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, items)
}

func (h *Handler) GetItem(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid item ID")
	}
	item, err := h.service.GetItem(c.Context(), id)
	if err != nil {
		return response.NotFound(c, "MenuItem")
	}
	return response.Success(c, item)
}

func (h *Handler) CreateItem(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	var req CreateItemRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	item, err := h.service.CreateItem(c.Context(), restaurantID, req)
	if err != nil {
		h.logger.Error("create item failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, item)
}

func (h *Handler) UpdateItem(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid item ID")
	}
	var req UpdateItemRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	item, err := h.service.UpdateItem(c.Context(), id, restaurantID, req)
	if err != nil {
		if err == ErrNotFound {
			return response.NotFound(c, "MenuItem")
		}
		h.logger.Error("update item failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, item)
}

func (h *Handler) DeleteItem(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid item ID")
	}
	if err := h.service.DeleteItem(c.Context(), id, restaurantID); err != nil {
		h.logger.Error("delete item failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.NoContent(c)
}

func (h *Handler) CreateVariant(c fiber.Ctx) error {
	itemID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid item ID")
	}
	var req CreateVariantRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	v, err := h.service.CreateVariant(c.Context(), itemID, req)
	if err != nil {
		h.logger.Error("create variant failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, v)
}

func (h *Handler) CreateAddOn(c fiber.Ctx) error {
	itemID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid item ID")
	}
	var req CreateAddOnRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	a, err := h.service.CreateAddOn(c.Context(), itemID, req)
	if err != nil {
		h.logger.Error("create addon failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, a)
}

func (h *Handler) ListVariants(c fiber.Ctx) error {
	itemID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid item ID")
	}
	variants, err := h.service.ListVariants(c.Context(), itemID)
	if err != nil {
		h.logger.Error("list variants failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, variants)
}

func (h *Handler) ListAddOns(c fiber.Ctx) error {
	itemID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid item ID")
	}
	addons, err := h.service.ListAddOns(c.Context(), itemID)
	if err != nil {
		h.logger.Error("list addons failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, addons)
}

func (h *Handler) DeleteVariant(c fiber.Ctx) error {
	itemID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid item ID")
	}
	variantID, err := uuid.Parse(c.Params("variant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid variant ID")
	}
	if err := h.service.DeleteVariant(c.Context(), variantID, itemID); err != nil {
		h.logger.Error("delete variant failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.NoContent(c)
}

func (h *Handler) DeleteAddOn(c fiber.Ctx) error {
	itemID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid item ID")
	}
	addonID, err := uuid.Parse(c.Params("addon_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid addon ID")
	}
	if err := h.service.DeleteAddOn(c.Context(), addonID, itemID); err != nil {
		h.logger.Error("delete addon failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.NoContent(c)
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	// Sibling routes share prefixes, so auth/role middleware is attached
	// per-route (see the comment in restaurant.RegisterRoutes for why).
	auth1, role1 := authMW.RequireAuth(), authMW.RequireRole(string(auth.RoleMerchant))

	restMenu := router.Group("/api/v1/restaurants/:restaurant_id/menu")
	restMenu.Get("/categories", h.ListCategories)
	restMenu.Get("/items", h.ListItems)
	restMenu.Post("/categories", auth1, role1, h.CreateCategory)
	restMenu.Delete("/categories/:id", auth1, role1, h.DeleteCategory)
	restMenu.Post("/items", auth1, role1, h.CreateItem)
	restMenu.Put("/items/:id", auth1, role1, h.UpdateItem)
	restMenu.Delete("/items/:id", auth1, role1, h.DeleteItem)

	items := router.Group("/api/v1/menu-items")
	items.Get("/:id", h.GetItem)
	items.Get("/:id/variants", h.ListVariants)
	items.Get("/:id/addons", h.ListAddOns)
	items.Post("/:id/variants", auth1, role1, h.CreateVariant)
	items.Post("/:id/addons", auth1, role1, h.CreateAddOn)
	items.Delete("/:id/variants/:variant_id", auth1, role1, h.DeleteVariant)
	items.Delete("/:id/addons/:addon_id", auth1, role1, h.DeleteAddOn)
}
