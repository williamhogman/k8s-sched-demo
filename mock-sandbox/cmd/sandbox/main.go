package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/williamhogman/k8s-sched-demo/mock-sandbox/internal/server"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func main() {
	// Create a context that will be canceled when we receive a termination signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := fx.New(
		// Provide a logger
		fx.Provide(
			func() *zap.Logger {
				logger, _ := zap.NewProduction()
				return logger
			},
		),
		// Include the server module
		server.Module,
		// Add lifecycle hooks
		fx.Invoke(func(logger *zap.Logger, lc fx.Lifecycle) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					logger.Info("Sandbox application starting")
					return nil
				},
				OnStop: func(ctx context.Context) error {
					logger.Info("Sandbox application stopping")
					return nil
				},
			})
		}),
	)

	// Start the application
	app.Start(ctx)

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for termination signal
	sig := <-sigChan
	logger, _ := zap.NewProduction()
	logger.Info("Received termination signal", zap.String("signal", sig.String()))

	// Create a timeout context for graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Stop the application with timeout
	logger.Info("Initiating graceful shutdown")
	app.Stop(shutdownCtx)
	logger.Info("Application shutdown complete")
}
