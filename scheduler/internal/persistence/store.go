package persistence

import (
	"context"
	"time"
)

// Store defines the interface for an idempotence store
type Store interface {
	// GetSandboxID returns the sandbox ID for a given idempotence key, or empty string if not found
	GetSandboxID(ctx context.Context, idempotenceKey string) (string, error)

	// StoreSandboxID stores the sandbox ID for a given idempotence key
	StoreSandboxID(ctx context.Context, idempotenceKey, sandboxID string, ttl time.Duration) error

	// MarkPendingCreation marks that a sandbox is being created for this idempotence key
	// Returns true if successfully marked, false if already marked
	MarkPendingCreation(ctx context.Context, idempotenceKey string, ttl time.Duration) (bool, error)

	// CompletePendingCreation updates a pending sandbox creation with its final sandboxID
	CompletePendingCreation(ctx context.Context, idempotenceKey, sandboxID string, ttl time.Duration) error

	// ReleaseIdempotenceKey removes the idempotence key
	ReleaseIdempotenceKey(ctx context.Context, idempotenceKey string) error

	// MarkSandboxReleased marks a sandbox as recently released
	// This helps avoid unnecessary calls to K8s when a sandbox is already deleted
	MarkSandboxReleased(ctx context.Context, sandboxID string, ttl time.Duration) error

	// IsSandboxReleased checks if a sandbox was recently released
	// Returns true if the sandbox was marked as released, false otherwise
	IsSandboxReleased(ctx context.Context, sandboxID string) (bool, error)

	// UnmarkSandboxReleased removes the released mark for a sandbox
	// Used when a release operation fails and we need to try again later
	UnmarkSandboxReleased(ctx context.Context, sandboxID string) error

	// SetSandboxExpiration sets the expiration time for a sandbox
	// The expiration time is a Unix timestamp after which the sandbox should be deleted
	SetSandboxExpiration(ctx context.Context, sandboxID string, expiration time.Time) error

	// ExtendSandboxExpiration extends the expiration time for a sandbox by the given duration
	// Returns the new expiration time
	ExtendSandboxExpiration(ctx context.Context, sandboxID string, extension time.Duration) (time.Time, error)

	// GetExpiredSandboxes returns a list of sandbox IDs that have expired (expiration time <= now)
	// The limit parameter specifies the maximum number of sandboxes to return
	GetExpiredSandboxes(ctx context.Context, now time.Time, limit int) ([]string, error)

	// RemoveSandboxExpiration removes the expiration tracking for a sandbox
	// Used when a sandbox is explicitly released
	RemoveSandboxExpiration(ctx context.Context, sandboxID string) error

	// GetProjectSandbox returns the sandbox ID for a given project, or empty string if not found
	GetProjectSandbox(ctx context.Context, projectID string) (string, error)

	// SetProjectSandbox stores the sandbox ID for a given project
	SetProjectSandbox(ctx context.Context, projectID, sandboxID string) error

	// RemoveProjectSandbox removes the project-sandbox mapping
	RemoveProjectSandbox(ctx context.Context, projectID string) error

	// GetSandboxExpiration returns the expiration time for a sandbox
	GetSandboxExpiration(ctx context.Context, sandboxID string) (time.Time, error)

	// IsSandboxValid checks if a sandbox is still valid (not expired)
	// Returns true if the sandbox exists and is not expired, false otherwise
	IsSandboxValid(ctx context.Context, sandboxID string) (bool, error)

	// FindProjectForSandbox finds the project ID associated with a sandbox
	// Returns the project ID if found, empty string if not found
	FindProjectForSandbox(ctx context.Context, sandboxID string) (string, error)

	// FindIdempotenceKeyForSandbox finds the idempotence key associated with a sandbox
	// Returns the idempotence key if found, empty string if not found
	FindIdempotenceKeyForSandbox(ctx context.Context, sandboxID string) (string, error)

	// RemoveSandboxMapping removes all mappings for a sandbox (idempotence key and project)
	RemoveSandboxMapping(ctx context.Context, sandboxID string) error

	// Close cleans up resources used by the store
	Close() error
}

// Config defines configuration options for the idempotence store
type Config struct {
	Type     string // "memory" or "redis"
	RedisURI string // Redis connection URI (e.g. "redis://:password@localhost:6379/0")
}

// NewStore creates and returns an appropriate idempotence store implementation
// based on the provided config
func NewStore(cfg Config) (Store, error) {
	switch cfg.Type {
	case "memory":
		return NewMemoryStore(), nil
	case "redis":
		return newRedisStore(cfg.RedisURI)
	default:
		// Default to memory store if unspecified
		return NewMemoryStore(), nil
	}
}
