package notification

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type Service struct {
	repo    Repository
	senders []Sender
}

func NewService(repo Repository, senders ...Sender) *Service {
	return &Service{repo: repo, senders: senders}
}

type SendRequest struct {
	UserID   uuid.UUID              `json:"user_id" validate:"required"`
	Type     Type                   `json:"type" validate:"required,oneof=push email sms in_app"`
	Title    string                 `json:"title" validate:"required,max=255"`
	Body     string                 `json:"body" validate:"required"`
	Data     map[string]interface{} `json:"data"`
	Priority Priority               `json:"priority" validate:"omitempty,oneof=low normal high"`
}

func (s *Service) Send(ctx context.Context, req SendRequest) (*Notification, error) {
	if req.Priority == "" {
		req.Priority = PriorityNormal
	}

	n := &Notification{
		ID:         uuid.New(),
		UserID:     req.UserID,
		Type:       req.Type,
		Title:      req.Title,
		Body:       req.Body,
		Data:       req.Data,
		Priority:   req.Priority,
		MaxRetries: 3,
	}

	if err := s.repo.Create(ctx, n); err != nil {
		return nil, fmt.Errorf("create notification: %w", err)
	}

	for _, sender := range s.senders {
		if !sender.CanHandle(req.Type) {
			continue
		}
		if err := sender.Send(ctx, n); err != nil {
			_ = s.repo.IncrementRetry(ctx, n.ID)
			continue
		}
		_ = s.repo.MarkSent(ctx, n.ID)
	}

	return n, nil
}

func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID, page, perPage int) ([]Notification, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 50 {
		perPage = 20
	}
	return s.repo.ListByUser(ctx, userID, page, perPage)
}

func (s *Service) UnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.repo.GetUnreadCount(ctx, userID)
}

func (s *Service) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.MarkRead(ctx, id, userID)
}
