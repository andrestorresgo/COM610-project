package cache

import (
	"context"
	"fmt"
	"time"

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

// GetCourierLocation retrieves the coordinates for a courier from active couriers
func (r *RedisStore) GetCourierLocation(ctx context.Context, courierID string) (*redis.GeoPos, error) {
	pos, err := r.Client.GeoPos(ctx, activeCouriersKey, courierID).Result()
	if err != nil {
		return nil, err
	}
	if len(pos) == 0 || pos[0] == nil {
		return nil, fmt.Errorf("courier location not found")
	}
	return pos[0], nil
}

// UpdateCourierActiveTime sets the last active timestamp of a courier
func (r *RedisStore) UpdateCourierActiveTime(ctx context.Context, courierID string) error {
	key := fmt.Sprintf("courier:%s:last_active", courierID)
	return r.Client.Set(ctx, key, time.Now().Unix(), 0).Err()
}

// GetCourierActiveTime gets the last active timestamp of a courier
func (r *RedisStore) GetCourierActiveTime(ctx context.Context, courierID string) (int64, error) {
	key := fmt.Sprintf("courier:%s:last_active", courierID)
	val, err := r.Client.Get(ctx, key).Int64()
	if err != nil {
		return 0, err
	}
	return val, nil
}
