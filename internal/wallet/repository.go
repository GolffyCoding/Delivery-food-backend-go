package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type postgresRepository struct {
	db *bun.DB
}

func NewPostgresRepository(db *bun.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*Wallet, error) {
	w := &Wallet{}
	err := r.db.NewSelect().Model(w).Where("user_id = ?", userID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, fmt.Errorf("get wallet: %w", err)
	}
	return w, nil
}

func (r *postgresRepository) CreateWallet(ctx context.Context, userID uuid.UUID) (*Wallet, error) {
	w := &Wallet{ID: uuid.New(), UserID: userID, Balance: 0, IsActive: true}
	if _, err := r.db.NewInsert().Model(w).Exec(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

// Credit and Debit use SELECT ... FOR UPDATE inside a transaction so concurrent
// balance changes on the same wallet serialize instead of racing on a stale read.

func (r *postgresRepository) Credit(ctx context.Context, walletID uuid.UUID, amount float64, txType TransactionType, refID *uuid.UUID, desc string) (*Transaction, error) {
	var tx *Transaction
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, dbtx bun.Tx) error {
		w := &Wallet{}
		if err := dbtx.NewSelect().Model(w).Where("id = ?", walletID).For("UPDATE").Scan(ctx); err != nil {
			return err
		}

		before := w.Balance
		after := before + amount

		if _, err := dbtx.NewUpdate().Model((*Wallet)(nil)).
			Set("balance = ?", after).
			Set("updated_at = NOW()").
			Where("id = ?", walletID).
			Exec(ctx); err != nil {
			return err
		}

		tx = &Transaction{
			ID:            uuid.New(),
			WalletID:      walletID,
			Type:          txType,
			Amount:        amount,
			BalanceBefore: before,
			BalanceAfter:  after,
			Status:        TxStatusSuccess,
			ReferenceID:   refID,
			Description:   desc,
		}
		_, err := dbtx.NewInsert().Model(tx).Exec(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (r *postgresRepository) Debit(ctx context.Context, walletID uuid.UUID, amount float64, txType TransactionType, refID *uuid.UUID, desc string) (*Transaction, error) {
	var tx *Transaction
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, dbtx bun.Tx) error {
		w := &Wallet{}
		if err := dbtx.NewSelect().Model(w).Where("id = ?", walletID).For("UPDATE").Scan(ctx); err != nil {
			return err
		}

		if w.Balance < amount {
			return ErrInsufficientBalance
		}

		before := w.Balance
		after := before - amount

		if _, err := dbtx.NewUpdate().Model((*Wallet)(nil)).
			Set("balance = ?", after).
			Set("updated_at = NOW()").
			Where("id = ?", walletID).
			Exec(ctx); err != nil {
			return err
		}

		tx = &Transaction{
			ID:            uuid.New(),
			WalletID:      walletID,
			Type:          txType,
			Amount:        amount,
			BalanceBefore: before,
			BalanceAfter:  after,
			Status:        TxStatusSuccess,
			ReferenceID:   refID,
			Description:   desc,
		}
		_, err := dbtx.NewInsert().Model(tx).Exec(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (r *postgresRepository) ListTransactions(ctx context.Context, walletID uuid.UUID, page, perPage int) ([]Transaction, int64, error) {
	var txs []Transaction
	query := r.db.NewSelect().Model(&txs).Where("wallet_id = ?", walletID)

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Order("created_at DESC").Limit(perPage).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, err
	}
	return txs, int64(total), nil
}
