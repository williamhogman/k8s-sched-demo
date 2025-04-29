package client

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/williamhogman/k8s-sched-demo/activator/internal/config"
	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/gen/will/scheduler/v1/schedulerv1connect"
)

var Module = fx.Options(
	fx.Provide(NewSchedulerClient),
)

// SchedulerClient implements the service.SchedulerClient interface
type SchedulerClient struct {
	client schedulerv1connect.ProjectServiceClient
	logger *zap.Logger
}

// NewSchedulerClient creates a new scheduler client
func NewSchedulerClient(cfg *config.Config, logger *zap.Logger) *SchedulerClient {
	client := schedulerv1connect.NewProjectServiceClient(
		http.DefaultClient,
		cfg.Scheduler.Address,
	)

	return &SchedulerClient{
		client: client,
		logger: logger.Named("scheduler-client"),
	}
}

// GetSandbox gets a sandbox from the scheduler service
func (c *SchedulerClient) GetSandbox(ctx context.Context, projectID string) (string, error) {
	// Create a request to get a sandbox
	req := connect.NewRequest(&schedulerv1.GetProjectSandboxRequest{
		ProjectId:       projectID,
		WaitForCreation: true,
	})

	// Call the scheduler service
	resp, err := c.client.GetProjectSandbox(ctx, req)
	if err != nil {
		c.logger.Error("failed to get sandbox",
			zap.Error(err),
		)
		return "", fmt.Errorf("failed to get sandbox: %v", err)
	}

	// Check the sandbox status
	if resp.Msg.Status != schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ACTIVE {
		c.logger.Error("sandbox not active",
			zap.String("status", resp.Msg.Status.String()),
		)
		return "", fmt.Errorf("sandbox not active: %s", resp.Msg.Status.String())
	}

	return resp.Msg.Hostname, nil
}
