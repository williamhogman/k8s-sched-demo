package main

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
)

// CleanupParams contains the dependencies for the cleanup manager
type CleanupParams struct {
	fx.In

	Lifecycle        fx.Lifecycle
	Config           *config.Config
	SchedulerService *service.SchedulerService
	Logger           *zap.Logger
}

// ProvideCleanupManager creates and registers the cleanup manager with fx lifecycle
func ProvideCleanupManager(p CleanupParams) {
	logger := p.Logger.Named("cleanup-manager")

	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting sandbox cleanup job",
				zap.Int("intervalSeconds", p.Config.Sandbox.CleanupIntervalSecs),
				zap.Int("batchSize", p.Config.Sandbox.CleanupBatchSize))

			cleanupManager := NewCleanupManager(
				p.SchedulerService,
				p.Config.Sandbox.CleanupIntervalSecs,
				p.Config.Sandbox.CleanupBatchSize,
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

// CleanupModule provides the cleanup components to the fx container
var CleanupModule = fx.Options(
	fx.Invoke(ProvideCleanupManager),
)
