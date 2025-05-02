package persistence

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"go.uber.org/fx"
)

// ProvideRedisClient creates a Redis client based on the configuration
func ProvideRedisClient(cfg *config.Config, lc fx.Lifecycle) (*redis.Client, error) {
	client, err := newRedisClient(cfg.RedisURI)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return client.Close()
		},
	})

	return client, nil
}

// ProvideStore creates an idempotence store based on the configuration
func ProvideStore(client *redis.Client, cfg *config.Config) (Store, error) {
	store, err := newRedisStore(client, defaultKeyPrefix)
	if err != nil {
		return nil, err
	}

	return store, nil
}

// ProvideIdempotenceStore creates an idempotence store based on the configuration
func ProvideIdempotenceStore(client *redis.Client, cfg *config.Config) (IdempotenceStore, error) {
	store, err := NewIdempotenceStore(client, defaultKeyPrefix)
	if err != nil {
		return nil, err
	}

	return store, nil
}

// Module provides the persistence dependencies to the fx container
var Module = fx.Options(
	fx.Provide(ProvideRedisClient),
	fx.Provide(ProvideStore),
	fx.Provide(ProvideIdempotenceStore),
)
