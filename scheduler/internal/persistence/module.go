package persistence

import (
	"context"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"go.uber.org/fx"
)

// ProvideStore creates an idempotence store based on the configuration
func ProvideStore(cfg *config.Config, lc fx.Lifecycle) (Store, error) {
	store, err := newRedisStore(cfg.Idempotence.RedisURI)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return store.Close()
		},
	})

	return store, nil
}

// Module provides the idempotence store dependency to the fx container
var Module = fx.Options(
	fx.Provide(ProvideStore),
)
