package k8sclient

import (
	"context"
	"time"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

// K8sClientInterface defines the interface for Kubernetes operations
type K8sClientInterface interface {
	// Sandbox operations
	ScheduleSandbox(ctx context.Context) (types.SandboxID, error)
	ReleaseSandbox(ctx context.Context, sandboxID types.SandboxID) error
	GetEventChannel() <-chan types.PodEvent
	StartWatchers()
	StopWatchers()
	IsSandboxReady(ctx context.Context, sandboxID types.SandboxID) (bool, error)
	IsSandboxGone(ctx context.Context, sandboxID types.SandboxID) (bool, error)
	WaitForSandboxReady(ctx context.Context, sandboxID types.SandboxID) (bool, error)

	// Get pod names older than a specific time (max 100 results per call)
	// continueToken is used for pagination; pass empty string for first page
	// Returns pod names and a continueToken for the next page (empty if no more results)
	GetPodsOlderThan(ctx context.Context, olderThan time.Time, continueToken string) ([]string, string, error)

	// Project service operations
	CreateOrUpdateProjectService(ctx context.Context, projectID types.ProjectID, sandboxID types.SandboxID) error
	DeleteProjectService(ctx context.Context, projectID types.ProjectID) error
	GetProjectServiceHostname(projectID types.ProjectID) string
}
