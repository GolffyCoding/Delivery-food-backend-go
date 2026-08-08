package driver

import (
	"context"
	"math"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// LocationCache stores driver positions in Redis using GEOADD/GEOSEARCH so nearest-driver
// lookups don't require scanning every online driver row in Postgres.
type LocationCache struct {
	rdb *redis.Client
	key string
}

func NewLocationCache(rdb *redis.Client) *LocationCache {
	return &LocationCache{rdb: rdb, key: "drivers:geo"}
}

func (c *LocationCache) UpdateLocation(ctx context.Context, driverID uuid.UUID, lat, lng float64) error {
	return c.rdb.GeoAdd(ctx, c.key, &redis.GeoLocation{
		Longitude: lng,
		Latitude:  lat,
		Name:      driverID.String(),
	}).Err()
}

func (c *LocationCache) RemoveDriver(ctx context.Context, driverID uuid.UUID) error {
	return c.rdb.ZRem(ctx, c.key, driverID.String()).Err()
}

type GeoResult struct {
	DriverID uuid.UUID
	DistKm   float64
}

func (c *LocationCache) FindNearest(ctx context.Context, lat, lng, radiusKm float64, limit int) ([]GeoResult, error) {
	res, err := c.rdb.GeoSearchLocation(ctx, c.key, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  lng,
			Latitude:   lat,
			Radius:     radiusKm,
			RadiusUnit: "km",
			Sort:       "ASC",
			Count:      limit,
		},
		WithCoord: false,
		WithDist:  true,
	}).Result()
	if err != nil {
		return nil, err
	}

	results := make([]GeoResult, 0, len(res))
	for _, loc := range res {
		id, err := uuid.Parse(loc.Name)
		if err != nil {
			continue
		}
		results = append(results, GeoResult{
			DriverID: id,
			DistKm:   math.Round(loc.Dist*100) / 100,
		})
	}
	return results, nil
}
