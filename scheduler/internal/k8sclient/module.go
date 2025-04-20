package k8sclient

import (
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

// Module provides the K8s client dependency to the fx container
var Module = fx.Options(
	fx.Provide(ProvideK8sClient),
)
