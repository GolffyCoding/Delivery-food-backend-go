package payment

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	// StatusAuthorized means funds are held/reserved but not yet captured — the
	// customer had an explicit confirm step before money actually moves, addressing
	// reviews about payments going through with zero confirmation and no way to
	// back out of a mis-tap.
	StatusAuthorized Status = "authorized"
	StatusSuccess    Status = "success"
	StatusFailed     Status = "failed"
	StatusRefunded   Status = "refunded"
	StatusVoided     Status = "voided"
)

type Method string

const (
	MethodCash       Method = "cash"
	MethodCreditCard Method = "credit_card"
	MethodPromptPay  Method = "promptpay"
	MethodWallet     Method = "wallet"
)

type Payment struct {
	bun.BaseModel `bun:"table:payments"`

	ID             uuid.UUID `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	OrderID        uuid.UUID `json:"order_id" bun:",type:uuid,notnull,unique"`
	Amount         float64   `json:"amount" bun:",notnull"`
	Method         Method    `json:"method" bun:",notnull"`
	Status         Status    `json:"status" bun:",default:'pending'"`
	TransactionRef string    `json:"transaction_ref" bun:",unique"`
	ProviderName   string    `json:"provider_name"`
	CreatedAt      time.Time `json:"created_at" bun:",nullzero,default:now()"`
	UpdatedAt      time.Time `json:"updated_at" bun:",nullzero,default:now()"`
}

// Provider is implemented by each payment strategy (cash, card, PromptPay, wallet, ...).
type Provider interface {
	Name() string
	ProcessPayment(ctx context.Context, payment *Payment) (*Payment, error)
	// Authorize reserves funds without capturing them, letting the caller show the
	// customer an explicit confirmation step before money actually moves.
	Authorize(ctx context.Context, payment *Payment) (*Payment, error)
	// Capture completes a previously authorized payment.
	Capture(ctx context.Context, payment *Payment) (*Payment, error)
	// Void releases a hold placed by Authorize without ever capturing it.
	Void(ctx context.Context, payment *Payment) (*Payment, error)
	Refund(ctx context.Context, paymentID uuid.UUID, amount float64) (*Payment, error)
	SupportedMethods() []Method
}

type Repository interface {
	Create(ctx context.Context, p *Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*Payment, error)
	GetByOrderID(ctx context.Context, orderID uuid.UUID) (*Payment, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status Status) error
}

type PaymentError struct {
	Code    string
	Message string
}

func (e *PaymentError) Error() string { return e.Message }

var ErrUnsupportedMethod = &PaymentError{Code: "UNSUPPORTED_PAYMENT_METHOD", Message: "This payment method is not supported"}
var ErrPaymentNotFound = &PaymentError{Code: "PAYMENT_NOT_FOUND", Message: "Payment not found"}
var ErrAlreadyPaid = &PaymentError{Code: "ALREADY_PAID", Message: "This order has already been paid for"}
var ErrNotAuthorized = &PaymentError{Code: "NOT_AUTHORIZED", Message: "Payment must be authorized before it can be confirmed"}
