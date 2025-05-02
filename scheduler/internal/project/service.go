package project

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"

	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/sandboxpool"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/scheduler"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

// ServiceParams contains dependencies for the project service
type ServiceParams struct {
	fx.In

	Config           *config.Config
	SchedulerService *scheduler.Service
	SandboxPool      *sandboxpool.Service `optional:"true"`
	Store            persistence.Store
	K8sClient        k8sclient.K8sClientInterface
	Logger           *zap.Logger
}

// Service implements the project-level sandbox management
type Service struct {
	config           *config.Config
	schedulerService *scheduler.Service
	sandboxPool      *sandboxpool.Service
	store            persistence.Store
	k8sClient        k8sclient.K8sClientInterface
	logger           *zap.Logger
}

// New creates a new project service
func New(
	params ServiceParams,
) *Service {
	return &Service{
		config:           params.Config,
		schedulerService: params.SchedulerService,
		sandboxPool:      params.SandboxPool,
		store:            params.Store,
		k8sClient:        params.K8sClient,
		logger:           params.Logger.Named("project-service"),
	}
}

// GetProjectSandbox gets or creates a sandbox for a project
func (s *Service) GetProjectSandbox(
	ctx context.Context,
	projectID types.ProjectID,
	waitForCreation bool,
) (*schedulerv1.GetProjectSandboxResponse, error) {
	// Create log context
	logContext := []zap.Field{projectID.ZapField()}

	// Check if project already has an active sandbox
	sandboxID, err := s.store.GetProjectSandbox(ctx, projectID)
	if err == nil && sandboxID != "" {
		// Project has a sandbox in our store, check if it's still alive in Kubernetes
		gone, err := s.schedulerService.IsSandboxGone(ctx, sandboxID)
		if err != nil {
			s.logger.Error("Failed to check if sandbox is gone",
				append(logContext, sandboxID.ZapField(), zap.Error(err))...)
			// Continue with sandbox creation if we can't determine if it's gone
		} else if gone {
			s.logger.Info("Sandbox is gone, will create a new one",
				append(logContext, sandboxID.ZapField())...)
			sandboxID = ""
		} else {
			// Sandbox exists, return it
			hostname := s.k8sClient.GetProjectServiceHostname(projectID)
			return &schedulerv1.GetProjectSandboxResponse{
				SandboxId: sandboxID.String(),
				Status:    schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ACTIVE,
				Hostname:  hostname,
			}, nil
		}
	}

	// Create a new sandbox for the project
	sandboxID, schedulingErr := s.sandboxPool.GetSandbox(ctx, projectID.String())
	if schedulingErr != nil && !errors.Is(schedulingErr, persistence.ErrPoolEmpty) {
		return nil, schedulingErr
	}
	if sandboxID == "" {
		sandboxID, schedulingErr = s.schedulerService.ScheduleSandbox(ctx, projectID.String())
	}

	if schedulingErr != nil {
		s.logger.Error("Failed to create sandbox for project",
			append(logContext, zap.Error(schedulingErr))...)
		return &schedulerv1.GetProjectSandboxResponse{
			Status: schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ERROR,
		}, fmt.Errorf("failed to create sandbox: %v", schedulingErr)
	}

	// Store the project-sandbox mapping
	if err := s.store.SetProjectSandbox(ctx, projectID, sandboxID); err != nil {
		s.logger.Error("Failed to store project-sandbox mapping",
			append(logContext, sandboxID.ZapField(), zap.Error(err))...)
		// Continue anyway, the sandbox is created
	}

	// Create or update the headless service for this project
	if err := s.k8sClient.CreateOrUpdateProjectService(ctx, projectID, sandboxID); err != nil {
		s.logger.Error("Failed to create/update project service",
			append(logContext, sandboxID.ZapField(), zap.Error(err))...)
		// Continue anyway, the sandbox is created
	}

	if waitForCreation {
		// Wait for the sandbox to be ready
		ready, err := s.schedulerService.WaitForSandboxReady(ctx, sandboxID)
		if err != nil {
			return nil, fmt.Errorf("failed to wait for sandbox to be ready: %v", err)
		}
		if !ready {
			return &schedulerv1.GetProjectSandboxResponse{
				Status: schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ERROR,
			}, nil
		}
		return &schedulerv1.GetProjectSandboxResponse{
			SandboxId: sandboxID.String(),
			Status:    schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ACTIVE,
			Hostname:  s.k8sClient.GetProjectServiceHostname(projectID),
		}, nil
	} else {
		return &schedulerv1.GetProjectSandboxResponse{
			SandboxId: sandboxID.String(),
			Status:    schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_CREATING,
			Hostname:  s.k8sClient.GetProjectServiceHostname(projectID),
		}, nil
	}
}
