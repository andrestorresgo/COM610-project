package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	Client *redis.Client
}

func NewRedisStore(redisURL string) (*RedisStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	// Configure automatic retries and exponential backoff
	opts.MaxRetries = 5
	opts.MinRetryBackoff = 8 * time.Millisecond
	opts.MaxRetryBackoff = 512 * time.Millisecond

	client := redis.NewClient(opts)

	// Ping the server to ensure connection is established
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return &RedisStore{
		Client: client,
	}, nil
}
