package order

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/opendelivery/opendelivery/pkg/response"
	"github.com/uptrace/bun"
)

type RestaurantChecker interface {
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	IsOpen(ctx context.Context, id uuid.UUID) (bool, error)
	GetDeliveryFee(ctx context.Context, id uuid.UUID) (float64, error)
	GetMinimumOrder(ctx context.Context, id uuid.UUID) (float64, error)
}

type CouponValidator interface {
	// Validate returns the discount amount to apply, or an error if the coupon cannot be used.
	Validate(ctx context.Context, code string, userID uuid.UUID, subtotal float64) (float64, error)
	// Redeem records that the coupon was used for a specific order, once the order
	// has actually been created (so a validated-but-abandoned order never burns usage).
	Redeem(ctx context.Context, code string, userID, orderID uuid.UUID) error
}

type EventPublisher interface {
	PublishOrderEvent(ctx context.Context, orderID uuid.UUID, eventType string, data interface{}) error
}

// Locker abstracts a distributed mutual-exclusion lock (e.g. Redis SETNX-based).
type Locker interface {
	Acquire(ctx context.Context, resource string, ttl time.Duration) (release func(context.Context) error, err error)
}

// OutboxAppender writes an outbox row inside the same DB transaction as the order
// insert, implementing the transactional outbox pattern (see internal/outbox).
type OutboxAppender interface {
	AppendInTx(ctx context.Context, tx bun.Tx, aggregate string, aggregateID uuid.UUID, eventType string, payload interface{}) error
}

type Service struct {
	repo              Repository
	restaurantChecker RestaurantChecker
	couponValidator   CouponValidator
	events            EventPublisher
	locker            Locker
	outbox            OutboxAppender
}

func NewService(repo Repository, rc RestaurantChecker, cv CouponValidator, events EventPublisher, locker Locker, outbox OutboxAppender) *Service {
	return &Service{
		repo:              repo,
		restaurantChecker: rc,
		couponValidator:   cv,
		events:            events,
		locker:            locker,
		outbox:            outbox,
	}
}

type ItemRequest struct {
	MenuItemID   uuid.UUID   `json:"menu_item_id" validate:"required"`
	Name         string      `json:"name" validate:"required"`
	Quantity     int         `json:"quantity" validate:"required,min=1,max=50"`
	UnitPrice    float64     `json:"unit_price" validate:"required,gte=0"`
	VariantName  string      `json:"variant_name"`
	VariantPrice float64     `json:"variant_price"`
	AddOns       []AddOnItem `json:"addons"`
	SpecialNotes string      `json:"special_notes"`
}

type CreateOrderRequest struct {
	RestaurantID    uuid.UUID     `json:"restaurant_id" validate:"required"`
	Items           []ItemRequest `json:"items" validate:"required,min=1"`
	CouponCode      string        `json:"coupon_code"`
	PaymentMethod   string        `json:"payment_method" validate:"required,oneof=cash credit_card promptpay wallet"`
	DeliveryAddress string        `json:"delivery_address" validate:"required,min=5"`
	DeliveryLat     float64       `json:"delivery_lat"`
	DeliveryLng     float64       `json:"delivery_lng"`
	SpecialNotes    string        `json:"special_notes" validate:"max=500"`
}

type UpdateStatusRequest struct {
	Status           Status `json:"status" validate:"required"`
	CancelReason     string `json:"cancel_reason" validate:"omitempty,max=500"`
	DeliveryPhotoURL string `json:"delivery_photo_url" validate:"omitempty,url"`
}

func (s *Service) Create(ctx context.Context, customerID uuid.UUID, req CreateOrderRequest) (*Order, error) {
	exists, err := s.restaurantChecker.Exists(ctx, req.RestaurantID)
	if err != nil || !exists {
		return nil, ErrRestaurantClosed
	}

	open, err := s.restaurantChecker.IsOpen(ctx, req.RestaurantID)
	if err != nil || !open {
		return nil, ErrRestaurantClosed
	}

	if len(req.Items) == 0 {
		return nil, ErrEmptyOrder
	}

	var items []OrderItem
	var subtotal float64

	for _, ir := range req.Items {
		var addOnTotal float64
		for _, a := range ir.AddOns {
			addOnTotal += a.Price * float64(a.Qty)
		}

		unitTotal := ir.UnitPrice + ir.VariantPrice
		itemTotal := (unitTotal + addOnTotal) * float64(ir.Quantity)
		subtotal += itemTotal

		items = append(items, OrderItem{
			ID:           uuid.New(),
			MenuItemID:   ir.MenuItemID,
			Name:         ir.Name,
			Quantity:     ir.Quantity,
			UnitPrice:    ir.UnitPrice,
			VariantName:  ir.VariantName,
			VariantPrice: ir.VariantPrice,
			AddOns:       ir.AddOns,
			TotalPrice:   math.Round(itemTotal*100) / 100,
			SpecialNotes: ir.SpecialNotes,
		})
	}

	minOrder, _ := s.restaurantChecker.GetMinimumOrder(ctx, req.RestaurantID)
	if subtotal < minOrder {
		return nil, ErrBelowMinimumOrder
	}

	deliveryFee, _ := s.restaurantChecker.GetDeliveryFee(ctx, req.RestaurantID)
	taxAmount := math.Round(subtotal*0.07*100) / 100

	discountAmount := 0.0
	if req.CouponCode != "" && s.couponValidator != nil {
		discount, err := s.couponValidator.Validate(ctx, req.CouponCode, customerID, subtotal)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidCoupon, err)
		}
		discountAmount = discount
	}

	total := subtotal + taxAmount + deliveryFee - discountAmount
	if total < 0 {
		total = 0
	}

	eta := time.Now().Add(45 * time.Minute)
	newOrder := &Order{
		ID:                uuid.New(),
		OrderNumber:       generateOrderNumber(),
		CustomerID:        customerID,
		RestaurantID:      req.RestaurantID,
		Status:            StatusPending,
		Items:             items,
		Subtotal:          math.Round(subtotal*100) / 100,
		TaxAmount:         taxAmount,
		DeliveryFee:       deliveryFee,
		DiscountAmount:    discountAmount,
		CouponCode:        req.CouponCode,
		TotalAmount:       math.Round(total*100) / 100,
		PaymentMethod:     req.PaymentMethod,
		PaymentStatus:     "pending",
		DeliveryAddress:   req.DeliveryAddress,
		DeliveryLat:       req.DeliveryLat,
		DeliveryLng:       req.DeliveryLng,
		SpecialNotes:      req.SpecialNotes,
		EstimatedDelivery: &eta,
		PickupCode:        generatePickupCode(),
		Version:           1,
	}

	var hook TxHook
	if s.outbox != nil {
		hook = func(ctx context.Context, tx bun.Tx) error {
			return s.outbox.AppendInTx(ctx, tx, "order", newOrder.ID, "created", newOrder)
		}
	}

	if err := s.repo.CreateWithHook(ctx, newOrder, hook); err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}

	if req.CouponCode != "" && discountAmount > 0 && s.couponValidator != nil {
		_ = s.couponValidator.Redeem(ctx, req.CouponCode, customerID, newOrder.ID)
	}

	if s.events != nil {
		_ = s.events.PublishOrderEvent(ctx, newOrder.ID, "order.created", newOrder)
	}

	return newOrder, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Order, error) {
	return s.repo.GetByID(ctx, id)
}

// Snapshot returns a JSON-able summary of an order's current state, satisfying
// support.OrderSnapshotter structurally (no import needed): it lets a support ticket
// permanently carry order context (status, totals, timestamps, who's assigned) even
// as the order itself keeps changing state after the ticket is filed.
func (s *Service) Snapshot(ctx context.Context, id uuid.UUID) (map[string]interface{}, error) {
	ord, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"order_number":       ord.OrderNumber,
		"status":             ord.Status,
		"restaurant_id":      ord.RestaurantID,
		"driver_id":          ord.DriverID,
		"total_amount":       ord.TotalAmount,
		"payment_method":     ord.PaymentMethod,
		"payment_status":     ord.PaymentStatus,
		"delivery_address":   ord.DeliveryAddress,
		"delivery_photo_url": ord.DeliveryPhotoURL,
		"estimated_delivery": ord.EstimatedDelivery,
		"created_at":         ord.CreatedAt,
	}, nil
}

func (s *Service) GetByOrderNumber(ctx context.Context, number string) (*Order, error) {
	return s.repo.GetByOrderNumber(ctx, number)
}

func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, req UpdateStatusRequest) (*Order, error) {
	ord, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrOrderNotFound
	}

	if err := ValidateTransition(ord.Status, req.Status); err != nil {
		return nil, err
	}

	now := time.Now()
	switch req.Status {
	case StatusAccepted:
		ord.AcceptedAt = &now
	case StatusReady:
		ord.PreparedAt = &now
	case StatusPickedUp:
		ord.PickedUpAt = &now
	case StatusDelivered:
		ord.DeliveredAt = &now
		ord.DeliveryPhotoURL = req.DeliveryPhotoURL
	case StatusCompleted:
		ord.CompletedAt = &now
	case StatusCancelled:
		ord.CancelledAt = &now
		ord.CancelReason = req.CancelReason
	}

	ord.Status = req.Status

	if err := s.repo.UpdateWithOptimisticLock(ctx, ord); err != nil {
		if errors.Is(err, ErrOptimisticLock) {
			return nil, ErrOptimisticLock
		}
		return nil, fmt.Errorf("update order status: %w", err)
	}

	if s.events != nil {
		_ = s.events.PublishOrderEvent(ctx, ord.ID, "order."+string(req.Status), ord)
	}

	return ord, nil
}

// AcceptOrder is used by drivers claiming a ready-for-pickup order. It uses a
// distributed lock keyed by the order id so two drivers cannot both win the race.
// pickupCode must match the order's generated code (shown to the restaurant/customer),
// which forces the driver to actually check they are taking the right order instead of
// grabbing whichever one looks like the best payout.
func (s *Service) AcceptOrder(ctx context.Context, orderID, driverID uuid.UUID, pickupCode string) (*Order, error) {
	if s.locker != nil {
		release, err := s.locker.Acquire(ctx, fmt.Sprintf("order-accept:%s", orderID), 10*time.Second)
		if err != nil {
			return nil, fmt.Errorf("another driver is accepting this order")
		}
		defer release(ctx)
	}

	ord, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return nil, ErrOrderNotFound
	}
	if ord.Status != StatusReady {
		return nil, ErrInvalidTransition
	}
	if ord.DriverID != nil {
		return nil, errors.New("order already has a driver assigned")
	}
	if pickupCode != ord.PickupCode {
		return nil, ErrPickupCodeMismatch
	}

	ord.DriverID = &driverID
	ord.Status = StatusPickedUp
	now := time.Now()
	ord.PickedUpAt = &now

	if err := s.repo.UpdateWithOptimisticLock(ctx, ord); err != nil {
		return nil, err
	}

	if s.events != nil {
		_ = s.events.PublishOrderEvent(ctx, ord.ID, "order.picked_up", ord)
	}

	return ord, nil
}

func (s *Service) Cancel(ctx context.Context, id uuid.UUID, reason string, customerID uuid.UUID) (*Order, error) {
	ord, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrOrderNotFound
	}
	if ord.CustomerID != customerID {
		return nil, ErrNotYourOrder
	}
	if ord.Status != StatusPending && ord.Status != StatusAccepted {
		return nil, ErrInvalidTransition
	}

	return s.UpdateStatus(ctx, id, UpdateStatusRequest{Status: StatusCancelled, CancelReason: reason})
}

func (s *Service) ListByCustomer(ctx context.Context, customerID uuid.UUID, page, perPage int) ([]Order, *response.Meta, error) {
	page, perPage = normalizePaging(page, perPage)
	orders, total, err := s.repo.ListByCustomer(ctx, customerID, page, perPage)
	if err != nil {
		return nil, nil, err
	}
	return orders, buildMeta(page, perPage, total), nil
}

func (s *Service) ListByRestaurant(ctx context.Context, restaurantID uuid.UUID, status *Status, page, perPage int) ([]Order, *response.Meta, error) {
	page, perPage = normalizePaging(page, perPage)
	orders, total, err := s.repo.ListByRestaurant(ctx, restaurantID, status, page, perPage)
	if err != nil {
		return nil, nil, err
	}
	return orders, buildMeta(page, perPage, total), nil
}

func (s *Service) ListByDriver(ctx context.Context, driverID uuid.UUID, page, perPage int) ([]Order, *response.Meta, error) {
	page, perPage = normalizePaging(page, perPage)
	orders, total, err := s.repo.ListByDriver(ctx, driverID, page, perPage)
	if err != nil {
		return nil, nil, err
	}
	return orders, buildMeta(page, perPage, total), nil
}

func (s *Service) GetPendingOrders(ctx context.Context, restaurantID uuid.UUID) ([]Order, error) {
	return s.repo.GetPendingOrders(ctx, restaurantID)
}

func (s *Service) CountByStatus(ctx context.Context, restaurantID uuid.UUID) (map[Status]int64, error) {
	return s.repo.CountByStatus(ctx, restaurantID)
}

// GetSalesReport defaults to the trailing 30 days when from/to are zero values.
func (s *Service) GetSalesReport(ctx context.Context, restaurantID uuid.UUID, from, to time.Time) (*SalesReport, error) {
	if to.IsZero() {
		to = time.Now()
	}
	if from.IsZero() {
		from = to.AddDate(0, 0, -30)
	}
	// `to` is treated as exclusive upper bound; bump it to the end of the given day
	// so a caller passing e.g. "2026-08-08" as both from and to gets that whole day.
	to = time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, to.Location()).Add(time.Second)
	return s.repo.GetSalesReport(ctx, restaurantID, from, to)
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

func buildMeta(page, perPage int, total int64) *response.Meta {
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	return &response.Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
		HasMore:    page < totalPages,
	}
}

func generateOrderNumber() string {
	now := time.Now()
	return fmt.Sprintf("OD-%s-%06d", now.Format("20060102"), now.UnixNano()%1000000)
}

// generatePickupCode returns a short human-readable code a driver reads off the order
// and enters to confirm they're claiming the right delivery.
func generatePickupCode() string {
	return fmt.Sprintf("%04d", time.Now().UnixNano()%10000)
}
