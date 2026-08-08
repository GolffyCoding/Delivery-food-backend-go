package review

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
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	var req CreateReviewRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	rv, err := h.service.Create(c.Context(), userID, req)
	if err != nil {
		if err == ErrAlreadyReviewed {
			return response.Conflict(c, "This order has already been reviewed")
		}
		h.logger.Error("create review failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, rv)
}

func (h *Handler) ListByRestaurant(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	reviews, total, err := h.service.ListByRestaurant(c.Context(), restaurantID, page, perPage)
	if err != nil {
		h.logger.Error("list reviews failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Paginated(c, reviews, response.Meta{Page: page, PerPage: perPage, Total: total})
}

func (h *Handler) Summary(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	summary, err := h.service.GetRestaurantSummary(c.Context(), restaurantID)
	if err != nil {
		h.logger.Error("get rating summary failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, summary)
}

func (h *Handler) Reply(c fiber.Ctx) error {
	restaurantID, err := uuid.Parse(c.Params("restaurant_id"))
	if err != nil {
		return response.BadRequest(c, "Invalid restaurant ID")
	}
	reviewID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "Invalid review ID")
	}

	var req struct {
		Reply string `json:"reply" validate:"required,max=1000"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	if err := h.service.Reply(c.Context(), reviewID, restaurantID, req.Reply); err != nil {
		h.logger.Error("reply to review failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, fiber.Map{"message": "reply added"})
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	restReviews := router.Group("/api/v1/restaurants/:restaurant_id/reviews")
	restReviews.Get("/", h.ListByRestaurant)
	restReviews.Get("/summary", h.Summary)
	restReviews.Post("/:id/reply", authMW.RequireAuth(), authMW.RequireRole(string(auth.RoleMerchant)), h.Reply)

	customer := router.Group("/api/v1/reviews", authMW.RequireAuth(), authMW.RequireRole(string(auth.RoleCustomer)))
	customer.Post("/", h.Create)
}
