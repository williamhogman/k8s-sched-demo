package main

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/williamhogman/k8s-sched-demo/gen/will/scheduler/v1/schedulerv1connect"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
)

// ServerParams contains the dependencies for the server
type ServerParams struct {
	fx.In

	Lifecycle        fx.Lifecycle
	Config           *config.Config
	SchedulerService *service.SchedulerService
	Logger           *zap.Logger
}

// ProvideServer creates and registers the HTTP server with fx lifecycle
func ProvideServer(p ServerParams) {
	logger := p.Logger.Named("server")
	server := NewSchedulerServer(p.SchedulerService, logger)

	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				// Set up the HTTP routes with Connect handlers
				mux := http.NewServeMux()
				path, handler := schedulerv1connect.NewSandboxSchedulerHandler(server)
				mux.Handle(path, handler)

				// Start the server
				addr := fmt.Sprintf(":%d", p.Config.Server.Port)
				logger.Info("Starting Scheduler server with Connect API",
					zap.String("address", addr),
					zap.Int("port", p.Config.Server.Port))

				if err := http.ListenAndServe(
					addr,
					// Use h2c so we can serve HTTP/2 without TLS
					h2c.NewHandler(mux, &http2.Server{}),
				); err != nil {
					logger.Fatal("Failed to start server", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// We could implement server graceful shutdown here if needed
			logger.Info("Stopping server")
			return nil
		},
	})
}

// ServerModule provides the server components to the fx container
var ServerModule = fx.Options(
	fx.Invoke(ProvideServer),
)
