package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
)

// ProjectService implements the project-level sandbox management
type ProjectService struct {
	schedulerService *SchedulerService
	store            persistence.Store
	k8sClient        k8sclient.K8sClientInterface
	logger           *zap.Logger
}

// NewProjectService creates a new project service
func NewProjectService(
	schedulerService *SchedulerService,
	store persistence.Store,
	k8sClient k8sclient.K8sClientInterface,
	logger *zap.Logger,
) *ProjectService {
	return &ProjectService{
		schedulerService: schedulerService,
		store:            store,
		k8sClient:        k8sClient,
		logger:           logger.Named("project-service"),
	}
}

// GetProjectSandbox gets or creates a sandbox for a project
func (s *ProjectService) GetProjectSandbox(
	ctx context.Context,
	projectID string,
	metadata map[string]string,
	waitForCreation bool,
) (*schedulerv1.GetProjectSandboxResponse, error) {
	// Create log context
	logContext := []zap.Field{zap.String("projectID", projectID)}

	// Check if project already has an active sandbox
	sandboxID, err := s.store.GetProjectSandbox(ctx, projectID)
	if err == nil && sandboxID != "" {
		// Project has an active sandbox
		hostname := s.k8sClient.GetProjectServiceHostname(sandboxID)
		return &schedulerv1.GetProjectSandboxResponse{
			SandboxId: sandboxID,
			Status:    schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ACTIVE,
			Hostname:  hostname,
		}, nil
	}

	// No active sandbox found
	if !waitForCreation {
		return &schedulerv1.GetProjectSandboxResponse{
			Status: schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_NOT_FOUND,
		}, nil
	}

	// Create a new sandbox for the project
	sandboxID, success, err := s.schedulerService.ScheduleSandbox(ctx, projectID, metadata)
	if err != nil {
		s.logger.Error("Failed to create sandbox for project",
			append(logContext, zap.Error(err))...)
		return &schedulerv1.GetProjectSandboxResponse{
			Status: schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ERROR,
		}, fmt.Errorf("failed to create sandbox: %v", err)
	}

	if !success {
		s.logger.Error("Failed to create sandbox for project",
			append(logContext, zap.String("sandboxID", sandboxID))...)
		return &schedulerv1.GetProjectSandboxResponse{
			Status: schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ERROR,
		}, fmt.Errorf("failed to create sandbox")
	}

	// Store the project-sandbox mapping
	if err := s.store.SetProjectSandbox(ctx, projectID, sandboxID); err != nil {
		s.logger.Error("Failed to store project-sandbox mapping",
			append(logContext, zap.String("sandboxID", sandboxID), zap.Error(err))...)
		// Continue anyway, the sandbox is created
	}

	// Create or update the headless service for this project
	if err := s.k8sClient.CreateOrUpdateProjectService(ctx, projectID, sandboxID); err != nil {
		s.logger.Error("Failed to create/update project service",
			append(logContext, zap.String("sandboxID", sandboxID), zap.Error(err))...)
		// Continue anyway, the sandbox is created
	}

	return &schedulerv1.GetProjectSandboxResponse{
		SandboxId: sandboxID,
		Status:    schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ACTIVE,
		Hostname:  s.k8sClient.GetProjectServiceHostname(projectID),
	}, nil
}

// ReleaseProjectSandbox releases the current sandbox for a project
func (s *ProjectService) ReleaseProjectSandbox(
	ctx context.Context,
	projectID string,
) (*schedulerv1.ReleaseProjectSandboxResponse, error) {
	// Create log context
	logContext := []zap.Field{zap.String("projectID", projectID)}

	// Get the current sandbox for the project
	sandboxID, err := s.store.GetProjectSandbox(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get project sandbox",
			append(logContext, zap.Error(err))...)
		return &schedulerv1.ReleaseProjectSandboxResponse{
			Success: false,
		}, fmt.Errorf("failed to get project sandbox: %v", err)
	}

	if sandboxID == "" {
		return &schedulerv1.ReleaseProjectSandboxResponse{
			Success: true, // No sandbox to release is still a success
		}, nil
	}

	// Release the sandbox
	success, err := s.schedulerService.ReleaseSandbox(ctx, sandboxID)
	if err != nil {
		s.logger.Error("Failed to release sandbox",
			append(logContext, zap.String("sandboxID", sandboxID), zap.Error(err))...)
		return &schedulerv1.ReleaseProjectSandboxResponse{
			Success: false,
		}, fmt.Errorf("failed to release sandbox: %v", err)
	}

	// Remove the project-sandbox mapping
	if err := s.store.RemoveProjectSandbox(ctx, projectID); err != nil {
		s.logger.Error("Failed to remove project-sandbox mapping",
			append(logContext, zap.String("sandboxID", sandboxID), zap.Error(err))...)
		// Continue anyway, the sandbox is released
	}

	// Delete the project's headless service
	if err := s.k8sClient.DeleteProjectService(ctx, projectID); err != nil {
		s.logger.Error("Failed to delete project service",
			append(logContext, zap.String("sandboxID", sandboxID), zap.Error(err))...)
		// Continue anyway, the sandbox is released
	}

	return &schedulerv1.ReleaseProjectSandboxResponse{
		Success:           success,
		ReleasedSandboxId: sandboxID,
	}, nil
}

// GetProjectStatus gets the current status of a project's sandbox
func (s *ProjectService) GetProjectStatus(
	ctx context.Context,
	projectID string,
) (*schedulerv1.GetProjectStatusResponse, error) {
	// Create log context
	logContext := []zap.Field{zap.String("projectID", projectID)}

	// Get the current sandbox for the project
	sandboxID, err := s.store.GetProjectSandbox(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get project sandbox",
			append(logContext, zap.Error(err))...)
		return &schedulerv1.GetProjectStatusResponse{
			Status: schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ERROR,
		}, fmt.Errorf("failed to get project sandbox: %v", err)
	}

	if sandboxID == "" {
		return &schedulerv1.GetProjectStatusResponse{
			Status: schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_NOT_FOUND,
		}, nil
	}

	return &schedulerv1.GetProjectStatusResponse{
		SandboxId: sandboxID,
		Status:    schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ACTIVE,
	}, nil
}
