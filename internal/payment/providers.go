package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/google/uuid"
)

// CashProvider marks orders paid on delivery; no external gateway involved.
type CashProvider struct{}

func NewCashProvider() *CashProvider { return &CashProvider{} }

func (p *CashProvider) Name() string { return "cash" }

func (p *CashProvider) ProcessPayment(ctx context.Context, payment *Payment) (*Payment, error) {
	payment.Status = StatusSuccess
	payment.TransactionRef = "CASH-" + randomRef()
	return payment, nil
}

// Authorize has nothing to hold for cash-on-delivery; it just records the intent so
// the same authorize -> confirm flow works uniformly across payment methods.
func (p *CashProvider) Authorize(ctx context.Context, payment *Payment) (*Payment, error) {
	payment.Status = StatusAuthorized
	payment.TransactionRef = "CASH-" + randomRef()
	return payment, nil
}

func (p *CashProvider) Capture(ctx context.Context, payment *Payment) (*Payment, error) {
	payment.Status = StatusSuccess
	return payment, nil
}

func (p *CashProvider) Void(ctx context.Context, payment *Payment) (*Payment, error) {
	payment.Status = StatusVoided
	return payment, nil
}

func (p *CashProvider) Refund(ctx context.Context, paymentID uuid.UUID, amount float64) (*Payment, error) {
	return nil, nil
}

func (p *CashProvider) SupportedMethods() []Method { return []Method{MethodCash} }

// MockGatewayProvider simulates a card/PromptPay/wallet gateway for card, PromptPay
// and wallet methods without calling out to a real network in this environment.
type MockGatewayProvider struct {
	name    string
	methods []Method
}

func NewMockGatewayProvider(name string, methods ...Method) *MockGatewayProvider {
	return &MockGatewayProvider{name: name, methods: methods}
}

func (p *MockGatewayProvider) Name() string { return p.name }

func (p *MockGatewayProvider) ProcessPayment(ctx context.Context, payment *Payment) (*Payment, error) {
	payment.Status = StatusSuccess
	payment.TransactionRef = p.name + "-" + randomRef()
	return payment, nil
}

// Authorize simulates placing a hold on the customer's funds without capturing them.
func (p *MockGatewayProvider) Authorize(ctx context.Context, payment *Payment) (*Payment, error) {
	payment.Status = StatusAuthorized
	payment.TransactionRef = p.name + "-hold-" + randomRef()
	return payment, nil
}

func (p *MockGatewayProvider) Capture(ctx context.Context, payment *Payment) (*Payment, error) {
	payment.Status = StatusSuccess
	return payment, nil
}

func (p *MockGatewayProvider) Void(ctx context.Context, payment *Payment) (*Payment, error) {
	payment.Status = StatusVoided
	return payment, nil
}

func (p *MockGatewayProvider) Refund(ctx context.Context, paymentID uuid.UUID, amount float64) (*Payment, error) {
	return nil, nil
}

func (p *MockGatewayProvider) SupportedMethods() []Method { return p.methods }

func randomRef() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
