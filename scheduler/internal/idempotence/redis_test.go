package idempotence

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRedisTest(t *testing.T) (*redisStore, *miniredis.Miniredis, context.Context) {
	// Start a mock Redis server
	s, err := miniredis.Run()
	require.NoError(t, err)

	// Create a new Redis store using the mock Redis server
	store, err := newRedisStore("redis://" + s.Addr())
	require.NoError(t, err)

	redisStore, ok := store.(*redisStore)
	require.True(t, ok, "Expected Redis store implementation")

	return redisStore, s, context.Background()
}

func TestRedisStore_GetSetSandboxID(t *testing.T) {
	store, s, ctx := setupRedisTest(t)
	defer s.Close()
	defer store.Close()

	idempotenceKey := "idem-key-1"
	sandboxID := "sandbox-123"
	ttl := time.Minute

	// Initially, there should be no sandbox ID
	id, err := store.GetSandboxID(ctx, idempotenceKey)
	require.NoError(t, err)
	assert.Empty(t, id)

	// Store a sandbox ID
	err = store.StoreSandboxID(ctx, idempotenceKey, sandboxID, ttl)
	require.NoError(t, err)

	// Verify the sandbox ID is stored
	id, err = store.GetSandboxID(ctx, idempotenceKey)
	require.NoError(t, err)
	assert.Equal(t, sandboxID, id)

	// Check the TTL
	ttlDuration, err := s.TTL(store.formKey(idempotenceKey))
	require.NoError(t, err)
	assert.True(t, ttlDuration > 0)
}

func TestRedisStore_MarkPendingCreation(t *testing.T) {
	store, s, ctx := setupRedisTest(t)
	defer s.Close()
	defer store.Close()

	idempotenceKey := "idem-key-1"
	ttl := time.Minute

	// Mark as pending
	acquired, err := store.MarkPendingCreation(ctx, idempotenceKey, ttl)
	require.NoError(t, err)
	assert.True(t, acquired)

	// Verify the pending key exists
	exists := s.Exists(store.formPendingKey(idempotenceKey))
	assert.True(t, exists)

	// Try to mark as pending again
	acquired, err = store.MarkPendingCreation(ctx, idempotenceKey, ttl)
	require.NoError(t, err)
	assert.False(t, acquired)
}

func TestRedisStore_CompletePendingCreation(t *testing.T) {
	store, s, ctx := setupRedisTest(t)
	defer s.Close()
	defer store.Close()

	idempotenceKey := "idem-key-1"
	sandboxID := "sandbox-123"
	ttl := time.Minute

	// Mark as pending first
	acquired, err := store.MarkPendingCreation(ctx, idempotenceKey, ttl)
	require.NoError(t, err)
	assert.True(t, acquired)

	// Complete the pending creation
	err = store.CompletePendingCreation(ctx, idempotenceKey, sandboxID, ttl)
	require.NoError(t, err)

	// Verify the pending key is removed
	exists := s.Exists(store.formPendingKey(idempotenceKey))
	assert.False(t, exists)

	// Verify the sandbox ID is stored
	id, err := store.GetSandboxID(ctx, idempotenceKey)
	require.NoError(t, err)
	assert.Equal(t, sandboxID, id)
}

func TestRedisStore_ReleaseIdempotenceKey(t *testing.T) {
	store, s, ctx := setupRedisTest(t)
	defer s.Close()
	defer store.Close()

	idempotenceKey := "idem-key-1"
	sandboxID := "sandbox-123"
	ttl := time.Minute

	// Store a sandbox ID
	err := store.StoreSandboxID(ctx, idempotenceKey, sandboxID, ttl)
	require.NoError(t, err)

	// Release the idempotence key
	err = store.ReleaseIdempotenceKey(ctx, idempotenceKey)
	require.NoError(t, err)

	// Verify the sandbox ID is removed
	exists := s.Exists(store.formKey(idempotenceKey))
	assert.False(t, exists)
}

func TestRedisStore_ReleasedSandbox(t *testing.T) {
	store, s, ctx := setupRedisTest(t)
	defer s.Close()
	defer store.Close()

	sandboxID := "sandbox-123"
	ttl := time.Minute

	// Test marking a sandbox as released
	err := store.MarkSandboxReleased(ctx, sandboxID, ttl)
	require.NoError(t, err)

	// Verify the released key exists in Redis
	exists := s.Exists(store.formReleasedKey(sandboxID))
	assert.True(t, exists)

	// Test checking if a sandbox is released
	isReleased, err := store.IsSandboxReleased(ctx, sandboxID)
	require.NoError(t, err)
	assert.True(t, isReleased)

	// Test checking a non-existent sandbox
	isReleased, err = store.IsSandboxReleased(ctx, "non-existent")
	require.NoError(t, err)
	assert.False(t, isReleased)

	// Test unmarking a sandbox
	err = store.UnmarkSandboxReleased(ctx, sandboxID)
	require.NoError(t, err)

	// Verify the key is removed
	exists = s.Exists(store.formReleasedKey(sandboxID))
	assert.False(t, exists)

	// Verify it's no longer marked as released
	isReleased, err = store.IsSandboxReleased(ctx, sandboxID)
	require.NoError(t, err)
	assert.False(t, isReleased)

	// Test unmarking a non-existent sandbox (should not error)
	err = store.UnmarkSandboxReleased(ctx, "non-existent")
	assert.NoError(t, err)
}

func TestRedisStore_TTL(t *testing.T) {
	store, s, ctx := setupRedisTest(t)
	defer s.Close()
	defer store.Close()

	sandboxID := "sandbox-123"
	ttl := 100 * time.Millisecond

	// Mark sandbox as released with a short TTL
	err := store.MarkSandboxReleased(ctx, sandboxID, ttl)
	require.NoError(t, err)

	// Verify it's marked as released
	isReleased, err := store.IsSandboxReleased(ctx, sandboxID)
	require.NoError(t, err)
	assert.True(t, isReleased)

	// Fast-forward time past the TTL
	s.FastForward(ttl + 10*time.Millisecond)

	// Verify it's no longer marked as released due to TTL expiration
	isReleased, err = store.IsSandboxReleased(ctx, sandboxID)
	require.NoError(t, err)
	assert.False(t, isReleased)
}

func TestRedisStore_ConcurrentCreation(t *testing.T) {
	store, s, ctx := setupRedisTest(t)
	defer s.Close()
	defer store.Close()

	idempotenceKey := "idem-key-1"
	ttl := time.Minute

	// First call should acquire the lock
	acquired, err := store.MarkPendingCreation(ctx, idempotenceKey, ttl)
	require.NoError(t, err)
	assert.True(t, acquired)

	// Simulate a concurrent request - should not acquire the lock
	acquired, err = store.MarkPendingCreation(ctx, idempotenceKey, ttl)
	require.NoError(t, err)
	assert.False(t, acquired)
}

func TestRedisStore_ErrorHandling(t *testing.T) {
	store, s, ctx := setupRedisTest(t)
	defer store.Close()

	idempotenceKey := "idem-key-1"
	sandboxID := "sandbox-123"
	ttl := time.Minute

	// Store a sandbox ID
	err := store.StoreSandboxID(ctx, idempotenceKey, sandboxID, ttl)
	require.NoError(t, err)

	// Shutdown Redis to force an error
	s.Close()

	// Operations should now return errors
	_, err = store.GetSandboxID(ctx, idempotenceKey)
	assert.Error(t, err)

	err = store.StoreSandboxID(ctx, idempotenceKey, sandboxID, ttl)
	assert.Error(t, err)

	_, err = store.MarkPendingCreation(ctx, idempotenceKey, ttl)
	assert.Error(t, err)
}

func TestRedisStore_EmptyValues(t *testing.T) {
	store, s, ctx := setupRedisTest(t)
	defer s.Close()
	defer store.Close()

	// Test with empty idempotence key
	emptyKey := ""
	_, err := store.GetSandboxID(ctx, emptyKey)
	assert.NoError(t, err, "Empty key should not cause an error")

	// Test with empty sandbox ID
	idempotenceKey := "idem-key-1"
	emptySandboxID := ""
	ttl := time.Minute

	err = store.StoreSandboxID(ctx, idempotenceKey, emptySandboxID, ttl)
	require.NoError(t, err)

	id, err := store.GetSandboxID(ctx, idempotenceKey)
	require.NoError(t, err)
	assert.Equal(t, emptySandboxID, id)
}
