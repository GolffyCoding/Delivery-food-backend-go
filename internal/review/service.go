package review

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrAlreadyReviewed = errors.New("order already reviewed")

// RestaurantRatingUpdater lets the review service push aggregate rating updates
// back onto a restaurant without importing the restaurant package directly.
type RestaurantRatingUpdater interface {
	UpdateRating(ctx context.Context, restaurantID uuid.UUID, rating float64, count int) error
}

type Service struct {
	repo        Repository
	restaurants RestaurantRatingUpdater
}

func NewService(repo Repository, restaurants RestaurantRatingUpdater) *Service {
	return &Service{repo: repo, restaurants: restaurants}
}

type CreateReviewRequest struct {
	OrderID      uuid.UUID  `json:"order_id" validate:"required"`
	RestaurantID uuid.UUID  `json:"restaurant_id" validate:"required"`
	DriverID     *uuid.UUID `json:"driver_id"`
	Rating       int        `json:"rating" validate:"required,min=1,max=5"`
	Comment      string     `json:"comment" validate:"max=2000"`
	PhotoURLs    []string   `json:"photo_urls"`
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, req CreateReviewRequest) (*Review, error) {
	already, err := s.repo.HasReviewed(ctx, req.OrderID)
	if err != nil {
		return nil, err
	}
	if already {
		return nil, ErrAlreadyReviewed
	}

	rv := &Review{
		ID:           uuid.New(),
		UserID:       userID,
		OrderID:      req.OrderID,
		RestaurantID: req.RestaurantID,
		DriverID:     req.DriverID,
		Rating:       req.Rating,
		Comment:      req.Comment,
		PhotoURLs:    req.PhotoURLs,
	}
	if err := s.repo.Create(ctx, rv); err != nil {
		return nil, fmt.Errorf("create review: %w", err)
	}

	if s.restaurants != nil {
		if summary, err := s.repo.GetRestaurantSummary(ctx, req.RestaurantID); err == nil {
			_ = s.restaurants.UpdateRating(ctx, req.RestaurantID, summary.Average, summary.Count)
		}
	}

	return rv, nil
}

func (s *Service) ListByRestaurant(ctx context.Context, restaurantID uuid.UUID, page, perPage int) ([]Review, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 50 {
		perPage = 20
	}
	return s.repo.ListByRestaurant(ctx, restaurantID, page, perPage)
}

func (s *Service) GetRestaurantSummary(ctx context.Context, restaurantID uuid.UUID) (*RatingSummary, error) {
	return s.repo.GetRestaurantSummary(ctx, restaurantID)
}

func (s *Service) Reply(ctx context.Context, reviewID, restaurantID uuid.UUID, reply string) error {
	return s.repo.AddReply(ctx, reviewID, restaurantID, reply)
}
