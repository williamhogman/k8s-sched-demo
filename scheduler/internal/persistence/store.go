package persistence

import (
	"context"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

// Store defines the interface for the persistence layer
type Store interface {
	// GetProjectSandbox returns the sandbox ID for a given project
	GetProjectSandbox(ctx context.Context, projectID types.ProjectID) (types.SandboxID, error)

	// SetProjectSandbox stores the sandbox ID for a given project
	SetProjectSandbox(ctx context.Context, projectID types.ProjectID, sandboxID types.SandboxID) error

	// FindProjectForSandbox finds the project ID associated with a sandbox
	FindProjectForSandbox(ctx context.Context, sandboxID types.SandboxID) (types.ProjectID, error)

	// RemoveSandboxMapping removes all mappings for a sandbox
	RemoveSandboxMapping(ctx context.Context, sandboxID types.SandboxID) error

	// IsSandboxReleased checks if a sandbox was recently released
	IsSandboxReleased(ctx context.Context, sandboxID types.SandboxID) (bool, error)

	// SubscribeToProjectSandboxUpdates subscribes to project-sandbox mapping updates
	SubscribeToProjectSandboxUpdates(ctx context.Context, callback func(projectID types.ProjectID, sandboxID types.SandboxID, isRemoval bool)) error

	// GetAllProjectSandboxMappings returns all project-sandbox mappings
	GetAllProjectSandboxMappings(ctx context.Context) (map[types.ProjectID]types.SandboxID, error)

	// Close closes any connections
	Close() error
}
