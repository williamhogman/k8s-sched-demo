package main

import (
	"context"
	"log"

	"go.uber.org/fx"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
)

// CleanupParams contains the dependencies for the cleanup manager
type CleanupParams struct {
	fx.In

	Lifecycle        fx.Lifecycle
	Config           *config.Config
	SchedulerService *service.SchedulerService
}

// ProvideCleanupManager creates and registers the cleanup manager with fx lifecycle
func ProvideCleanupManager(p CleanupParams) {
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Printf("Starting sandbox cleanup job with interval=%ds, batch size=%d",
				p.Config.Sandbox.CleanupIntervalSecs,
				p.Config.Sandbox.CleanupBatchSize)

			cleanupManager := NewCleanupManager(
				p.SchedulerService,
				p.Config.Sandbox.CleanupIntervalSecs,
				p.Config.Sandbox.CleanupBatchSize,
			)
			cleanupManager.Start()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Println("Stopping cleanup manager...")
			// Cleanup happens automatically when the application exits
			return nil
		},
	})
}

// CleanupModule provides the cleanup components to the fx container
var CleanupModule = fx.Options(
	fx.Invoke(ProvideCleanupManager),
)
