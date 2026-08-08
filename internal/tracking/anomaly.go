package tracking

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Reviews repeatedly describe riders whose route visibly detours to another
// address/restaurant, with support dismissing the complaint as "a GPS problem" because
// there was never any server-side record to check against. LastLocationStore keeps the
// previous ping per driver so the service can compute an implied speed and flag
// physically-impossible jumps (teleporting between pings) instead of trusting every
// update blindly.
type LastLocationStore interface {
	Get(ctx context.Context, driverID uuid.UUID) (lat, lng float64, ts time.Time, ok bool, err error)
	Set(ctx context.Context, driverID uuid.UUID, lat, lng float64, ts time.Time) error
}

type redisLastLocationStore struct {
	rdb *redis.Client
}

func NewRedisLastLocationStore(rdb *redis.Client) LastLocationStore {
	return &redisLastLocationStore{rdb: rdb}
}

type storedLocation struct {
	Lat float64   `json:"lat"`
	Lng float64   `json:"lng"`
	TS  time.Time `json:"ts"`
}

func (s *redisLastLocationStore) key(driverID uuid.UUID) string {
	return "driver:last_location:" + driverID.String()
}

func (s *redisLastLocationStore) Get(ctx context.Context, driverID uuid.UUID) (float64, float64, time.Time, bool, error) {
	raw, err := s.rdb.Get(ctx, s.key(driverID)).Result()
	if err == redis.Nil {
		return 0, 0, time.Time{}, false, nil
	}
	if err != nil {
		return 0, 0, time.Time{}, false, err
	}
	var loc storedLocation
	if err := json.Unmarshal([]byte(raw), &loc); err != nil {
		return 0, 0, time.Time{}, false, err
	}
	return loc.Lat, loc.Lng, loc.TS, true, nil
}

func (s *redisLastLocationStore) Set(ctx context.Context, driverID uuid.UUID, lat, lng float64, ts time.Time) error {
	data, err := json.Marshal(storedLocation{Lat: lat, Lng: lng, TS: ts})
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.key(driverID), data, time.Hour).Err()
}

// maxPlausibleSpeedKmH is generous enough to cover a car/motorcycle in traffic; a jump
// implying a faster speed between two consecutive pings can't be genuine movement.
const maxPlausibleSpeedKmH = 140.0

// checkAnomaly returns true (with the implied speed) when the jump between two pings is
// physically implausible.
func checkAnomaly(lastLat, lastLng float64, lastTS time.Time, lat, lng float64, ts time.Time) (anomalous bool, impliedSpeedKmH float64) {
	elapsed := ts.Sub(lastTS).Hours()
	if elapsed <= 0 {
		return false, 0
	}
	distKm := haversine(lastLat, lastLng, lat, lng)
	speed := distKm / elapsed
	return speed > maxPlausibleSpeedKmH, speed
}
