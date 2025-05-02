package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

const (
	readySandboxesKey   = "sandboxes:ready"
	pendingSandboxesKey = "sandboxes:pending"
)

// RedisSandboxPoolStore implements SandboxPoolStore using Redis
type RedisSandboxPoolStore struct {
	client    *redis.Client
	keyPrefix string
}

// Ensure RedisSandboxPoolStore implements the SandboxPoolStore interface
var _ SandboxPoolStore = (*RedisSandboxPoolStore)(nil)

// NewRedisSandboxPoolStore creates a new Redis-based sandbox pool store
func NewRedisSandboxPoolStore(client *redis.Client, keyPrefix string) (*RedisSandboxPoolStore, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client cannot be nil")
	}

	return &RedisSandboxPoolStore{
		client:    client,
		keyPrefix: keyPrefix,
	}, nil
}

// getKey returns a prefixed key for Redis
func (s *RedisSandboxPoolStore) getKey(key string) string {
	return fmt.Sprintf("%s:%s", s.keyPrefix, key)
}

// AddReadySandbox adds a ready sandbox to the pool
func (s *RedisSandboxPoolStore) AddReadySandbox(ctx context.Context, sandboxID types.SandboxID) error {
	now := time.Now().Unix()
	key := s.getKey(readySandboxesKey)

	// Add to the sorted set with current timestamp as score
	_, err := s.client.ZAdd(ctx, key, redis.Z{
		Score:  float64(now),
		Member: sandboxID.String(),
	}).Result()

	return err
}

// GetReadySandbox gets and removes a ready sandbox from the pool
func (s *RedisSandboxPoolStore) GetReadySandbox(ctx context.Context) (types.SandboxID, error) {
	key := s.getKey(readySandboxesKey)

	// Get the oldest sandbox (lowest score) and remove it atomically
	result, err := s.client.ZPopMin(ctx, key, 1).Result()
	if err != nil {
		return "", err
	}

	if len(result) == 0 {
		return "", ErrPoolEmpty
	}

	return types.NewSandboxID(result[0].Member.(string))
}

// CountReadySandboxes returns the number of ready sandboxes in the pool
func (s *RedisSandboxPoolStore) CountReadySandboxes(ctx context.Context) (int64, error) {
	key := s.getKey(readySandboxesKey)
	return s.client.ZCard(ctx, key).Result()
}

// AddPendingSandbox adds a sandbox to the pending set with creation timestamp
func (s *RedisSandboxPoolStore) AddPendingSandbox(ctx context.Context, sandboxID types.SandboxID) error {
	now := time.Now().Unix()
	key := s.getKey(pendingSandboxesKey)

	// Add to the sorted set with current timestamp as score
	_, err := s.client.ZAdd(ctx, key, redis.Z{
		Score:  float64(now),
		Member: sandboxID.String(),
	}).Result()

	return err
}

// GetPendingSandboxes returns the list of pending sandboxes
func (s *RedisSandboxPoolStore) GetPendingSandboxes(ctx context.Context) ([]types.SandboxID, error) {
	key := s.getKey(pendingSandboxesKey)

	// Get all pending sandboxes sorted by time
	result, err := s.client.ZRangeWithScores(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	sandboxes := make([]types.SandboxID, 0, len(result))
	for _, z := range result {
		sandboxID, err := types.NewSandboxID(z.Member.(string))
		if err != nil {
			continue // Skip invalid sandbox IDs
		}
		sandboxes = append(sandboxes, sandboxID)
	}

	return sandboxes, nil
}

// RemovePendingSandbox removes a pending sandbox from the pending set
func (s *RedisSandboxPoolStore) RemovePendingSandbox(ctx context.Context, sandboxID types.SandboxID) error {
	key := s.getKey(pendingSandboxesKey)

	_, err := s.client.ZRem(ctx, key, sandboxID.String()).Result()
	return err
}

// CountPendingSandboxes returns the number of pending sandboxes
func (s *RedisSandboxPoolStore) CountPendingSandboxes(ctx context.Context) (int64, error) {
	key := s.getKey(pendingSandboxesKey)
	return s.client.ZCard(ctx, key).Result()
}
