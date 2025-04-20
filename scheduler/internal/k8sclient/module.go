package k8sclient

import (
	"context"
	"log"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
	"go.uber.org/fx"
)

// ProvideK8sClient creates a K8s client based on the configuration
func ProvideK8sClient(cfg *config.Config) (service.K8sClientInterface, error) {
	if cfg.Kubernetes.MockMode {
		return NewMockK8sClient(), nil
	}

	return NewK8sClient(cfg)
}

// RegisterHooks registers lifecycle hooks for the K8s client
func RegisterHooks(lc fx.Lifecycle, client service.K8sClientInterface) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Println("Starting Kubernetes client watchers")
			client.StartWatchers()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Println("Stopping Kubernetes client watchers")
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
