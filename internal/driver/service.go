package driver

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/google/uuid"
)

type EventNotifier interface {
	NotifyDriver(ctx context.Context, driverID uuid.UUID, eventType string, data interface{}) error
}

type Service struct {
	repo     Repository
	cache    *LocationCache
	notifier EventNotifier
}

func NewService(repo Repository, cache *LocationCache, notifier EventNotifier) *Service {
	return &Service{repo: repo, cache: cache, notifier: notifier}
}

type RegisterDriverRequest struct {
	VehicleType  VehicleType `json:"vehicle_type" validate:"required,oneof=motorcycle car bicycle"`
	LicensePlate string      `json:"license_plate"`
	VehicleColor string      `json:"vehicle_color"`
}

type LocationUpdateRequest struct {
	Latitude  float64 `json:"latitude" validate:"required"`
	Longitude float64 `json:"longitude" validate:"required"`
}

type MatchResult struct {
	DriverID   uuid.UUID `json:"driver_id"`
	DistanceKm float64   `json:"distance_km"`
	Rating     float64   `json:"rating"`
	Score      float64   `json:"score"`
}

func (s *Service) Register(ctx context.Context, userID uuid.UUID, req RegisterDriverRequest) (*Driver, error) {
	d := &Driver{
		ID:           uuid.New(),
		UserID:       userID,
		VehicleType:  req.VehicleType,
		LicensePlate: req.LicensePlate,
		VehicleColor: req.VehicleColor,
		Status:       StatusOffline,
		Rating:       5.0,
		IsActive:     true,
	}
	if err := s.repo.Create(ctx, d); err != nil {
		return nil, fmt.Errorf("register driver: %w", err)
	}
	return d, nil
}

func (s *Service) GetProfile(ctx context.Context, userID uuid.UUID) (*Driver, error) {
	return s.repo.GetByUserID(ctx, userID)
}

func (s *Service) GoOnline(ctx context.Context, userID uuid.UUID) error {
	d, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.UpdateStatus(ctx, d.ID, StatusOnline)
}

func (s *Service) GoOffline(ctx context.Context, userID uuid.UUID) error {
	d, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateStatus(ctx, d.ID, StatusOffline); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.RemoveDriver(ctx, d.ID)
	}
	return nil
}

func (s *Service) UpdateLocation(ctx context.Context, userID uuid.UUID, req LocationUpdateRequest) error {
	d, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateLocation(ctx, d.ID, req.Latitude, req.Longitude); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.UpdateLocation(ctx, d.ID, req.Latitude, req.Longitude)
	}
	return nil
}

func (s *Service) GetEarnings(ctx context.Context, userID uuid.UUID) ([]Earning, float64, error) {
	d, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.GetEarnings(ctx, d.ID)
}

// FindNearestDriver ranks online drivers within searchRadiusKm by a weighted score of
// proximity (70%) and rating (30%), preferring the Redis GEO index when available and
// falling back to an in-memory Haversine scan over drivers loaded from Postgres.
func (s *Service) FindNearestDriver(ctx context.Context, lat, lng, searchRadiusKm float64) (*MatchResult, error) {
	if s.cache != nil {
		geoResults, err := s.cache.FindNearest(ctx, lat, lng, searchRadiusKm, 20)
		if err == nil && len(geoResults) > 0 {
			return s.scoreAndPick(ctx, geoResults, searchRadiusKm)
		}
	}

	drivers, err := s.repo.ListOnline(ctx)
	if err != nil {
		return nil, err
	}
	if len(drivers) == 0 {
		return nil, errors.New("no available drivers")
	}

	var candidates []MatchResult
	for _, d := range drivers {
		dist := haversine(lat, lng, d.CurrentLat, d.CurrentLng)
		if dist > searchRadiusKm {
			continue
		}
		candidates = append(candidates, buildMatch(d.ID, dist, d.Rating, searchRadiusKm))
	}
	if len(candidates) == 0 {
		return nil, errors.New("no available drivers")
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	return &candidates[0], nil
}

func (s *Service) scoreAndPick(ctx context.Context, geoResults []GeoResult, radiusKm float64) (*MatchResult, error) {
	var candidates []MatchResult
	for _, g := range geoResults {
		d, err := s.repo.GetByID(ctx, g.DriverID)
		if err != nil || d.Status != StatusOnline {
			continue
		}
		candidates = append(candidates, buildMatch(d.ID, g.DistKm, d.Rating, radiusKm))
	}
	if len(candidates) == 0 {
		return nil, errors.New("no available drivers")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	return &candidates[0], nil
}

func buildMatch(driverID uuid.UUID, distKm, rating, radiusKm float64) MatchResult {
	distanceScore := 1.0 - (distKm / radiusKm)
	if distanceScore < 0 {
		distanceScore = 0
	}
	ratingScore := rating / 5.0
	score := (distanceScore * 0.7) + (ratingScore * 0.3)
	return MatchResult{DriverID: driverID, DistanceKm: distKm, Rating: rating, Score: score}
}

// AutoAssign finds the best driver for a restaurant location and notifies them.
func (s *Service) AutoAssign(ctx context.Context, orderID uuid.UUID, restLat, restLng float64) (*MatchResult, error) {
	result, err := s.FindNearestDriver(ctx, restLat, restLng, 10.0)
	if err != nil {
		return nil, err
	}
	if s.notifier != nil {
		_ = s.notifier.NotifyDriver(ctx, result.DriverID, "order.assigned", map[string]interface{}{
			"order_id": orderID,
		})
	}
	return result, nil
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

func toRad(deg float64) float64 { return deg * math.Pi / 180 }
