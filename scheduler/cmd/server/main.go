package main

import (
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/jobs"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/logging"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/transport"
)

func main() {
	app := fx.New(
		// Register logging module first
		logging.Module,

		// Configure fx logging
		fx.WithLogger(func(logger *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: logger}
		}),

		// Register other modules
		config.Module,
		k8sclient.Module,
		persistence.Module,
		service.Module,
		transport.Module,
		jobs.Module,
	)

	app.Run()
}
