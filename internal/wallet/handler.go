package wallet

import (
	"strconv"

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

func (h *Handler) Me(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	w, err := h.service.GetOrCreate(c.Context(), userID)
	if err != nil {
		h.logger.Error("get wallet failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Success(c, w)
}

func (h *Handler) TopUp(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	var req TopUpRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}
	tx, err := h.service.TopUp(c.Context(), userID, req)
	if err != nil {
		h.logger.Error("wallet top-up failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Created(c, tx)
}

func (h *Handler) Transactions(c fiber.Ctx) error {
	userID, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Invalid user")
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	txs, total, err := h.service.ListTransactions(c.Context(), userID, page, perPage)
	if err != nil {
		h.logger.Error("list wallet transactions failed", zap.Error(err))
		return response.InternalError(c)
	}
	return response.Paginated(c, txs, response.Meta{Page: page, PerPage: perPage, Total: total})
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	api := router.Group("/api/v1/wallet", authMW.RequireAuth())
	api.Get("/", h.Me)
	api.Post("/topup", h.TopUp)
	api.Get("/transactions", h.Transactions)
}
