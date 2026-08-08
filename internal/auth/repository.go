package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
)

type postgresRepository struct {
	db  *bun.DB
	rdb *redis.Client
}

func NewPostgresRepository(db *bun.DB, rdb *redis.Client) Repository {
	return &postgresRepository{db: db, rdb: rdb}
}

func (r *postgresRepository) CreateUser(ctx context.Context, user *User) error {
	_, err := r.db.NewInsert().Model(user).Exec(ctx)
	return err
}

func (r *postgresRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	user := &User{}
	err := r.db.NewSelect().
		Model(user).
		Where("email = ? AND deleted_at IS NULL", email).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("find user by email: %w", err)
	}
	return user, nil
}

func (r *postgresRepository) FindByID(ctx context.Context, id uuid.UUID) (*User, error) {
	user := &User{}
	err := r.db.NewSelect().
		Model(user).
		Where("id = ? AND deleted_at IS NULL", id).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return user, nil
}

func (r *postgresRepository) UpdateUser(ctx context.Context, user *User) error {
	user.UpdatedAt = time.Now()
	_, err := r.db.NewUpdate().Model(user).WherePK().Exec(ctx)
	return err
}

func (r *postgresRepository) StoreRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	key := fmt.Sprintf("refresh:%s", tokenHash)
	ttl := time.Until(expiresAt)

	pipe := r.rdb.Pipeline()
	pipe.Set(ctx, key, userID.String(), ttl)
	setKey := fmt.Sprintf("refresh-set:%s", userID)
	pipe.SAdd(ctx, setKey, tokenHash)
	pipe.Expire(ctx, setKey, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *postgresRepository) FindRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	key := fmt.Sprintf("refresh:%s", tokenHash)
	val, err := r.rdb.Get(ctx, key).Result()
	if err != nil {
		return uuid.Nil, ErrTokenRevoked
	}
	return uuid.Parse(val)
}

func (r *postgresRepository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	key := fmt.Sprintf("refresh:%s", tokenHash)
	return r.rdb.Del(ctx, key).Err()
}

func (r *postgresRepository) RevokeAllRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	// Refresh tokens are opaque and keyed by hash, not user id, so we keep a
	// secondary set per user to allow bulk revocation.
	setKey := fmt.Sprintf("refresh-set:%s", userID)
	hashes, err := r.rdb.SMembers(ctx, setKey).Result()
	if err != nil {
		return nil
	}
	if len(hashes) == 0 {
		return nil
	}
	keys := make([]string, 0, len(hashes))
	for _, h := range hashes {
		keys = append(keys, fmt.Sprintf("refresh:%s", h))
	}
	pipe := r.rdb.Pipeline()
	pipe.Del(ctx, keys...)
	pipe.Del(ctx, setKey)
	_, err = pipe.Exec(ctx)
	return err
}

func (r *postgresRepository) StoreEmailVerification(ctx context.Context, ev *EmailVerification) error {
	_, err := r.db.NewInsert().Model(ev).Exec(ctx)
	return err
}

func (r *postgresRepository) FindEmailVerification(ctx context.Context, token string) (*EmailVerification, error) {
	ev := &EmailVerification{}
	err := r.db.NewSelect().
		Model(ev).
		Where("token = ? AND used = false AND expires_at > NOW()", token).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("find email verification: %w", err)
	}
	return ev, nil
}

func (r *postgresRepository) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.NewUpdate().
		Model((*User)(nil)).
		Set("email_verified = true").
		Where("id = ?", userID).
		Exec(ctx)
	return err
}

func (r *postgresRepository) StorePasswordReset(ctx context.Context, pr *PasswordReset) error {
	_, err := r.db.NewInsert().Model(pr).Exec(ctx)
	return err
}

func (r *postgresRepository) FindPasswordReset(ctx context.Context, token string) (*PasswordReset, error) {
	pr := &PasswordReset{}
	err := r.db.NewSelect().
		Model(pr).
		Where("token = ? AND used = false AND expires_at > NOW()", token).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("find password reset: %w", err)
	}
	return pr, nil
}

func (r *postgresRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, hash string) error {
	_, err := r.db.NewUpdate().
		Model((*User)(nil)).
		Set("password_hash = ?", hash).
		Where("id = ?", userID).
		Exec(ctx)
	return err
}

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
