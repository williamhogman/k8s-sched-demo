package persistence

import (
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"go.uber.org/fx"
)

// ProvideStore creates an idempotence store based on the configuration
func ProvideStore(cfg *config.Config) (Store, error) {
	return NewStore(Config{
		Type:     cfg.Idempotence.Type,
		RedisURI: cfg.Idempotence.RedisURI,
	})
}

// Module provides the idempotence store dependency to the fx container
var Module = fx.Options(
	fx.Provide(ProvideStore),
)
