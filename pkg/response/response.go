package response

import (
	"net/http"

	"github.com/gofiber/fiber/v3"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Meta struct {
	Page       int   `json:"page,omitempty"`
	PerPage    int   `json:"per_page,omitempty"`
	Total      int64 `json:"total,omitempty"`
	TotalPages int   `json:"total_pages,omitempty"`
	HasMore    bool  `json:"has_more,omitempty"`
}

func Success(c fiber.Ctx, data interface{}) error {
	return c.Status(http.StatusOK).JSON(APIResponse{Success: true, Data: data})
}

func Created(c fiber.Ctx, data interface{}) error {
	return c.Status(http.StatusCreated).JSON(APIResponse{Success: true, Data: data})
}

func NoContent(c fiber.Ctx) error {
	return c.SendStatus(http.StatusNoContent)
}

func Paginated(c fiber.Ctx, data interface{}, meta Meta) error {
	return c.Status(http.StatusOK).JSON(APIResponse{Success: true, Data: data, Meta: &meta})
}

func Error(c fiber.Ctx, status int, code string, message string) error {
	return c.Status(status).JSON(APIResponse{
		Success: false,
		Error:   &APIError{Code: code, Message: message},
	})
}

func BadRequest(c fiber.Ctx, message string) error {
	return Error(c, http.StatusBadRequest, "BAD_REQUEST", message)
}

func Unauthorized(c fiber.Ctx, message string) error {
	return Error(c, http.StatusUnauthorized, "UNAUTHORIZED", message)
}

func Forbidden(c fiber.Ctx, message string) error {
	return Error(c, http.StatusForbidden, "FORBIDDEN", message)
}

func NotFound(c fiber.Ctx, resource string) error {
	return Error(c, http.StatusNotFound, resource+"_NOT_FOUND", resource+" not found")
}

func Conflict(c fiber.Ctx, message string) error {
	return Error(c, http.StatusConflict, "CONFLICT", message)
}

func InternalError(c fiber.Ctx) error {
	return Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred")
}

func ValidationError(c fiber.Ctx, message string) error {
	return Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", message)
}

func TooManyRequests(c fiber.Ctx) error {
	return Error(c, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Too many requests, please try again later")
}
