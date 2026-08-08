package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/opendelivery/opendelivery/configs"
	"github.com/opendelivery/opendelivery/pkg/middleware"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo      Repository
	jwtConfig *configs.JWTConfig
}

func NewService(repo Repository, jwtConfig *configs.JWTConfig) *Service {
	return &Service{repo: repo, jwtConfig: jwtConfig}
}

type RegisterRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8,max=128"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name" validate:"required,min=1,max=100"`
	Phone     string `json:"phone" validate:"omitempty"`
	Role      Role   `json:"role" validate:"omitempty,oneof=customer driver merchant admin"`
}

type RegisterResponse struct {
	User User `json:"user"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type LoginResponse struct {
	User      User      `json:"user"`
	TokenPair TokenPair `json:"token_pair"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordRequest struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	if req.Role == "" {
		req.Role = RoleCustomer
	}
	if !req.Role.IsValid() {
		return nil, ErrInvalidRole
	}

	_, err := s.repo.FindByEmail(ctx, req.Email)
	if err == nil {
		return nil, ErrUserAlreadyExists
	}
	if !errors.Is(err, ErrUserNotFound) {
		return nil, fmt.Errorf("check existing user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &User{
		ID:           uuid.New(),
		Email:        req.Email,
		PasswordHash: string(hash),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Phone:        req.Phone,
		Role:         req.Role,
		Active:       true,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	verifyToken := generateSecureToken(32)
	_ = s.repo.StoreEmailVerification(ctx, &EmailVerification{
		ID:        uuid.New(),
		UserID:    user.ID,
		Token:     verifyToken,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	return &RegisterResponse{User: *user}, nil
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	user, err := s.repo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if !user.Active {
		return nil, ErrAccountDisabled
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	tokenPair, err := s.generateTokenPair(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	return &LoginResponse{User: *user, TokenPair: *tokenPair}, nil
}

func (s *Service) RefreshToken(ctx context.Context, req RefreshRequest) (*TokenPair, error) {
	tokenHash := HashToken(req.RefreshToken)

	userID, err := s.repo.FindRefreshToken(ctx, tokenHash)
	if err != nil {
		return nil, ErrInvalidToken
	}

	_ = s.repo.RevokeRefreshToken(ctx, tokenHash)

	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if !user.Active {
		return nil, ErrAccountDisabled
	}

	return s.generateTokenPair(ctx, user)
}

func (s *Service) Logout(ctx context.Context, userID uuid.UUID) error {
	return s.repo.RevokeAllRefreshTokens(ctx, userID)
}

func (s *Service) VerifyEmail(ctx context.Context, req VerifyEmailRequest) error {
	ev, err := s.repo.FindEmailVerification(ctx, req.Token)
	if err != nil {
		return ErrInvalidToken
	}

	if err := s.repo.MarkEmailVerified(ctx, ev.UserID); err != nil {
		return fmt.Errorf("mark email verified: %w", err)
	}

	return nil
}

func (s *Service) ForgotPassword(ctx context.Context, req ForgotPasswordRequest) error {
	user, err := s.repo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil // do not leak whether the email exists
	}

	resetToken := generateSecureToken(32)
	return s.repo.StorePasswordReset(ctx, &PasswordReset{
		ID:        uuid.New(),
		UserID:    user.ID,
		Token:     resetToken,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})
}

func (s *Service) ResetPassword(ctx context.Context, req ResetPasswordRequest) error {
	pr, err := s.repo.FindPasswordReset(ctx, req.Token)
	if err != nil {
		return ErrInvalidToken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := s.repo.UpdatePassword(ctx, pr.UserID, string(hash)); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	_ = s.repo.RevokeAllRefreshTokens(ctx, pr.UserID)
	return nil
}

func (s *Service) GetProfile(ctx context.Context, userID uuid.UUID) (*User, error) {
	return s.repo.FindByID(ctx, userID)
}

func (s *Service) generateTokenPair(ctx context.Context, user *User) (*TokenPair, error) {
	now := time.Now()
	accessExpiry := now.Add(s.jwtConfig.AccessTokenTTL)

	accessClaims := &middleware.Claims{
		UserID: user.ID.String(),
		Email:  user.Email,
		Role:   user.Role.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    s.jwtConfig.Issuer,
			Subject:   user.ID.String(),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessString, err := accessToken.SignedString([]byte(s.jwtConfig.AccessTokenSecret))
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	refreshRaw := generateSecureToken(64)
	refreshExpiry := now.Add(s.jwtConfig.RefreshTokenTTL)
	refreshHash := HashToken(refreshRaw)

	if err := s.repo.StoreRefreshToken(ctx, user.ID, refreshHash, refreshExpiry); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessString,
		RefreshToken: refreshRaw,
		ExpiresAt:    accessExpiry,
	}, nil
}

func generateSecureToken(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
