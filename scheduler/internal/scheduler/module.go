package scheduler

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// StartServiceParams contains dependencies for starting the scheduler service
type StartServiceParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	Service   *Service
	Logger    *zap.Logger
}

// StartService starts the event processing for pod events
func StartService(p StartServiceParams) {
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			p.Logger.Info("Starting scheduler service event processing")
			p.Service.StartEventProcessing()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			p.Logger.Info("Stopping scheduler service event processing")
			p.Service.Close()
			return nil
		},
	})
}

// Module provides the scheduler service dependencies to the fx container
var Module = fx.Options(
	fx.Provide(New),
	fx.Invoke(StartService),
)
