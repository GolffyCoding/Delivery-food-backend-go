package support

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OrderSnapshotter fetches a lightweight, JSON-able snapshot of an order's current
// state (status, restaurant, driver, totals, timestamps) so a ticket carries that
// context permanently even if the order later changes state.
type OrderSnapshotter interface {
	Snapshot(ctx context.Context, orderID uuid.UUID) (map[string]interface{}, error)
}

// EventPublisher pushes a ticket event to whoever is watching the live support queue
// (see internal/websocket's "support:queue" room, admin-only).
type EventPublisher interface {
	PublishTicketEvent(ctx context.Context, eventType string, ticket interface{}) error
}

type Service struct {
	repo   Repository
	orders OrderSnapshotter
	events EventPublisher
}

func NewService(repo Repository, orders OrderSnapshotter, events EventPublisher) *Service {
	return &Service{repo: repo, orders: orders, events: events}
}

// categoriesNeedingUrgentAttention are food-in-hand situations where every extra
// minute matters (cold food, wrong/missing items) — they're auto-escalated to high
// priority instead of relying on the customer to know to flag it as urgent.
var categoriesNeedingUrgentAttention = map[Category]bool{
	CategoryMissingFood: true,
	CategoryWrongOrder:  true,
	CategoryDamagedFood: true,
}

type CreateTicketRequest struct {
	OrderID     *uuid.UUID `json:"order_id"`
	Category    Category   `json:"category" validate:"required,oneof=missing_food wrong_order damaged_food late_delivery payment driver_behavior other"`
	Subject     string     `json:"subject" validate:"required,max=200"`
	Description string     `json:"description" validate:"required,max=4000"`
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, req CreateTicketRequest) (*Ticket, error) {
	priority := PriorityNormal
	if categoriesNeedingUrgentAttention[req.Category] {
		priority = PriorityHigh
	}

	t := &Ticket{
		ID:          uuid.New(),
		UserID:      userID,
		OrderID:     req.OrderID,
		Category:    req.Category,
		Subject:     req.Subject,
		Description: req.Description,
		Status:      StatusOpen,
		Priority:    priority,
	}

	if req.OrderID != nil && s.orders != nil {
		if snap, err := s.orders.Snapshot(ctx, *req.OrderID); err == nil {
			t.OrderSnapshot = snap
		}
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("create support ticket: %w", err)
	}

	if s.events != nil {
		_ = s.events.PublishTicketEvent(ctx, "ticket.created", t)
	}

	return t, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Ticket, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID, page, perPage int) ([]Ticket, int64, error) {
	page, perPage = normalizePaging(page, perPage)
	return s.repo.ListByUser(ctx, userID, page, perPage)
}

func (s *Service) ListOpen(ctx context.Context, page, perPage int) ([]Ticket, int64, error) {
	page, perPage = normalizePaging(page, perPage)
	return s.repo.ListOpen(ctx, page, perPage)
}

// Assign claims a ticket for an admin working the queue, moving it to in_progress so
// it drops off other agents' "open" view instead of multiple agents duplicating work
// on the same complaint.
func (s *Service) Assign(ctx context.Context, ticketID, adminID uuid.UUID) (*Ticket, error) {
	t, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if t.Status == StatusResolved || t.Status == StatusClosed {
		return nil, ErrAlreadyResolved
	}

	t.AssignedAdminID = &adminID
	t.Status = StatusInProgress
	t.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	if s.events != nil {
		_ = s.events.PublishTicketEvent(ctx, "ticket.assigned", t)
	}
	return t, nil
}

type ResolveRequest struct {
	Resolution string `json:"resolution" validate:"required,max=4000"`
}

func (s *Service) Resolve(ctx context.Context, ticketID, adminID uuid.UUID, resolution string) (*Ticket, error) {
	t, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if t.Status == StatusResolved || t.Status == StatusClosed {
		return nil, ErrAlreadyResolved
	}

	now := time.Now()
	t.Status = StatusResolved
	t.Resolution = resolution
	t.ResolvedAt = &now
	t.UpdatedAt = now
	if t.AssignedAdminID == nil {
		t.AssignedAdminID = &adminID
	}

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	if s.events != nil {
		_ = s.events.PublishTicketEvent(ctx, "ticket.resolved", t)
	}
	return t, nil
}

func normalizePaging(page, perPage int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 50 {
		perPage = 20
	}
	return page, perPage
}
