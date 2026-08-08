package auth

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

func (h *Handler) Register(c fiber.Ctx) error {
	var req RegisterRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	result, err := h.service.Register(c.Context(), req)
	if err != nil {
		switch err {
		case ErrUserAlreadyExists:
			return response.Conflict(c, "Email already registered")
		case ErrInvalidRole:
			return response.BadRequest(c, "Invalid role specified")
		default:
			h.logger.Error("register failed", zap.Error(err))
			return response.InternalError(c)
		}
	}

	return response.Created(c, result)
}

func (h *Handler) Login(c fiber.Ctx) error {
	var req LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	result, err := h.service.Login(c.Context(), req)
	if err != nil {
		switch err {
		case ErrInvalidCredentials:
			return response.Unauthorized(c, "Invalid email or password")
		case ErrAccountDisabled:
			return response.Forbidden(c, "Account is disabled")
		default:
			h.logger.Error("login failed", zap.Error(err))
			return response.InternalError(c)
		}
	}

	return response.Success(c, result)
}

func (h *Handler) RefreshToken(c fiber.Ctx) error {
	var req RefreshRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	result, err := h.service.RefreshToken(c.Context(), req)
	if err != nil {
		switch err {
		case ErrInvalidToken, ErrTokenRevoked:
			return response.Unauthorized(c, "Invalid or expired refresh token")
		default:
			h.logger.Error("refresh token failed", zap.Error(err))
			return response.InternalError(c)
		}
	}

	return response.Success(c, result)
}

func (h *Handler) Logout(c fiber.Ctx) error {
	id, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Not authenticated")
	}

	if err := h.service.Logout(c.Context(), id); err != nil {
		h.logger.Error("logout failed", zap.Error(err))
		return response.InternalError(c)
	}

	return response.Success(c, fiber.Map{"message": "Logged out successfully"})
}

func (h *Handler) Me(c fiber.Ctx) error {
	id, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		return response.Unauthorized(c, "Not authenticated")
	}

	user, err := h.service.GetProfile(c.Context(), id)
	if err != nil {
		return response.NotFound(c, "User")
	}

	return response.Success(c, user)
}

func (h *Handler) VerifyEmail(c fiber.Ctx) error {
	var req VerifyEmailRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	if err := h.service.VerifyEmail(c.Context(), req); err != nil {
		switch err {
		case ErrInvalidToken:
			return response.BadRequest(c, "Invalid or expired verification token")
		default:
			h.logger.Error("verify email failed", zap.Error(err))
			return response.InternalError(c)
		}
	}

	return response.Success(c, fiber.Map{"message": "Email verified successfully"})
}

func (h *Handler) ForgotPassword(c fiber.Ctx) error {
	var req ForgotPasswordRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	if err := h.service.ForgotPassword(c.Context(), req); err != nil {
		h.logger.Error("forgot password failed", zap.Error(err))
	}

	return response.Success(c, fiber.Map{"message": "If the email exists, a reset link has been sent"})
}

func (h *Handler) ResetPassword(c fiber.Ctx) error {
	var req ResetPasswordRequest
	if err := c.Bind().JSON(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	if err := h.validate.Struct(req); err != nil {
		return response.ValidationError(c, err.Error())
	}

	if err := h.service.ResetPassword(c.Context(), req); err != nil {
		switch err {
		case ErrInvalidToken:
			return response.BadRequest(c, "Invalid or expired reset token")
		default:
			h.logger.Error("reset password failed", zap.Error(err))
			return response.InternalError(c)
		}
	}

	return response.Success(c, fiber.Map{"message": "Password reset successfully"})
}

func RegisterRoutes(router fiber.Router, h *Handler, authMW *middleware.AuthMiddleware) {
	api := router.Group("/api/v1/auth")

	api.Post("/register", h.Register)
	api.Post("/login", h.Login)
	api.Post("/refresh", h.RefreshToken)
	api.Post("/verify-email", h.VerifyEmail)
	api.Post("/forgot-password", h.ForgotPassword)
	api.Post("/reset-password", h.ResetPassword)

	api.Post("/logout", authMW.RequireAuth(), h.Logout)
	api.Get("/me", authMW.RequireAuth(), h.Me)
}
