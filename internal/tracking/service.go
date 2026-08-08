package tracking

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
)

type LocationUpdate struct {
	DriverID  uuid.UUID `json:"driver_id"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Heading   float64   `json:"heading"`
	Speed     float64   `json:"speed"`
	Timestamp time.Time `json:"timestamp"`

	// Anomalous and ImpliedSpeedKmH are computed server-side (not client-supplied):
	// true when this ping implies physically impossible travel since the driver's
	// previous ping, so a support agent can see verified evidence instead of taking
	// "GPS was wrong" on faith.
	Anomalous       bool    `json:"anomalous,omitempty"`
	ImpliedSpeedKmH float64 `json:"implied_speed_kmh,omitempty"`
}

type ETAResult struct {
	EstimatedMin int     `json:"estimated_min"`
	DistanceKm   float64 `json:"distance_km"`
}

type DriverLocationRepo interface {
	UpdateLocation(ctx context.Context, driverID uuid.UUID, lat, lng float64) error
}

type EventPublisher interface {
	PublishDriverLocation(ctx context.Context, driverID uuid.UUID, data interface{}) error
	// PublishDispatchAlert notifies whoever is watching the live ops/support feed
	// (e.g. an admin dashboard subscribed to a "dispatch alerts" room) of a
	// server-verified anomaly, in real time.
	PublishDispatchAlert(ctx context.Context, data interface{}) error
}

type LocationCache interface {
	UpdateLocation(ctx context.Context, driverID uuid.UUID, lat, lng float64) error
}

type Service struct {
	driverRepo DriverLocationRepo
	events     EventPublisher
	cache      LocationCache
	lastLoc    LastLocationStore
}

func NewService(driverRepo DriverLocationRepo, events EventPublisher, cache LocationCache, lastLoc LastLocationStore) *Service {
	return &Service{driverRepo: driverRepo, events: events, cache: cache, lastLoc: lastLoc}
}

func (s *Service) UpdateDriverLocation(ctx context.Context, loc LocationUpdate) error {
	if err := s.driverRepo.UpdateLocation(ctx, loc.DriverID, loc.Latitude, loc.Longitude); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.UpdateLocation(ctx, loc.DriverID, loc.Latitude, loc.Longitude)
	}

	if s.lastLoc != nil {
		if lastLat, lastLng, lastTS, ok, err := s.lastLoc.Get(ctx, loc.DriverID); err == nil && ok {
			loc.Anomalous, loc.ImpliedSpeedKmH = checkAnomaly(lastLat, lastLng, lastTS, loc.Latitude, loc.Longitude, loc.Timestamp)
		}
		_ = s.lastLoc.Set(ctx, loc.DriverID, loc.Latitude, loc.Longitude, loc.Timestamp)
	}

	if s.events != nil {
		_ = s.events.PublishDriverLocation(ctx, loc.DriverID, loc)
		if loc.Anomalous {
			_ = s.events.PublishDispatchAlert(ctx, map[string]interface{}{
				"type":              "gps_anomaly",
				"driver_id":         loc.DriverID,
				"implied_speed_kmh": loc.ImpliedSpeedKmH,
				"latitude":          loc.Latitude,
				"longitude":         loc.Longitude,
				"timestamp":         loc.Timestamp,
			})
		}
	}
	return nil
}

func (s *Service) CalculateETA(driverLat, driverLng, destLat, destLng, avgSpeedKmH float64) ETAResult {
	if avgSpeedKmH <= 0 {
		avgSpeedKmH = 25
	}
	distance := haversine(driverLat, driverLng, destLat, destLng)
	minutes := int((distance / avgSpeedKmH) * 60)
	if minutes < 1 {
		minutes = 1
	}
	return ETAResult{EstimatedMin: minutes, DistanceKm: math.Round(distance*100) / 100}
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
