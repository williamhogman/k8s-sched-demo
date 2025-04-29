package k8sclient

import (
	"context"
	"time"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

// K8sClientInterface defines the interface for Kubernetes operations
type K8sClientInterface interface {
	// Sandbox operations
	ScheduleSandbox(ctx context.Context, podName string, metadata map[string]string) (string, error)
	ReleaseSandbox(ctx context.Context, sandboxID string) error
	GetEventChannel() <-chan types.PodEvent
	StartWatchers()
	StopWatchers()
	IsSandboxReady(ctx context.Context, sandboxID string) (bool, error)
	IsSandboxGone(ctx context.Context, sandboxID string) (bool, error)
	WaitForSandboxReady(ctx context.Context, sandboxID string, timeout time.Duration) (bool, error)

	// Project service operations
	CreateOrUpdateProjectService(ctx context.Context, projectID string, sandboxID string) error
	DeleteProjectService(ctx context.Context, projectID string) error
	GetProjectServiceHostname(projectID string) string
}
