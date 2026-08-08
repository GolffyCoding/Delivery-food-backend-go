package wallet

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetOrCreate(ctx context.Context, userID uuid.UUID) (*Wallet, error) {
	w, err := s.repo.GetByUserID(ctx, userID)
	if err == nil {
		return w, nil
	}
	if !errors.Is(err, ErrWalletNotFound) {
		return nil, err
	}
	return s.repo.CreateWallet(ctx, userID)
}

type TopUpRequest struct {
	Amount float64 `json:"amount" validate:"required,gt=0"`
}

func (s *Service) TopUp(ctx context.Context, userID uuid.UUID, req TopUpRequest) (*Transaction, error) {
	w, err := s.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.Credit(ctx, w.ID, req.Amount, TxTopUp, nil, "wallet top-up")
}

func (s *Service) Pay(ctx context.Context, userID uuid.UUID, amount float64, orderID uuid.UUID) (*Transaction, error) {
	w, err := s.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.Debit(ctx, w.ID, amount, TxPayment, &orderID, "order payment")
}

func (s *Service) CreditEarning(ctx context.Context, userID uuid.UUID, amount float64, orderID uuid.UUID) (*Transaction, error) {
	w, err := s.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.Credit(ctx, w.ID, amount, TxEarning, &orderID, "delivery earning")
}

func (s *Service) ListTransactions(ctx context.Context, userID uuid.UUID, page, perPage int) ([]Transaction, int64, error) {
	w, err := s.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 50 {
		perPage = 20
	}
	return s.repo.ListTransactions(ctx, w.ID, page, perPage)
}
