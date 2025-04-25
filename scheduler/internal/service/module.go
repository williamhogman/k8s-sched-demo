package service

import (
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// ProvideSchedulerService creates a scheduler service with the given dependencies
func ProvideSchedulerService(
	cfg *config.Config,
	k8sClient K8sClientInterface,
	idempotenceStore persistence.Store,
	logger *zap.Logger,
) *SchedulerService {
	return NewSchedulerService(
		k8sClient,
		idempotenceStore,
		SchedulerServiceConfig{
			IdempotenceKeyTTL: cfg.Idempotence.TTL,
			SandboxTTL:        cfg.Sandbox.TTL,
		},
		logger,
	)
}

// ProvideProjectService creates a project service with the given dependencies
func ProvideProjectService(
	schedulerService *SchedulerService,
	idempotenceStore persistence.Store,
	logger *zap.Logger,
) *ProjectService {
	return NewProjectService(
		schedulerService,
		idempotenceStore,
		logger,
	)
}

// Module provides the service dependencies to the fx container
var Module = fx.Options(
	fx.Provide(ProvideSchedulerService),
	fx.Provide(ProvideProjectService),
)
