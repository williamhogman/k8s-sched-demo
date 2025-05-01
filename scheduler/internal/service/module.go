package service

import (
	"context"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// ProvideSchedulerService creates a scheduler service with the given dependencies
func ProvideSchedulerService(
	cfg *config.Config,
	k8sClient k8sclient.K8sClientInterface,
	idempotenceStore persistence.Store,
	logger *zap.Logger,
) *SchedulerService {
	return NewSchedulerService(
		k8sClient,
		idempotenceStore,
		SchedulerServiceConfig{
			SandboxTTL: cfg.SandboxTTL,
		},
		logger,
	)
}

// StartSchedulerService starts the event processing for pod events
func StartSchedulerService(lc fx.Lifecycle, service *SchedulerService, logger *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting scheduler service event processing")
			service.startEventProcessing()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping scheduler service event processing")
			service.eventCancel()
			return nil
		},
	})
}

// ProvideProjectService creates a project service with the given dependencies
func ProvideProjectService(
	schedulerService *SchedulerService,
	idempotenceStore persistence.Store,
	k8sClient k8sclient.K8sClientInterface,
	logger *zap.Logger,
) *ProjectService {
	return NewProjectService(
		schedulerService,
		idempotenceStore,
		k8sClient,
		logger,
	)
}

// Module provides the service dependencies to the fx container
var Module = fx.Options(
	fx.Provide(ProvideSchedulerService),
	fx.Invoke(StartSchedulerService),
	fx.Provide(ProvideProjectService),
)
