package events

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/gen/will/scheduler/v1/schedulerv1connect"
)

// SandboxEventClient is a client for the SandboxEventService
type SandboxEventClient struct {
	client     schedulerv1connect.SandboxEventServiceClient
	httpClient *http.Client
}

// NewSandboxEventClient creates a new client for the SandboxEventService
func NewSandboxEventClient(endpoint string) *SandboxEventClient {
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Create the Connect client
	baseURL := fmt.Sprintf("http://%s", endpoint)
	client := schedulerv1connect.NewSandboxEventServiceClient(
		httpClient,
		baseURL,
	)

	return &SandboxEventClient{
		client:     client,
		httpClient: httpClient,
	}
}

// SendEvent sends a single sandbox event to the event service
func (c *SandboxEventClient) SendEvent(ctx context.Context, event *schedulerv1.SandboxEvent) (*schedulerv1.SandboxEventResponse, error) {
	req := connect.NewRequest(event)
	resp, err := c.client.SendSandboxEvent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to send event: %v", err)
	}
	return resp.Msg, nil
}
