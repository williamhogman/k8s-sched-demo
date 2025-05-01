package persistence

import (
	"context"
	"errors"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

// ClaimResult represents the result of attempting to claim an idempotence key
type ClaimResult struct {
	// Claimed is true if we successfully claimed the idempotence key
	Claimed bool
	// SandboxID is non-empty if we found an existing sandbox or waited for one to be created
	SandboxID types.SandboxID
	// Err contains any error that occurred during the operation
	Err error

	// Private fields
	store          idempotenceSink
	idempotenceKey string
	completed      bool
}

// HasExistingSandbox returns true if the result contains a valid sandbox ID
func (r ClaimResult) HasExistingSandbox() bool {
	return r.SandboxID != ""
}

// ShouldCreateSandbox returns true if we should proceed with creating a new sandbox
func (r ClaimResult) ShouldCreateSandbox() bool {
	return r.Claimed || (r.SandboxID == "" && r.Err != nil)
}

// IsTimeout returns true if the error was a timeout or cancellation
func (r ClaimResult) IsTimeout() bool {
	return errors.Is(r.Err, context.DeadlineExceeded) || errors.Is(r.Err, context.Canceled)
}

// HasError returns true if there was an error during the operation
func (r ClaimResult) HasError() bool {
	return r.Err != nil
}

// Error returns the error message or an empty string if no error
func (r ClaimResult) Error() string {
	if r.Err != nil {
		return r.Err.Error()
	}
	return ""
}

// CompleteClaim completes the idempotence claim with the provided sandbox ID
// Only applicable if Claimed is true
func (r *ClaimResult) Complete(ctx context.Context, sandboxID types.SandboxID) error {
	if r.completed {
		return errors.New("already completed")
	}

	if !r.Claimed {
		return nil
	}

	if r.store == nil {
		return errors.New("store reference is missing")
	}

	err := r.store.CompletePendingCreation(ctx, r.idempotenceKey, sandboxID)
	if err != nil {
		return err
	}

	r.completed = true
	return nil
}

func (r *ClaimResult) Drop(ctx context.Context) error {
	if r.completed {
		return nil
	}

	if !r.Claimed {
		return nil
	}

	if r.store == nil {
		return errors.New("store reference is missing")
	}

	// If we haven't assigned a sandbox ID yet, we can just drop the pending key
	return r.store.DropPendingCreation(ctx, r.idempotenceKey)
}

// idempotenceSink allows completing and dropping idempotence keys
type idempotenceSink interface {
	// CompletePendingCreation updates a pending sandbox creation with its final sandboxID
	CompletePendingCreation(ctx context.Context, idempotenceKey string, sandboxID types.SandboxID) error

	// DropPendingCreation removes a pending creation marker without setting a sandbox ID
	// Used when sandbox creation fails and we need to allow another attempt
	DropPendingCreation(ctx context.Context, idempotenceKey string) error
}

// Store defines the interface for an idempotence store
type Store interface {
	// ClaimOrWait tries to claim the idempotence key, and if unable, waits for it to complete
	// Returns a ClaimResult indicating whether we claimed it, got an existing sandbox, or encountered an error
	// Uses the context for timeout/cancellation
	ClaimOrWait(ctx context.Context, idempotenceKey string) ClaimResult

	// IsSandboxReleased checks if a sandbox was recently released
	// Returns true if the sandbox was marked as released, false otherwise
	IsSandboxReleased(ctx context.Context, sandboxID types.SandboxID) (bool, error)

	// GetProjectSandbox returns the sandbox ID for a given project, or empty string if not found
	GetProjectSandbox(ctx context.Context, projectID types.ProjectID) (types.SandboxID, error)

	// SetProjectSandbox stores the sandbox ID for a given project
	SetProjectSandbox(ctx context.Context, projectID types.ProjectID, sandboxID types.SandboxID) error

	// FindProjectForSandbox finds the project ID associated with a sandbox
	// Returns the project ID if found, empty string if not found
	FindProjectForSandbox(ctx context.Context, sandboxID types.SandboxID) (types.ProjectID, error)

	// RemoveSandboxMapping removes all mappings for a sandbox (idempotence key and project)
	RemoveSandboxMapping(ctx context.Context, sandboxID types.SandboxID) error

	// Close cleans up resources used by the store
	Close() error
}
