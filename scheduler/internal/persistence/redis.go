package persistence

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

const (
	pendingKeyTTL        = 1 * time.Minute
	idempotenceKeyTTL    = 10 * time.Minute
	defaultKeyPrefix     = "sch:"
	projectUpdateChannel = "project-sandbox-updates"
	projectSetKey        = "all-projects-set" // Set containing all project IDs
)

// redisStore implements the Store interface using Redis
type redisStore struct {
	client    *redis.Client
	keyPrefix string
}

// newRedisClient creates a new Redis client from a URI
func newRedisClient(redisURI string) (*redis.Client, error) {
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

	return redis.NewClient(opts), nil
}

// newRedisStore creates a new Redis-backed store
func newRedisStore(client *redis.Client, keyPrefix string) (*redisStore, error) {
	if client == nil {
		return nil, errors.New("redis client is required")
	}

	if keyPrefix == "" {
		keyPrefix = defaultKeyPrefix
	}

	return &redisStore{
		client:    client,
		keyPrefix: keyPrefix,
	}, nil
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

// formAllProjectsSetKey creates a Redis key for the set of all projects
func (r *redisStore) formAllProjectsSetKey() string {
	return r.keyPrefix + projectSetKey
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

	// Add to the set of all projects
	allProjectsKey := r.formAllProjectsSetKey()
	pipe.SAdd(ctx, allProjectsKey, projectID.String())
	pipe.Expire(ctx, allProjectsKey, projectSandboxTTL)

	// Publish an update notification
	updateMsg := fmt.Sprintf("%s:update:%s", projectID.String(), sandboxID.String())
	pipe.Publish(ctx, projectUpdateChannel, updateMsg)

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

// RemoveSandboxMapping removes all mappings for a sandbox (project)
func (r *redisStore) RemoveSandboxMapping(ctx context.Context, sandboxID types.SandboxID) error {
	// Get the project ID
	projectID, err := r.FindProjectForSandbox(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("failed to get project for sandbox %s: %w", sandboxID, err)
	}

	// Use a pipeline to remove all mappings in a single round-trip
	pipe := r.client.Pipeline()

	// Remove the sandbox -> project mapping if we found a project
	if projectID != "" {
		sandboxProjectKey := r.formSandboxProjectKey(sandboxID)
		pipe.Del(ctx, sandboxProjectKey)
	}

	// Remove the project -> sandbox mapping if we found a project
	if projectID != "" {
		projectSandboxKey := r.formProjectSandboxKey(projectID)
		pipe.Del(ctx, projectSandboxKey)

		// Remove from set of all projects
		allProjectsKey := r.formAllProjectsSetKey()
		pipe.SRem(ctx, allProjectsKey, projectID.String())

		// Publish a removal notification
		updateMsg := fmt.Sprintf("%s:remove:", projectID.String())
		pipe.Publish(ctx, projectUpdateChannel, updateMsg)
	}

	releasedKey := r.formReleasedKey(sandboxID)
	pipe.Set(ctx, releasedKey, "1", 5*time.Minute)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove sandbox mapping for %s: %w", sandboxID, err)
	}

	return nil
}

// SubscribeToProjectSandboxUpdates subscribes to project-sandbox mapping updates.
// The callback will be called whenever a project-sandbox mapping is updated or removed.
func (r *redisStore) SubscribeToProjectSandboxUpdates(ctx context.Context, callback func(projectID types.ProjectID, sandboxID types.SandboxID, isRemoval bool)) error {
	pubsub := r.client.Subscribe(ctx, projectUpdateChannel)
	defer pubsub.Close()

	// Listen for messages in a goroutine
	go func() {
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				return
			}

			// Parse the message
			parts := strings.SplitN(msg.Payload, ":", 3)
			if len(parts) != 3 {
				continue
			}

			projectID, err := types.NewProjectID(parts[0])
			if err != nil {
				continue
			}

			isRemoval := parts[1] == "remove"

			if isRemoval {
				// For removals, call with empty sandboxID
				callback(projectID, "", true)
			} else {
				// For updates, parse the sandboxID
				sandboxID, err := types.NewSandboxID(parts[2])
				if err != nil {
					continue
				}
				callback(projectID, sandboxID, false)
			}
		}
	}()

	return nil
}

// GetAllProjectSandboxMappings returns all project-sandbox mappings
func (r *redisStore) GetAllProjectSandboxMappings(ctx context.Context) (map[types.ProjectID]types.SandboxID, error) {
	// Get all project IDs from the set
	allProjectsKey := r.formAllProjectsSetKey()
	projectIDStrs, err := r.client.SMembers(ctx, allProjectsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get all project IDs: %w", err)
	}

	if len(projectIDStrs) == 0 {
		return make(map[types.ProjectID]types.SandboxID), nil
	}

	// Create a pipeline to batch get all sandbox IDs
	pipe := r.client.Pipeline()
	cmds := make(map[string]*redis.StringCmd)

	// Queue up all the GET commands
	for _, projectIDStr := range projectIDStrs {
		projectID, err := types.NewProjectID(projectIDStr)
		if err != nil {
			continue
		}
		key := r.formProjectSandboxKey(projectID)
		cmds[projectIDStr] = pipe.Get(ctx, key)
	}

	// Execute the pipeline
	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get all project-sandbox mappings: %w", err)
	}

	// Process the results
	result := make(map[types.ProjectID]types.SandboxID)
	for projectIDStr, cmd := range cmds {
		sandboxIDStr, err := cmd.Result()
		if err != nil {
			if err == redis.Nil {
				continue // Skip if the mapping doesn't exist anymore
			}
			return nil, fmt.Errorf("failed to get sandbox for project %s: %w", projectIDStr, err)
		}

		projectID, err := types.NewProjectID(projectIDStr)
		if err != nil {
			continue
		}

		sandboxID, err := types.NewSandboxID(sandboxIDStr)
		if err != nil {
			continue
		}

		result[projectID] = sandboxID
	}

	return result, nil
}
