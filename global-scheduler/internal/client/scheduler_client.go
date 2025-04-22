package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/gen/will/scheduler/v1/schedulerv1connect"
)

// SchedulerClientInterface defines the interface for scheduler client
type SchedulerClientInterface interface {
	ScheduleSandbox(ctx context.Context, endpoint string, idempotenceKey string, metadata map[string]string) (*schedulerv1.ScheduleResponse, error)
	ReleaseSandbox(ctx context.Context, endpoint string, sandboxID string) (*schedulerv1.ReleaseSandboxResponse, error)
	RetainSandbox(ctx context.Context, endpoint string, sandboxID string) (*schedulerv1.RetainSandboxResponse, error)
}

// SchedulerClient is a client for the scheduler service
type SchedulerClient struct {
	httpClient *http.Client
}

// NewSchedulerClient creates a new scheduler client
func NewSchedulerClient() *SchedulerClient {
	return &SchedulerClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ScheduleSandbox schedules a sandbox on a selected cluster using Connect RPC
func (c *SchedulerClient) ScheduleSandbox(ctx context.Context, endpoint string, idempotenceKey string, metadata map[string]string) (*schedulerv1.ScheduleResponse, error) {
	// Create the Connect client
	baseURL := fmt.Sprintf("http://%s", endpoint)
	client := schedulerv1connect.NewSandboxSchedulerClient(
		c.httpClient,
		baseURL,
	)

	// Create the request
	req := connect.NewRequest(&schedulerv1.ScheduleRequest{
		IdempotenceKey: idempotenceKey,
		Metadata:       metadata,
	})

	// Make the request
	resp, err := client.ScheduleSandbox(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to schedule sandbox: %v", err)
	}

	return resp.Msg, nil
}

// ReleaseSandbox releases a sandbox on a selected cluster using Connect RPC
func (c *SchedulerClient) ReleaseSandbox(ctx context.Context, endpoint string, sandboxID string) (*schedulerv1.ReleaseSandboxResponse, error) {
	// Create the Connect client
	baseURL := fmt.Sprintf("http://%s", endpoint)
	client := schedulerv1connect.NewSandboxSchedulerClient(
		c.httpClient,
		baseURL,
	)

	// Create the request
	req := connect.NewRequest(&schedulerv1.ReleaseSandboxRequest{
		SandboxId: sandboxID,
	})

	// Make the request
	resp, err := client.ReleaseSandbox(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to release sandbox: %v", err)
	}

	return resp.Msg, nil
}

// RetainSandbox extends the expiration time of a sandbox on a selected cluster using Connect RPC
func (c *SchedulerClient) RetainSandbox(ctx context.Context, endpoint string, sandboxID string) (*schedulerv1.RetainSandboxResponse, error) {
	// Create the Connect client
	baseURL := fmt.Sprintf("http://%s", endpoint)
	client := schedulerv1connect.NewSandboxSchedulerClient(
		c.httpClient,
		baseURL,
	)

	// Create the request
	req := connect.NewRequest(&schedulerv1.RetainSandboxRequest{
		SandboxId: sandboxID,
	})

	// Make the request
	resp, err := client.RetainSandbox(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to retain sandbox: %v", err)
	}

	return resp.Msg, nil
}
