package persistence

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
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
	// Use default database if not specified

	opts := &redis.Options{
		Addr:     uri.Host,
		Password: password,
		DB:       db,
	}

	client := redis.NewClient(opts)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

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
func (r *redisStore) formReleasedKey(sandboxID string) string {
	return r.keyPrefix + "released:" + sandboxID
}

// formExpirationKey returns the key name for the sorted set containing sandbox expirations
func (r *redisStore) formExpirationKey() string {
	return r.keyPrefix + "expirations"
}

// formProjectSandboxKey creates a Redis key for project-sandbox mapping
func (r *redisStore) formProjectSandboxKey(projectID string) string {
	return r.keyPrefix + "project:" + projectID
}

// formSandboxProjectKey creates a Redis key for sandbox-project mapping (reverse mapping)
func (r *redisStore) formSandboxProjectKey(sandboxID string) string {
	return r.keyPrefix + "sandbox:" + sandboxID
}

// formSandboxIdempotenceKey creates a Redis key for sandbox-idempotence mapping (reverse mapping)
func (r *redisStore) formSandboxIdempotenceKey(sandboxID string) string {
	return r.keyPrefix + "sandbox-idempotence:" + sandboxID
}

// GetSandboxID retrieves the sandbox ID for a given idempotence key
func (r *redisStore) GetSandboxID(ctx context.Context, idempotenceKey string) (string, error) {
	key := r.formKey(idempotenceKey)
	sandboxID, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to get sandboxID for key %s: %w", idempotenceKey, err)
	}
	return sandboxID, nil
}

// StoreSandboxID stores the sandbox ID for a given idempotence key
func (r *redisStore) StoreSandboxID(ctx context.Context, idempotenceKey, sandboxID string, ttl time.Duration) error {
	// Use a pipeline to set both mappings in a single round-trip
	pipe := r.client.Pipeline()

	// Set the idempotence key -> sandbox mapping
	key := r.formKey(idempotenceKey)
	pipe.Set(ctx, key, sandboxID, ttl)

	// Set the sandbox -> idempotence key mapping (reverse mapping)
	sandboxKey := r.formSandboxIdempotenceKey(sandboxID)
	pipe.Set(ctx, sandboxKey, idempotenceKey, ttl)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to store sandboxID for key %s: %w", idempotenceKey, err)
	}
	return nil
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
func (r *redisStore) CompletePendingCreation(ctx context.Context, idempotenceKey, sandboxID string, ttl time.Duration) error {
	pipe := r.client.TxPipeline()

	// Store the sandbox ID with the idempotence key
	key := r.formKey(idempotenceKey)
	pipe.Set(ctx, key, sandboxID, ttl)

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

// ReleaseIdempotenceKey removes the idempotence key
func (r *redisStore) ReleaseIdempotenceKey(ctx context.Context, idempotenceKey string) error {
	// Get the sandbox ID first
	sandboxID, err := r.GetSandboxID(ctx, idempotenceKey)
	if err != nil {
		if err == ErrNotFound {
			// No mapping exists, nothing to remove
			return nil
		}
		return fmt.Errorf("failed to get sandboxID for key %s: %w", idempotenceKey, err)
	}

	// Use pipeline to delete all related keys in a single round-trip
	pipe := r.client.Pipeline()

	// Delete the idempotence key -> sandbox mapping
	key := r.formKey(idempotenceKey)
	pipe.Del(ctx, key)

	// Delete the sandbox -> idempotence key mapping
	sandboxKey := r.formSandboxIdempotenceKey(sandboxID)
	pipe.Del(ctx, sandboxKey)

	// Delete the pending key
	pendingKey := r.formPendingKey(idempotenceKey)
	pipe.Del(ctx, pendingKey)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to release idempotence key %s: %w", idempotenceKey, err)
	}

	return nil
}

// MarkSandboxReleased marks a sandbox as recently released
func (r *redisStore) MarkSandboxReleased(ctx context.Context, sandboxID string, ttl time.Duration) error {
	key := r.formReleasedKey(sandboxID)
	err := r.client.Set(ctx, key, "1", ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to mark sandbox %s as released: %w", sandboxID, err)
	}
	return nil
}

// IsSandboxReleased checks if a sandbox was recently released
func (r *redisStore) IsSandboxReleased(ctx context.Context, sandboxID string) (bool, error) {
	key := r.formReleasedKey(sandboxID)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check if sandbox %s is released: %w", sandboxID, err)
	}
	return exists > 0, nil
}

// UnmarkSandboxReleased removes the released mark for a sandbox
func (r *redisStore) UnmarkSandboxReleased(ctx context.Context, sandboxID string) error {
	key := r.formReleasedKey(sandboxID)
	_, err := r.client.Del(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to unmark sandbox %s as released: %w", sandboxID, err)
	}
	return nil
}

// SetSandboxExpiration sets the expiration time for a sandbox
func (r *redisStore) SetSandboxExpiration(ctx context.Context, sandboxID string, expiration time.Time) error {
	// Use a sorted set where the score is the expiration timestamp
	expKey := r.formExpirationKey()
	score := float64(expiration.Unix())

	// Add to sorted set with score as expiration time
	err := r.client.ZAdd(ctx, expKey, redis.Z{
		Score:  score,
		Member: sandboxID,
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to set expiration for sandbox %s: %w", sandboxID, err)
	}

	return nil
}

// ExtendSandboxExpiration extends the expiration time for a sandbox
func (r *redisStore) ExtendSandboxExpiration(ctx context.Context, sandboxID string, extension time.Duration) (time.Time, error) {
	expKey := r.formExpirationKey()

	// Get current score (expiration time)
	currentScore, err := r.client.ZScore(ctx, expKey, sandboxID).Result()
	if err != nil {
		if err == redis.Nil {
			// Sandbox not found in the expiration set, use current time as base
			currentScore = float64(time.Now().Unix())
		} else {
			return time.Time{}, fmt.Errorf("failed to get current expiration for sandbox %s: %w", sandboxID, err)
		}
	}

	// Calculate new expiration time
	newExpiration := time.Unix(int64(currentScore), 0).Add(extension)
	newScore := float64(newExpiration.Unix())

	// Update the sorted set with the new score
	err = r.client.ZAdd(ctx, expKey, redis.Z{
		Score:  newScore,
		Member: sandboxID,
	}).Err()

	if err != nil {
		return time.Time{}, fmt.Errorf("failed to extend expiration for sandbox %s: %w", sandboxID, err)
	}

	return newExpiration, nil
}

// GetExpiredSandboxes returns a list of sandbox IDs that have expired
func (r *redisStore) GetExpiredSandboxes(ctx context.Context, now time.Time, limit int) ([]string, error) {
	expKey := r.formExpirationKey()
	maxScore := float64(now.Unix())

	// Get all members with score <= now (i.e., expired sandboxes)
	results, err := r.client.ZRangeByScore(ctx, expKey, &redis.ZRangeBy{
		Min:    "-inf",
		Max:    fmt.Sprintf("%f", maxScore),
		Offset: 0,
		Count:  int64(limit),
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to get expired sandboxes: %w", err)
	}

	return results, nil
}

// RemoveSandboxExpiration removes the expiration tracking for a sandbox
func (r *redisStore) RemoveSandboxExpiration(ctx context.Context, sandboxID string) error {
	expKey := r.formExpirationKey()

	// Remove from the sorted set
	err := r.client.ZRem(ctx, expKey, sandboxID).Err()
	if err != nil {
		return fmt.Errorf("failed to remove expiration for sandbox %s: %w", sandboxID, err)
	}

	return nil
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
func (r *redisStore) GetProjectSandbox(ctx context.Context, projectID string) (string, error) {
	key := r.formProjectSandboxKey(projectID)
	sandboxID, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to get sandbox for project %s: %w", projectID, err)
	}
	return sandboxID, nil
}

// SetProjectSandbox stores the sandbox ID for a given project
func (r *redisStore) SetProjectSandbox(ctx context.Context, projectID, sandboxID string) error {
	// Set a reasonable TTL for the project-sandbox mapping (24 hours)
	const projectSandboxTTL = 24 * time.Hour

	// Use a pipeline to set both mappings in a single round-trip
	pipe := r.client.Pipeline()

	// Set the project -> sandbox mapping
	projectKey := r.formProjectSandboxKey(projectID)
	pipe.Set(ctx, projectKey, sandboxID, projectSandboxTTL)

	// Set the sandbox -> project mapping (reverse mapping)
	sandboxKey := r.formSandboxProjectKey(sandboxID)
	pipe.Set(ctx, sandboxKey, projectID, projectSandboxTTL)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to set project-sandbox mapping: %w", err)
	}

	return nil
}

// RemoveProjectSandbox removes the project-sandbox mapping
func (r *redisStore) RemoveProjectSandbox(ctx context.Context, projectID string) error {
	// Get the sandbox ID first
	sandboxID, err := r.GetProjectSandbox(ctx, projectID)
	if err != nil {
		if err == ErrNotFound {
			// No mapping exists, nothing to remove
			return nil
		}
		return fmt.Errorf("failed to get sandbox for project %s: %w", projectID, err)
	}

	// Use a pipeline to remove both mappings in a single round-trip
	pipe := r.client.Pipeline()

	// Remove the project -> sandbox mapping
	projectKey := r.formProjectSandboxKey(projectID)
	pipe.Del(ctx, projectKey)

	// Remove the sandbox -> project mapping (reverse mapping)
	sandboxKey := r.formSandboxProjectKey(sandboxID)
	pipe.Del(ctx, sandboxKey)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove project-sandbox mapping: %w", err)
	}

	return nil
}

// FindProjectForSandbox finds the project ID associated with a sandbox
func (r *redisStore) FindProjectForSandbox(ctx context.Context, sandboxID string) (string, error) {
	key := r.formSandboxProjectKey(sandboxID)
	projectID, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil // Not found, return empty string
		}
		return "", fmt.Errorf("failed to get project for sandbox %s: %w", sandboxID, err)
	}
	return projectID, nil
}

// GetSandboxExpiration returns the expiration time for a sandbox
func (r *redisStore) GetSandboxExpiration(ctx context.Context, sandboxID string) (time.Time, error) {
	expKey := r.formExpirationKey()

	// Get the score (expiration time) for the sandbox
	score, err := r.client.ZScore(ctx, expKey, sandboxID).Result()
	if err != nil {
		if err == redis.Nil {
			return time.Time{}, fmt.Errorf("sandbox %s not found", sandboxID)
		}
		return time.Time{}, fmt.Errorf("failed to get expiration for sandbox %s: %w", sandboxID, err)
	}

	// Convert score (Unix timestamp) to time.Time
	return time.Unix(int64(score), 0), nil
}

// IsSandboxValid checks if a sandbox is still valid (not expired)
func (r *redisStore) IsSandboxValid(ctx context.Context, sandboxID string) (bool, error) {
	expKey := r.formExpirationKey()

	// Get the score (expiration time) for the sandbox
	score, err := r.client.ZScore(ctx, expKey, sandboxID).Result()
	if err != nil {
		if err == redis.Nil {
			return false, fmt.Errorf("sandbox %s not found", sandboxID)
		}
		return false, fmt.Errorf("failed to get expiration for sandbox %s: %w", sandboxID, err)
	}

	// Convert score (Unix timestamp) to time.Time
	expirationTime := time.Unix(int64(score), 0)

	// Check if the sandbox is still valid
	return time.Now().Before(expirationTime), nil
}

// FindIdempotenceKeyForSandbox finds the idempotence key associated with a sandbox
func (r *redisStore) FindIdempotenceKeyForSandbox(ctx context.Context, sandboxID string) (string, error) {
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
func (r *redisStore) RemoveSandboxMapping(ctx context.Context, sandboxID string) error {
	// Get the idempotence key first
	idempotenceKey, err := r.FindIdempotenceKeyForSandbox(ctx, sandboxID)
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

	// Remove the expiration tracking
	expKey := r.formExpirationKey()
	pipe.ZRem(ctx, expKey, sandboxID)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove sandbox mapping for %s: %w", sandboxID, err)
	}

	return nil
}
