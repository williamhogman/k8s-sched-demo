package main

import (
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	app := fx.New(
		// Configure logging
		fx.WithLogger(func() fxevent.Logger {
			return &fxevent.ZapLogger{Logger: logger}
		}),

		// Register modules
		config.Module,
		k8sclient.Module,
		persistence.Module,
		service.Module,
		ServerModule,
		CleanupModule,
	)

	app.Run()
}
