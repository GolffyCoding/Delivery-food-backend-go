package wallet

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type TransactionType string

const (
	TxTopUp      TransactionType = "top_up"
	TxPayment    TransactionType = "payment"
	TxRefund     TransactionType = "refund"
	TxEarning    TransactionType = "earning"
	TxWithdrawal TransactionType = "withdrawal"
	TxBonus      TransactionType = "bonus"
)

type TransactionStatus string

const (
	TxStatusSuccess TransactionStatus = "success"
	TxStatusPending TransactionStatus = "pending"
	TxStatusFailed  TransactionStatus = "failed"
)

type Wallet struct {
	bun.BaseModel `bun:"table:wallets"`

	ID        uuid.UUID `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	UserID    uuid.UUID `json:"user_id" bun:",type:uuid,unique,notnull"`
	Balance   float64   `json:"balance"`
	IsActive  bool      `json:"is_active" bun:",default:true"`
	CreatedAt time.Time `json:"created_at" bun:",nullzero,default:now()"`
	UpdatedAt time.Time `json:"updated_at" bun:",nullzero,default:now()"`
}

type Transaction struct {
	bun.BaseModel `bun:"table:wallet_transactions"`

	ID            uuid.UUID         `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	WalletID      uuid.UUID         `json:"wallet_id" bun:",type:uuid,notnull"`
	Type          TransactionType   `json:"type" bun:",notnull"`
	Amount        float64           `json:"amount" bun:",notnull"`
	BalanceBefore float64           `json:"balance_before" bun:",notnull"`
	BalanceAfter  float64           `json:"balance_after" bun:",notnull"`
	Status        TransactionStatus `json:"status" bun:",default:'success'"`
	ReferenceID   *uuid.UUID        `json:"reference_id" bun:",type:uuid"`
	ReferenceType string            `json:"reference_type"`
	Description   string            `json:"description"`
	CreatedAt     time.Time         `json:"created_at" bun:",nullzero,default:now()"`
}

var (
	ErrWalletNotFound     = errors.New("wallet not found")
	ErrInsufficientBalance = errors.New("insufficient wallet balance")
)

type Repository interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*Wallet, error)
	CreateWallet(ctx context.Context, userID uuid.UUID) (*Wallet, error)
	Credit(ctx context.Context, walletID uuid.UUID, amount float64, txType TransactionType, refID *uuid.UUID, desc string) (*Transaction, error)
	Debit(ctx context.Context, walletID uuid.UUID, amount float64, txType TransactionType, refID *uuid.UUID, desc string) (*Transaction, error)
	ListTransactions(ctx context.Context, walletID uuid.UUID, page, perPage int) ([]Transaction, int64, error)
}
