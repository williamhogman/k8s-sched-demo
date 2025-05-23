package http

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
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/project"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/scheduler"
)

// ServerParams contains the dependencies for the server
type ServerParams struct {
	fx.In

	Lifecycle        fx.Lifecycle
	Config           *config.Config
	SchedulerService *scheduler.Service
	ProjectService   *project.Service
	Logger           *zap.Logger
}

// ProvideServer creates and registers the HTTP server with fx lifecycle
func ProvideServer(p ServerParams) {
	logger := p.Logger.Named("server")
	projectServer := NewProjectServer(p.ProjectService, logger)

	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				// Set up the HTTP routes with Connect handlers
				mux := http.NewServeMux()

				// Register the project service handler
				projectPath, projectHandler := schedulerv1connect.NewProjectServiceHandler(projectServer)
				mux.Handle(projectPath, projectHandler)

				addr := fmt.Sprintf(":%d", p.Config.Port)
				logger.Info("Starting Scheduler server with Connect API",
					zap.String("address", addr),
					zap.Int("port", p.Config.Port))

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

// Module provides the server components to the fx container
var Module = fx.Options(
	fx.Invoke(ProvideServer),
)
