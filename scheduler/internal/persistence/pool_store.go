package persistence

import (
	"context"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

// SandboxPoolStore defines the interface for storing and retrieving ready sandboxes
type SandboxPoolStore interface {
	// AddReadySandbox adds a ready sandbox to the pool
	AddReadySandbox(ctx context.Context, sandboxID types.SandboxID) error

	// GetReadySandbox gets and removes a ready sandbox from the pool
	GetReadySandbox(ctx context.Context) (types.SandboxID, error)

	// CountReadySandboxes returns the number of ready sandboxes in the pool
	CountReadySandboxes(ctx context.Context) (int64, error)

	// AddPendingSandbox adds a sandbox to the pending set with creation timestamp
	AddPendingSandbox(ctx context.Context, sandboxID types.SandboxID) error

	// GetPendingSandboxes returns the list of pending sandboxes and their creation times
	GetPendingSandboxes(ctx context.Context) ([]types.SandboxID, error)

	// RemovePendingSandbox removes a pending sandbox from the pending set
	RemovePendingSandbox(ctx context.Context, sandboxID types.SandboxID) error

	// CountPendingSandboxes returns the number of pending sandboxes
	CountPendingSandboxes(ctx context.Context) (int64, error)
}
