package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleCustomer Role = "customer"
	RoleDriver   Role = "driver"
	RoleMerchant Role = "merchant"
	RoleAdmin    Role = "admin"
)

func (r Role) IsValid() bool {
	switch r {
	case RoleCustomer, RoleDriver, RoleMerchant, RoleAdmin:
		return true
	}
	return false
}

func (r Role) String() string { return string(r) }

type User struct {
	ID            uuid.UUID  `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	Email         string     `json:"email" bun:",unique,notnull"`
	PasswordHash  string     `json:"-" bun:",notnull"`
	FirstName     string     `json:"first_name" bun:",notnull"`
	LastName      string     `json:"last_name" bun:",notnull"`
	Phone         string     `json:"phone"`
	AvatarURL     string     `json:"avatar_url"`
	Role          Role       `json:"role" bun:",notnull,default:'customer'"`
	EmailVerified bool       `json:"email_verified"`
	Active        bool       `json:"active"`
	CreatedAt     time.Time  `json:"created_at" bun:",nullzero,default:now()"`
	UpdatedAt     time.Time  `json:"updated_at" bun:",nullzero,default:now()"`
	DeletedAt     *time.Time `json:"-" bun:",soft_delete"`
}

func (u *User) TableName() string { return "users" }

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type EmailVerification struct {
	ID        uuid.UUID `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	UserID    uuid.UUID `json:"user_id" bun:",type:uuid,notnull"`
	Token     string    `json:"-" bun:",unique,notnull"`
	Used      bool      `json:"used"`
	ExpiresAt time.Time `json:"expires_at" bun:",notnull"`
	CreatedAt time.Time `json:"created_at" bun:",nullzero,default:now()"`
}

func (e *EmailVerification) TableName() string { return "email_verifications" }

type PasswordReset struct {
	ID        uuid.UUID `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	UserID    uuid.UUID `json:"user_id" bun:",type:uuid,notnull"`
	Token     string    `json:"-" bun:",unique,notnull"`
	Used      bool      `json:"used"`
	ExpiresAt time.Time `json:"expires_at" bun:",notnull"`
	CreatedAt time.Time `json:"created_at" bun:",nullzero,default:now()"`
}

func (p *PasswordReset) TableName() string { return "password_resets" }

var (
	ErrUserAlreadyExists  = errors.New("user with this email already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrAccountDisabled    = errors.New("account is disabled")
	ErrInvalidRole        = errors.New("invalid role")
	ErrTokenRevoked       = errors.New("token has been revoked")
)

type Repository interface {
	CreateUser(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	UpdateUser(ctx context.Context, user *User) error

	StoreRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	FindRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	RevokeAllRefreshTokens(ctx context.Context, userID uuid.UUID) error

	StoreEmailVerification(ctx context.Context, ev *EmailVerification) error
	FindEmailVerification(ctx context.Context, token string) (*EmailVerification, error)
	MarkEmailVerified(ctx context.Context, userID uuid.UUID) error

	StorePasswordReset(ctx context.Context, pr *PasswordReset) error
	FindPasswordReset(ctx context.Context, token string) (*PasswordReset, error)
	UpdatePassword(ctx context.Context, userID uuid.UUID, hash string) error
}
