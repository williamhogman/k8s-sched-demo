package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

// IdempotenceStore implementation using Redis
type redisIdempotenceStore struct {
	client    *redis.Client
	keyPrefix string
}

// NewIdempotenceStore creates a new Redis-backed idempotence store
func NewIdempotenceStore(client *redis.Client, keyPrefix string) (IdempotenceStore, error) {
	if client == nil {
		return nil, errors.New("redis client is required")
	}

	if keyPrefix == "" {
		keyPrefix = defaultKeyPrefix
	}

	return &redisIdempotenceStore{
		client:    client,
		keyPrefix: keyPrefix,
	}, nil
}

// formKey creates a Redis key with proper prefix for an idempotence key
func (r *redisIdempotenceStore) formKey(idempotenceKey string) string {
	return r.keyPrefix + "idem:" + idempotenceKey
}

// formPendingKey creates a Redis key for pending sandbox creation
func (r *redisIdempotenceStore) formPendingKey(idempotenceKey string) string {
	return r.keyPrefix + "pending:" + idempotenceKey
}

// formSandboxIdempotenceKey creates a Redis key for sandbox-idempotence mapping
func (r *redisIdempotenceStore) formSandboxIdempotenceKey(sandboxID types.SandboxID) string {
	return r.keyPrefix + "sandbox-idempotence:" + sandboxID.String()
}

// GetSandboxID retrieves the sandbox ID for a given idempotence key
func (r *redisIdempotenceStore) GetSandboxID(ctx context.Context, idempotenceKey string) (types.SandboxID, error) {
	key := r.formKey(idempotenceKey)
	sandboxIDStr, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to get sandboxID for key %s: %w", idempotenceKey, err)
	}
	sandboxID, err := types.NewSandboxID(sandboxIDStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse sandboxID for key %s: %w", idempotenceKey, err)
	}
	return sandboxID, nil
}

// MarkPendingCreation marks that a sandbox is being created for this idempotence key
func (r *redisIdempotenceStore) MarkPendingCreation(ctx context.Context, idempotenceKey string) (bool, error) {
	pendingKey := r.formPendingKey(idempotenceKey)
	set, err := r.client.SetNX(ctx, pendingKey, "1", pendingKeyTTL).Result()
	if err != nil {
		return false, fmt.Errorf("failed to mark pending creation for key %s: %w", idempotenceKey, err)
	}
	return set, nil
}

// CompletePendingCreation updates a pending sandbox creation with its final sandboxID
func (r *redisIdempotenceStore) CompletePendingCreation(ctx context.Context, idempotenceKey string, sandboxID types.SandboxID) error {
	pipe := r.client.TxPipeline()

	// Store the sandbox ID with the idempotence key
	key := r.formKey(idempotenceKey)
	pipe.Set(ctx, key, sandboxID.String(), idempotenceKeyTTL)

	// Set the sandbox -> idempotence key mapping (reverse mapping)
	sandboxKey := r.formSandboxIdempotenceKey(sandboxID)
	pipe.Set(ctx, sandboxKey, idempotenceKey, idempotenceKeyTTL)

	// Remove the pending key
	pendingKey := r.formPendingKey(idempotenceKey)
	pipe.Del(ctx, pendingKey)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to complete pending creation for key %s: %w", idempotenceKey, err)
	}

	return nil
}

// DropPendingCreation removes a pending creation marker without setting a sandbox ID
func (r *redisIdempotenceStore) DropPendingCreation(ctx context.Context, idempotenceKey string) error {
	pendingKey := r.formPendingKey(idempotenceKey)
	deleted, err := r.client.Del(ctx, pendingKey).Result()
	if err != nil {
		return fmt.Errorf("failed to drop pending creation for key %s: %w", idempotenceKey, err)
	}

	if deleted == 0 {
		// Key didn't exist, but that's not an error for dropping
		return nil
	}

	return nil
}

// awaitIdempotenceCompletion waits for the idempotence key to be completed
func (r *redisIdempotenceStore) awaitIdempotenceCompletion(ctx context.Context, idempotenceKey string) (types.SandboxID, error) {
	// Check immediately first before polling
	sandboxID, err := r.GetSandboxID(ctx, idempotenceKey)
	if err == nil && sandboxID != "" {
		// Already completed, return immediately
		return sandboxID, nil
	} else if err != nil && err != ErrNotFound {
		// Return actual errors (not just key not found)
		return "", fmt.Errorf("error checking idempotence key: %w", err)
	}

	// If not found, start polling
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	// Use the provided context for timeout/cancellation
	for {
		select {
		case <-ctx.Done():
			// Context was canceled or timed out
			return "", ctx.Err()
		case <-ticker.C:
			// Check if the sandbox ID is now available
			sandboxID, err := r.GetSandboxID(ctx, idempotenceKey)
			if err == nil && sandboxID != "" {
				// Found a completed sandbox ID, return success
				return sandboxID, nil
			} else if err != nil && err != ErrNotFound {
				// Return actual errors (not just key not found)
				return "", fmt.Errorf("error checking idempotence key: %w", err)
			}

			// Check if the pending key still exists
			pendingKey := r.formPendingKey(idempotenceKey)
			exists, err := r.client.Exists(ctx, pendingKey).Result()
			if err != nil {
				return "", fmt.Errorf("error checking pending key: %w", err)
			}
			// If pending key no longer exists but we still don't have a sandbox ID,
			// it means the concurrent creation was abandoned/failed
			if exists == 0 {
				return "", ErrConcurrentCreationFailed
			}
		}
	}
}

// ClaimOrWait tries to claim the idempotence key, and if unable, waits for it to complete
func (r *redisIdempotenceStore) ClaimOrWait(ctx context.Context, idempotenceKey string) ClaimResult {
	// Check for context cancellation first
	if err := ctx.Err(); err != nil {
		return ClaimResult{
			Claimed:        false,
			SandboxID:      "",
			Err:            fmt.Errorf("context error: %w", err),
			store:          r,
			idempotenceKey: idempotenceKey,
		}
	}

	// First check for an existing sandbox ID
	sandboxID, err := r.GetSandboxID(ctx, idempotenceKey)
	if err == nil && sandboxID != "" {
		// We found an existing sandbox, return it but indicate we didn't claim
		return ClaimResult{
			SandboxID:      sandboxID,
			store:          r,
			idempotenceKey: idempotenceKey,
		}
	} else if err != nil && err != ErrNotFound {
		// Return actual errors (not just key not found)
		return ClaimResult{
			Err:            fmt.Errorf("error checking for existing sandbox: %w", err),
			store:          r,
			idempotenceKey: idempotenceKey,
		}
	}

	// Try to mark this key as pending creation
	marked, err := r.MarkPendingCreation(ctx, idempotenceKey)
	if err != nil {
		return ClaimResult{
			Err:            fmt.Errorf("error marking pending creation: %w", err),
			store:          r,
			idempotenceKey: idempotenceKey,
		}
	}

	if marked {
		// We successfully claimed the key
		return ClaimResult{
			Claimed:        true,
			store:          r,
			idempotenceKey: idempotenceKey,
		}
	}

	// We couldn't claim it - wait for whoever is creating it to finish
	// Use the same context as provided to honor any timeout/cancellation
	sandboxID, err = r.awaitIdempotenceCompletion(ctx, idempotenceKey)
	if err != nil {
		return ClaimResult{
			Err:            fmt.Errorf("error waiting for idempotence completion: %w", err),
			store:          r,
			idempotenceKey: idempotenceKey,
		}
	}

	// Successfully waited for completion
	return ClaimResult{
		SandboxID:      sandboxID,
		store:          r,
		idempotenceKey: idempotenceKey,
	}
}

// TODO: Add method for releasing the idempotence key
