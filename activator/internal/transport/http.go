package transport

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/williamhogman/k8s-sched-demo/activator/internal/config"
	"github.com/williamhogman/k8s-sched-demo/activator/internal/service"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var Module = fx.Options(
	fx.Invoke(registerHooks),
)

// registerHooks registers lifecycle hooks for graceful startup and shutdown
func registerHooks(
	lifecycle fx.Lifecycle,
	cfg *config.Config,
	logger *zap.Logger,
	activatorSvc *service.ActivatorService,
) {
	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", activatorSvc.HandleRequest)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: mux,
	}

	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Log configuration
			logger.Info("starting activator service with configuration",
				zap.Int("server.port", cfg.Server.Port),
				zap.String("scheduler.address", cfg.Scheduler.Address),
				zap.Duration("scheduler.timeout", cfg.Scheduler.Timeout),
				zap.Bool("logging.development", cfg.Logging.Development),
			)

			// Start HTTP server in a separate goroutine
			go func() {
				logger.Info("starting HTTP server", zap.Int("port", cfg.Server.Port))
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.Error("server error", zap.Error(err))
					os.Exit(1)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("shutting down HTTP server")
			return server.Shutdown(ctx)
		},
	})
}
