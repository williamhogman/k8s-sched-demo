package k8sclient

import (
	"context"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// ProvideK8sClient creates a K8s client based on the configuration
func ProvideK8sClient(cfg *config.Config, logger *zap.Logger) (K8sClientInterface, error) {
	if cfg.Kubernetes.MockMode {
		return NewMockK8sClient(logger), nil
	}

	return NewK8sClient(cfg, logger)
}

// RegisterHooks registers lifecycle hooks for the K8s client
func RegisterHooks(lc fx.Lifecycle, client K8sClientInterface, logger *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting Kubernetes client watchers")
			client.StartWatchers()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping Kubernetes client watchers")
			client.StopWatchers()
			return nil
		},
	})
}

// Module provides the K8s client dependency to the fx container
var Module = fx.Options(
	fx.Provide(ProvideK8sClient),
	fx.Invoke(RegisterHooks),
)
