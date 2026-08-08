package menu

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

type CreateCategoryRequest struct {
	Name         string `json:"name" validate:"required,min=1,max=100"`
	DisplayOrder int    `json:"display_order"`
}

type CreateItemRequest struct {
	CategoryID  *uuid.UUID `json:"category_id"`
	Name        string     `json:"name" validate:"required,min=1,max=200"`
	Description string     `json:"description" validate:"max=2000"`
	BasePrice   float64    `json:"base_price" validate:"required,gt=0"`
	ImageURL    string     `json:"image_url"`
	PrepTimeMin int        `json:"prep_time_min" validate:"gte=0"`
	PrepTimeMax int        `json:"prep_time_max" validate:"gte=0"`
}

type UpdateItemRequest struct {
	Name        *string  `json:"name" validate:"omitempty,min=1,max=200"`
	Description *string  `json:"description"`
	BasePrice   *float64 `json:"base_price" validate:"omitempty,gt=0"`
	ImageURL    *string  `json:"image_url"`
	IsActive    *bool    `json:"is_active"`
}

type CreateVariantRequest struct {
	Name  string  `json:"name" validate:"required,min=1,max=100"`
	Price float64 `json:"price" validate:"required,gte=0"`
}

type CreateAddOnRequest struct {
	Name        string  `json:"name" validate:"required,min=1,max=100"`
	Price       float64 `json:"price" validate:"required,gte=0"`
	MaxQuantity int     `json:"max_quantity" validate:"gte=1"`
}

func (s *Service) ListCategories(ctx context.Context, restaurantID uuid.UUID) ([]Category, error) {
	return s.repo.ListCategories(ctx, restaurantID)
}

func (s *Service) CreateCategory(ctx context.Context, restaurantID uuid.UUID, req CreateCategoryRequest) (*Category, error) {
	cat := &Category{
		ID:           uuid.New(),
		RestaurantID: restaurantID,
		Name:         req.Name,
		DisplayOrder: req.DisplayOrder,
		IsActive:     true,
	}
	if err := s.repo.CreateCategory(ctx, cat); err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}
	return cat, nil
}

func (s *Service) DeleteCategory(ctx context.Context, id, restaurantID uuid.UUID) error {
	return s.repo.DeleteCategory(ctx, id, restaurantID)
}

func (s *Service) ListItems(ctx context.Context, restaurantID uuid.UUID, activeOnly bool) ([]Item, error) {
	return s.repo.ListItems(ctx, restaurantID, activeOnly)
}

func (s *Service) GetItem(ctx context.Context, id uuid.UUID) (*Item, error) {
	return s.repo.GetItem(ctx, id)
}

func (s *Service) CreateItem(ctx context.Context, restaurantID uuid.UUID, req CreateItemRequest) (*Item, error) {
	item := &Item{
		ID:           uuid.New(),
		RestaurantID: restaurantID,
		CategoryID:   req.CategoryID,
		Name:         req.Name,
		Description:  req.Description,
		BasePrice:    req.BasePrice,
		ImageURL:     req.ImageURL,
		IsActive:     true,
		PrepTimeMin:  req.PrepTimeMin,
		PrepTimeMax:  req.PrepTimeMax,
	}
	if item.PrepTimeMin == 0 {
		item.PrepTimeMin = 10
	}
	if item.PrepTimeMax == 0 {
		item.PrepTimeMax = 20
	}
	if err := s.repo.CreateItem(ctx, item); err != nil {
		return nil, fmt.Errorf("create item: %w", err)
	}
	return item, nil
}

func (s *Service) UpdateItem(ctx context.Context, id, restaurantID uuid.UUID, req UpdateItemRequest) (*Item, error) {
	item, err := s.repo.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.RestaurantID != restaurantID {
		return nil, ErrNotFound
	}

	if req.Name != nil {
		item.Name = *req.Name
	}
	if req.Description != nil {
		item.Description = *req.Description
	}
	if req.BasePrice != nil {
		item.BasePrice = *req.BasePrice
	}
	if req.ImageURL != nil {
		item.ImageURL = *req.ImageURL
	}
	if req.IsActive != nil {
		item.IsActive = *req.IsActive
	}

	if err := s.repo.UpdateItem(ctx, item); err != nil {
		return nil, fmt.Errorf("update item: %w", err)
	}
	return item, nil
}

func (s *Service) DeleteItem(ctx context.Context, id, restaurantID uuid.UUID) error {
	return s.repo.DeleteItem(ctx, id, restaurantID)
}

func (s *Service) ListVariants(ctx context.Context, itemID uuid.UUID) ([]Variant, error) {
	return s.repo.ListVariants(ctx, itemID)
}

func (s *Service) CreateVariant(ctx context.Context, itemID uuid.UUID, req CreateVariantRequest) (*Variant, error) {
	v := &Variant{ID: uuid.New(), ItemID: itemID, Name: req.Name, Price: req.Price}
	if err := s.repo.CreateVariant(ctx, v); err != nil {
		return nil, fmt.Errorf("create variant: %w", err)
	}
	return v, nil
}

func (s *Service) DeleteVariant(ctx context.Context, id, itemID uuid.UUID) error {
	return s.repo.DeleteVariant(ctx, id, itemID)
}

func (s *Service) ListAddOns(ctx context.Context, itemID uuid.UUID) ([]AddOn, error) {
	return s.repo.ListAddOns(ctx, itemID)
}

func (s *Service) CreateAddOn(ctx context.Context, itemID uuid.UUID, req CreateAddOnRequest) (*AddOn, error) {
	maxQty := req.MaxQuantity
	if maxQty <= 0 {
		maxQty = 5
	}
	a := &AddOn{ID: uuid.New(), ItemID: itemID, Name: req.Name, Price: req.Price, MaxQuantity: maxQty}
	if err := s.repo.CreateAddOn(ctx, a); err != nil {
		return nil, fmt.Errorf("create addon: %w", err)
	}
	return a, nil
}

func (s *Service) DeleteAddOn(ctx context.Context, id, itemID uuid.UUID) error {
	return s.repo.DeleteAddOn(ctx, id, itemID)
}
