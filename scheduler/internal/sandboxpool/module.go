package sandboxpool

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// StartParams contains dependencies for starting the sandbox pool
type StartParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	Service   *Service
	Logger    *zap.Logger
}

// StartService starts the sandbox pool service
func StartService(p StartParams) {
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			p.Logger.Info("Starting sandbox pool service")
			p.Service.Start()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			p.Logger.Info("Stopping sandbox pool service")
			p.Service.Stop()
			return nil
		},
	})
}

// Module provides the sandbox pool dependencies to the fx container
var Module = fx.Options(
	fx.Provide(New),
	fx.Invoke(StartService),
)
