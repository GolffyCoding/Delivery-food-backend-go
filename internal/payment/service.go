package payment

import (
	"context"

	"github.com/google/uuid"
)

type Service struct {
	repo      Repository
	providers map[Method]Provider
}

func NewService(repo Repository, providers ...Provider) *Service {
	pm := make(map[Method]Provider)
	for _, p := range providers {
		for _, m := range p.SupportedMethods() {
			pm[m] = p
		}
	}
	return &Service{repo: repo, providers: pm}
}

type Request struct {
	OrderID uuid.UUID `json:"order_id" validate:"required"`
	Amount  float64   `json:"amount" validate:"required,gt=0"`
	Method  Method    `json:"method" validate:"required"`
}

func (s *Service) ProcessPayment(ctx context.Context, req Request) (*Payment, error) {
	// Guard against duplicate submissions (double-tap, retried request) charging the
	// same order twice: a prior pending/processing/successful payment short-circuits
	// this call instead of racing the DB's unique(order_id) constraint into an
	// opaque 500.
	if existing, err := s.repo.GetByOrderID(ctx, req.OrderID); err == nil {
		if existing.Status == StatusSuccess || existing.Status == StatusAuthorized || existing.Status == StatusPending || existing.Status == StatusProcessing {
			return nil, ErrAlreadyPaid
		}
	}

	provider, ok := s.providers[req.Method]
	if !ok {
		return nil, ErrUnsupportedMethod
	}

	p := &Payment{
		ID:           uuid.New(),
		OrderID:      req.OrderID,
		Amount:       req.Amount,
		Method:       req.Method,
		Status:       StatusPending,
		ProviderName: provider.Name(),
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}

	result, err := provider.ProcessPayment(ctx, p)
	if err != nil {
		_ = s.repo.UpdateStatus(ctx, p.ID, StatusFailed)
		return nil, err
	}

	if err := s.repo.UpdateStatus(ctx, result.ID, result.Status); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*Payment, error) {
	return s.repo.GetByOrderID(ctx, orderID)
}

// AuthorizePayment places a hold for req.Amount without capturing it, so the caller
// can show the customer an explicit "confirm payment of X" step — the review theme
// was money moving with zero confirmation and no way to catch a mis-tap before it
// was too late.
func (s *Service) AuthorizePayment(ctx context.Context, req Request) (*Payment, error) {
	if existing, err := s.repo.GetByOrderID(ctx, req.OrderID); err == nil {
		if existing.Status == StatusSuccess || existing.Status == StatusAuthorized || existing.Status == StatusPending || existing.Status == StatusProcessing {
			return nil, ErrAlreadyPaid
		}
	}

	provider, ok := s.providers[req.Method]
	if !ok {
		return nil, ErrUnsupportedMethod
	}

	p := &Payment{
		ID:           uuid.New(),
		OrderID:      req.OrderID,
		Amount:       req.Amount,
		Method:       req.Method,
		Status:       StatusPending,
		ProviderName: provider.Name(),
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}

	result, err := provider.Authorize(ctx, p)
	if err != nil {
		_ = s.repo.UpdateStatus(ctx, p.ID, StatusFailed)
		return nil, err
	}

	if err := s.repo.UpdateStatus(ctx, result.ID, result.Status); err != nil {
		return nil, err
	}
	return result, nil
}

// ConfirmPayment captures a payment that was previously placed on hold via
// AuthorizePayment. Only the customer who owns the order should be able to trigger
// this — that check belongs to the handler, which has the authenticated user id.
func (s *Service) ConfirmPayment(ctx context.Context, paymentID uuid.UUID) (*Payment, error) {
	p, err := s.repo.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if p.Status != StatusAuthorized {
		return nil, ErrNotAuthorized
	}

	provider, ok := s.providers[p.Method]
	if !ok {
		return nil, ErrUnsupportedMethod
	}

	result, err := provider.Capture(ctx, p)
	if err != nil {
		_ = s.repo.UpdateStatus(ctx, p.ID, StatusFailed)
		return nil, err
	}
	if err := s.repo.UpdateStatus(ctx, result.ID, result.Status); err != nil {
		return nil, err
	}
	return result, nil
}

// VoidPayment releases a hold placed by AuthorizePayment without ever charging it —
// this is what backs the order cancellation flow for an authorized-but-unconfirmed
// payment.
func (s *Service) VoidPayment(ctx context.Context, paymentID uuid.UUID) (*Payment, error) {
	p, err := s.repo.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if p.Status != StatusAuthorized {
		return nil, ErrNotAuthorized
	}

	provider, ok := s.providers[p.Method]
	if !ok {
		return nil, ErrUnsupportedMethod
	}

	result, err := provider.Void(ctx, p)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateStatus(ctx, result.ID, result.Status); err != nil {
		return nil, err
	}
	return result, nil
}
