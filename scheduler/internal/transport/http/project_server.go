package http

import (
	"context"

	"connectrpc.com/connect"
	"go.uber.org/zap"

	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
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

	projectID, err := types.NewProjectIDWithoutPrefix(req.Msg.ProjectId)
	if err != nil {
		s.logger.Error("Failed to parse project ID", zap.Error(err))
		return nil, err
	}

	// Call the project service
	resp, err := s.projectService.GetProjectSandbox(
		ctx,
		projectID,
		req.Msg.WaitForCreation,
	)

	if err != nil {
		s.logger.Error("Failed to get project sandbox",
			projectID.ZapField(),
			zap.Error(err))
		return nil, err
	}

	// Build the Connect response
	return connect.NewResponse(resp), nil
}
