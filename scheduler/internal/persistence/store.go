package persistence

import (
	"context"
	"time"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

// Store defines the interface for an idempotence store
type Store interface {
	// GetSandboxID returns the sandbox ID for a given idempotence key, or empty string if not found
	GetSandboxID(ctx context.Context, idempotenceKey string) (types.SandboxID, error)

	// MarkPendingCreation marks that a sandbox is being created for this idempotence key
	// Returns true if successfully marked, false if already marked
	MarkPendingCreation(ctx context.Context, idempotenceKey string, ttl time.Duration) (bool, error)

	// CompletePendingCreation updates a pending sandbox creation with its final sandboxID
	CompletePendingCreation(ctx context.Context, idempotenceKey string, sandboxID types.SandboxID, ttl time.Duration) error

	// IsSandboxReleased checks if a sandbox was recently released
	// Returns true if the sandbox was marked as released, false otherwise
	IsSandboxReleased(ctx context.Context, sandboxID types.SandboxID) (bool, error)

	// GetProjectSandbox returns the sandbox ID for a given project, or empty string if not found
	GetProjectSandbox(ctx context.Context, projectID string) (types.SandboxID, error)

	// SetProjectSandbox stores the sandbox ID for a given project
	SetProjectSandbox(ctx context.Context, projectID string, sandboxID types.SandboxID) error

	// FindProjectForSandbox finds the project ID associated with a sandbox
	// Returns the project ID if found, empty string if not found
	FindProjectForSandbox(ctx context.Context, sandboxID types.SandboxID) (string, error)

	// RemoveSandboxMapping removes all mappings for a sandbox (idempotence key and project)
	RemoveSandboxMapping(ctx context.Context, sandboxID types.SandboxID) error

	// Close cleans up resources used by the store
	Close() error
}
