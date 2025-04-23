package http

import (
	"context"

	"connectrpc.com/connect"
	"go.uber.org/zap"

	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
)

// ProjectServer implements the ConnectRPC ProjectService
type ProjectServer struct {
	projectService *service.ProjectService
	logger         *zap.Logger
}

// NewProjectServer creates a new instance of ProjectServer
func NewProjectServer(projectService *service.ProjectService, logger *zap.Logger) *ProjectServer {
	return &ProjectServer{
		projectService: projectService,
		logger:         logger.Named("project-server"),
	}
}

// GetProjectSandbox implements the GetProjectSandbox method from the ProjectService
func (s *ProjectServer) GetProjectSandbox(
	ctx context.Context,
	req *connect.Request[schedulerv1.GetProjectSandboxRequest],
) (*connect.Response[schedulerv1.GetProjectSandboxResponse], error) {
	// Call the project service
	resp, err := s.projectService.GetProjectSandbox(
		ctx,
		req.Msg.ProjectId,
		req.Msg.Metadata,
		req.Msg.WaitForCreation,
	)

	if err != nil {
		s.logger.Error("Failed to get project sandbox",
			zap.String("projectID", req.Msg.ProjectId),
			zap.Error(err))
		return nil, err
	}

	// Build the Connect response
	return connect.NewResponse(resp), nil
}

// ReleaseProjectSandbox implements the ReleaseProjectSandbox method from the ProjectService
func (s *ProjectServer) ReleaseProjectSandbox(
	ctx context.Context,
	req *connect.Request[schedulerv1.ReleaseProjectSandboxRequest],
) (*connect.Response[schedulerv1.ReleaseProjectSandboxResponse], error) {
	// Call the project service
	resp, err := s.projectService.ReleaseProjectSandbox(
		ctx,
		req.Msg.ProjectId,
	)

	if err != nil {
		s.logger.Error("Failed to release project sandbox",
			zap.String("projectID", req.Msg.ProjectId),
			zap.Error(err))
		return nil, err
	}

	// Build the Connect response
	return connect.NewResponse(resp), nil
}

// GetProjectStatus implements the GetProjectStatus method from the ProjectService
func (s *ProjectServer) GetProjectStatus(
	ctx context.Context,
	req *connect.Request[schedulerv1.GetProjectStatusRequest],
) (*connect.Response[schedulerv1.GetProjectStatusResponse], error) {
	// Call the project service
	resp, err := s.projectService.GetProjectStatus(
		ctx,
		req.Msg.ProjectId,
	)

	if err != nil {
		s.logger.Error("Failed to get project status",
			zap.String("projectID", req.Msg.ProjectId),
			zap.Error(err))
		return nil, err
	}

	// Build the Connect response
	return connect.NewResponse(resp), nil
}
