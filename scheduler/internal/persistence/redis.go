package persistence

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

// redisStore implements the Store interface using Redis
type redisStore struct {
	client    *redis.Client
	keyPrefix string
}

// newRedisStore creates a new Redis-backed idempotence store
func newRedisStore(redisURI string) (Store, error) {
	if redisURI == "" {
		return nil, errors.New("redis URI is required")
	}

	uri, err := url.Parse(redisURI)
	if err != nil {
		return nil, fmt.Errorf("invalid Redis URI: %w", err)
	}

	password := ""
	if uri.User != nil {
		password, _ = uri.User.Password()
	}

	db := 0

	opts := &redis.Options{
		Addr:     uri.Host,
		Password: password,
		DB:       db,
	}

	client := redis.NewClient(opts)

	return &redisStore{
		client:    client,
		keyPrefix: "idempotence:",
	}, nil
}

// formKey creates a Redis key with proper prefix for an idempotence key
func (r *redisStore) formKey(idempotenceKey string) string {
	return r.keyPrefix + idempotenceKey
}

// formPendingKey creates a Redis key for pending sandbox creation
func (r *redisStore) formPendingKey(idempotenceKey string) string {
	return r.keyPrefix + "pending:" + idempotenceKey
}

// formReleasedKey creates a Redis key for marking a sandbox as released
func (r *redisStore) formReleasedKey(sandboxID types.SandboxID) string {
	return r.keyPrefix + "released:" + sandboxID.String()
}

// formProjectSandboxKey creates a Redis key for project-sandbox mapping
func (r *redisStore) formProjectSandboxKey(projectID types.ProjectID) string {
	return r.keyPrefix + "project:" + projectID.String()
}

// formSandboxProjectKey creates a Redis key for sandbox-project mapping (reverse mapping)
func (r *redisStore) formSandboxProjectKey(sandboxID types.SandboxID) string {
	return r.keyPrefix + "sandbox:" + sandboxID.String()
}

// formSandboxIdempotenceKey creates a Redis key for sandbox-idempotence mapping (reverse mapping)
func (r *redisStore) formSandboxIdempotenceKey(sandboxID types.SandboxID) string {
	return r.keyPrefix + "sandbox-idempotence:" + sandboxID.String()
}

// GetSandboxID retrieves the sandbox ID for a given idempotence key
func (r *redisStore) GetSandboxID(ctx context.Context, idempotenceKey string) (types.SandboxID, error) {
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
func (r *redisStore) MarkPendingCreation(ctx context.Context, idempotenceKey string, ttl time.Duration) (bool, error) {
	pendingKey := r.formPendingKey(idempotenceKey)
	set, err := r.client.SetNX(ctx, pendingKey, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to mark pending creation for key %s: %w", idempotenceKey, err)
	}
	return set, nil
}

// CompletePendingCreation updates a pending sandbox creation with its final sandboxID
func (r *redisStore) CompletePendingCreation(ctx context.Context, idempotenceKey string, sandboxID types.SandboxID, ttl time.Duration) error {
	pipe := r.client.TxPipeline()

	// Store the sandbox ID with the idempotence key
	key := r.formKey(idempotenceKey)
	pipe.Set(ctx, key, sandboxID.String(), ttl)

	// Set the sandbox -> idempotence key mapping (reverse mapping)
	sandboxKey := r.formSandboxIdempotenceKey(sandboxID)
	pipe.Set(ctx, sandboxKey, idempotenceKey, ttl)

	// Remove the pending key
	pendingKey := r.formPendingKey(idempotenceKey)
	pipe.Del(ctx, pendingKey)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to complete pending creation for key %s: %w", idempotenceKey, err)
	}

	return nil
}

// IsSandboxReleased checks if a sandbox was recently released
func (r *redisStore) IsSandboxReleased(ctx context.Context, sandboxID types.SandboxID) (bool, error) {
	key := r.formReleasedKey(sandboxID)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check if sandbox %s is released: %w", sandboxID, err)
	}
	return exists > 0, nil
}

// Close closes the Redis client connection
func (r *redisStore) Close() error {
	return r.client.Close()
}

// RedisClient returns the underlying Redis client (for testing and diagnostics)
func (r *redisStore) RedisClient() *redis.Client {
	return r.client
}

// GetProjectSandbox returns the sandbox ID for a given project
func (r *redisStore) GetProjectSandbox(ctx context.Context, projectID types.ProjectID) (types.SandboxID, error) {
	key := r.formProjectSandboxKey(projectID)
	sandboxIDStr, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to get sandbox for project %s: %w", projectID.String(), err)
	}
	sandboxID, err := types.NewSandboxID(sandboxIDStr)
	if err != nil {
		return "", fmt.Errorf("invalid sandbox ID: %w", err)
	}
	return sandboxID, nil
}

// SetProjectSandbox stores the sandbox ID for a given project
func (r *redisStore) SetProjectSandbox(ctx context.Context, projectID types.ProjectID, sandboxID types.SandboxID) error {
	// Set a reasonable TTL for the project-sandbox mapping (24 hours)
	const projectSandboxTTL = 24 * time.Hour

	// Use a pipeline to set both mappings in a single round-trip
	pipe := r.client.Pipeline()
	// Set the project -> sandbox mapping
	projectKey := r.formProjectSandboxKey(projectID)
	pipe.Set(ctx, projectKey, sandboxID.String(), projectSandboxTTL)

	// Set the sandbox -> project mapping (reverse mapping)
	sandboxKey := r.formSandboxProjectKey(sandboxID)
	pipe.Set(ctx, sandboxKey, projectID.String(), projectSandboxTTL)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to set project-sandbox mapping: %w", err)
	}

	return nil
}

// FindProjectForSandbox finds the project ID associated with a sandbox
func (r *redisStore) FindProjectForSandbox(ctx context.Context, sandboxID types.SandboxID) (types.ProjectID, error) {
	key := r.formSandboxProjectKey(sandboxID)
	projectIDStr, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil // Not found, return empty string
		}
		return "", fmt.Errorf("failed to get project for sandbox %s: %w", sandboxID, err)
	}

	projectID, err := types.NewProjectID(projectIDStr)
	if err != nil {
		return "", fmt.Errorf("invalid project ID: %w", err)
	}

	return projectID, nil
}

func (r *redisStore) findIdempotenceKeyForSandbox(ctx context.Context, sandboxID types.SandboxID) (string, error) {
	key := r.formSandboxIdempotenceKey(sandboxID)
	idempotenceKey, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil // Not found, return empty string
		}
		return "", fmt.Errorf("failed to get idempotence key for sandbox %s: %w", sandboxID, err)
	}
	return idempotenceKey, nil
}

// RemoveSandboxMapping removes all mappings for a sandbox (idempotence key and project)
func (r *redisStore) RemoveSandboxMapping(ctx context.Context, sandboxID types.SandboxID) error {
	// Get the idempotence key first
	idempotenceKey, err := r.findIdempotenceKeyForSandbox(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("failed to get idempotence key for sandbox %s: %w", sandboxID, err)
	}

	// Get the project ID
	projectID, err := r.FindProjectForSandbox(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("failed to get project for sandbox %s: %w", sandboxID, err)
	}

	// Use a pipeline to remove all mappings in a single round-trip
	pipe := r.client.Pipeline()

	// Remove the sandbox -> idempotence key mapping
	sandboxIdempotenceKey := r.formSandboxIdempotenceKey(sandboxID)
	pipe.Del(ctx, sandboxIdempotenceKey)

	// Remove the idempotence key -> sandbox mapping if we found an idempotence key
	if idempotenceKey != "" {
		idempotenceKey := r.formKey(idempotenceKey)
		pipe.Del(ctx, idempotenceKey)
	}

	// Remove the sandbox -> project mapping if we found a project
	if projectID != "" {
		sandboxProjectKey := r.formSandboxProjectKey(sandboxID)
		pipe.Del(ctx, sandboxProjectKey)
	}

	// Remove the project -> sandbox mapping if we found a project
	if projectID != "" {
		projectSandboxKey := r.formProjectSandboxKey(projectID)
		pipe.Del(ctx, projectSandboxKey)
	}

	releasedKey := r.formReleasedKey(sandboxID)
	pipe.Set(ctx, releasedKey, "1", 5*time.Minute)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove sandbox mapping for %s: %w", sandboxID, err)
	}

	return nil
}
