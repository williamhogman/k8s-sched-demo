package cleanup

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/scheduler"
)

// ManagerParams contains the dependencies for the cleanup manager
type ManagerParams struct {
	fx.In

	Lifecycle        fx.Lifecycle
	Config           *config.Config
	SchedulerService *scheduler.Service
	Logger           *zap.Logger
}

// ProvideManager creates and registers the cleanup manager with fx lifecycle
func ProvideManager(p ManagerParams) {
	logger := p.Logger.Named("cleanup-manager")

	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting sandbox cleanup job",
				zap.Int("intervalSeconds", p.Config.CleanupIntervalSecs),
				zap.Int("batchSize", p.Config.CleanupBatchSize))

			cleanupManager := NewManager(
				p.SchedulerService,
				p.Config.CleanupIntervalSecs,
				p.Config.CleanupBatchSize,
				logger,
			)
			cleanupManager.Start()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping cleanup manager")
			// Cleanup happens automatically when the application exits
			return nil
		},
	})
}

// Module provides the cleanup components to the fx container
var Module = fx.Options(
	fx.Invoke(ProvideManager),
)
