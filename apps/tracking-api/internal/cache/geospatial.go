package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const activeCouriersKey = "couriers:active"

// UpdateCourierLocation uses GEOADD to store the courier's lat/lon under a specific key
func (r *RedisStore) UpdateCourierLocation(ctx context.Context, courierID string, lon float64, lat float64) error {
	return r.Client.GeoAdd(ctx, activeCouriersKey, &redis.GeoLocation{
		Name:      courierID,
		Longitude: lon,
		Latitude:  lat,
	}).Err()
}

// GetNearbyCouriers uses GEOSEARCH to find couriers within a certain radius of a delivery point
func (r *RedisStore) GetNearbyCouriers(ctx context.Context, lon float64, lat float64, radiusKm float64) ([]redis.GeoLocation, error) {
	return r.Client.GeoSearchLocation(ctx, activeCouriersKey, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  lon,
			Latitude:   lat,
			Radius:     radiusKm,
			RadiusUnit: "km",
		},
		WithCoord: true,
		WithDist:  true,
	}).Result()
}

// UpdateDeliveryLocation stores the longitude and latitude under a key specific to the active delivery
func (r *RedisStore) UpdateDeliveryLocation(ctx context.Context, orderID string, lon float64, lat float64) error {
	key := fmt.Sprintf("delivery:%s:location", orderID)
	return r.Client.GeoAdd(ctx, key, &redis.GeoLocation{
		Name:      "courier",
		Longitude: lon,
		Latitude:  lat,
	}).Err()
}
