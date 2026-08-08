package restaurant

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/opendelivery/opendelivery/pkg/response"
)

var (
	ErrRestaurantNotFound = errors.New("restaurant not found")
	ErrNotOwner           = errors.New("not the owner of this restaurant")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

type CreateRestaurantRequest struct {
	Name             string         `json:"name" validate:"required,min=1,max=200"`
	Description      string         `json:"description" validate:"max=2000"`
	Phone            string         `json:"phone" validate:"required"`
	Address          string         `json:"address" validate:"required"`
	Latitude         float64        `json:"latitude" validate:"required"`
	Longitude        float64        `json:"longitude" validate:"required"`
	DeliveryRadiusKm float64        `json:"delivery_radius_km" validate:"gte=0.5,lte=50"`
	MinimumOrder     float64        `json:"minimum_order" validate:"gte=0"`
	DeliveryFee      float64        `json:"delivery_fee" validate:"gte=0"`
	CuisineTypes     []string       `json:"cuisine_types" validate:"required,min=1"`
	OpeningHours     []OpeningHours `json:"opening_hours" validate:"required,min=7,max=7"`
}

type UpdateRestaurantRequest struct {
	Name             *string        `json:"name" validate:"omitempty,min=1,max=200"`
	Description      *string        `json:"description" validate:"omitempty,max=2000"`
	Phone            *string        `json:"phone"`
	Address          *string        `json:"address"`
	Latitude         *float64       `json:"latitude"`
	Longitude        *float64       `json:"longitude"`
	DeliveryRadiusKm *float64       `json:"delivery_radius_km" validate:"omitempty,gte=0.5,lte=50"`
	MinimumOrder     *float64       `json:"minimum_order" validate:"omitempty,gte=0"`
	DeliveryFee      *float64       `json:"delivery_fee" validate:"omitempty,gte=0"`
	CuisineTypes     []string       `json:"cuisine_types"`
	OpeningHours     []OpeningHours `json:"opening_hours"`
}

type NearbyRequest struct {
	Latitude  float64 `json:"latitude" validate:"required"`
	Longitude float64 `json:"longitude" validate:"required"`
	RadiusKm  float64 `json:"radius_km" validate:"required,gte=0.5,lte=50"`
	Limit     int     `json:"limit" validate:"omitempty,gte=1,lte=50"`
}

type SearchRequest struct {
	Query string `json:"query" validate:"required,min=1"`
}

func (s *Service) Create(ctx context.Context, merchantID uuid.UUID, req CreateRestaurantRequest) (*Restaurant, error) {
	rest := &Restaurant{
		ID:               uuid.New(),
		MerchantID:       merchantID,
		Name:             req.Name,
		Description:      req.Description,
		Phone:            req.Phone,
		Address:          req.Address,
		Latitude:         req.Latitude,
		Longitude:        req.Longitude,
		DeliveryRadiusKm: req.DeliveryRadiusKm,
		MinimumOrder:     req.MinimumOrder,
		DeliveryFee:      req.DeliveryFee,
		Status:           StatusActive,
		CuisineTypes:     req.CuisineTypes,
		OpeningHours:     req.OpeningHours,
	}

	if err := s.repo.Create(ctx, rest); err != nil {
		return nil, fmt.Errorf("create restaurant: %w", err)
	}

	return rest, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Restaurant, error) {
	rest, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrRestaurantNotFound
	}
	return rest, nil
}

func (s *Service) Update(ctx context.Context, id, merchantID uuid.UUID, req UpdateRestaurantRequest) (*Restaurant, error) {
	rest, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrRestaurantNotFound
	}
	if rest.MerchantID != merchantID {
		return nil, ErrNotOwner
	}

	if req.Name != nil {
		rest.Name = *req.Name
	}
	if req.Description != nil {
		rest.Description = *req.Description
	}
	if req.Phone != nil {
		rest.Phone = *req.Phone
	}
	if req.Address != nil {
		rest.Address = *req.Address
	}
	if req.Latitude != nil {
		rest.Latitude = *req.Latitude
	}
	if req.Longitude != nil {
		rest.Longitude = *req.Longitude
	}
	if req.DeliveryRadiusKm != nil {
		rest.DeliveryRadiusKm = *req.DeliveryRadiusKm
	}
	if req.MinimumOrder != nil {
		rest.MinimumOrder = *req.MinimumOrder
	}
	if req.DeliveryFee != nil {
		rest.DeliveryFee = *req.DeliveryFee
	}
	if req.CuisineTypes != nil {
		rest.CuisineTypes = req.CuisineTypes
	}
	if req.OpeningHours != nil {
		rest.OpeningHours = req.OpeningHours
	}

	if err := s.repo.Update(ctx, rest); err != nil {
		return nil, fmt.Errorf("update restaurant: %w", err)
	}

	return rest, nil
}

func (s *Service) Delete(ctx context.Context, id, merchantID uuid.UUID) error {
	rest, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ErrRestaurantNotFound
	}
	if rest.MerchantID != merchantID {
		return ErrNotOwner
	}
	return s.repo.SoftDelete(ctx, id)
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]Restaurant, *response.Meta, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PerPage <= 0 || filter.PerPage > 100 {
		filter.PerPage = 20
	}

	restaurants, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, nil, err
	}

	totalPages := int(math.Ceil(float64(total) / float64(filter.PerPage)))
	meta := &response.Meta{
		Page:       filter.Page,
		PerPage:    filter.PerPage,
		Total:      total,
		TotalPages: totalPages,
		HasMore:    filter.Page < totalPages,
	}

	return restaurants, meta, nil
}

func (s *Service) ListByMerchant(ctx context.Context, merchantID uuid.UUID) ([]Restaurant, error) {
	return s.repo.ListByMerchant(ctx, merchantID)
}

func (s *Service) ListFeatured(ctx context.Context) ([]Restaurant, error) {
	return s.repo.ListFeatured(ctx)
}

func (s *Service) ListNearby(ctx context.Context, req NearbyRequest) ([]Restaurant, error) {
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.repo.ListNearby(ctx, req.Latitude, req.Longitude, req.RadiusKm, limit)
}

func (s *Service) Search(ctx context.Context, req SearchRequest) ([]Restaurant, error) {
	return s.repo.Search(ctx, req.Query)
}

// UpdateRating implements review.RestaurantRatingUpdater.
func (s *Service) UpdateRating(ctx context.Context, restaurantID uuid.UUID, rating float64, count int) error {
	rest, err := s.repo.GetByID(ctx, restaurantID)
	if err != nil {
		return err
	}
	rest.Rating = rating
	rest.RatingCount = count
	return s.repo.Update(ctx, rest)
}

func (s *Service) CheckAvailability(ctx context.Context, id uuid.UUID) (bool, error) {
	exists, err := s.repo.ExistsByID(ctx, id)
	if err != nil || !exists {
		return false, ErrRestaurantNotFound
	}
	return s.repo.IsOpen(ctx, id)
}

// --- Adapter used by the order module to avoid a direct dependency on this package's Repository ---

type CheckerAdapter struct {
	repo Repository
}

func NewCheckerAdapter(repo Repository) *CheckerAdapter {
	return &CheckerAdapter{repo: repo}
}

func (a *CheckerAdapter) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	return a.repo.ExistsByID(ctx, id)
}

func (a *CheckerAdapter) IsOpen(ctx context.Context, id uuid.UUID) (bool, error) {
	return a.repo.IsOpen(ctx, id)
}

func (a *CheckerAdapter) GetDeliveryFee(ctx context.Context, id uuid.UUID) (float64, error) {
	r, err := a.repo.GetByID(ctx, id)
	if err != nil {
		return 0, err
	}
	return r.DeliveryFee, nil
}

func (a *CheckerAdapter) GetMinimumOrder(ctx context.Context, id uuid.UUID) (float64, error) {
	r, err := a.repo.GetByID(ctx, id)
	if err != nil {
		return 0, err
	}
	return r.MinimumOrder, nil
}
