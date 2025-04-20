package idempotence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_GetSetSandboxID(t *testing.T) {
	// ... existing code ...
}

func TestMemoryStore_MarkPendingCreation(t *testing.T) {
	// ... existing code ...
}

func TestMemoryStore_CompletePendingCreation(t *testing.T) {
	// ... existing code ...
}

func TestMemoryStore_ReleaseIdempotenceKey(t *testing.T) {
	// ... existing code ...
}

func TestMemoryStore_ReleasedSandbox(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	sandboxID := "sandbox-123"
	ttl := time.Minute

	// Test marking a sandbox as released
	err := store.MarkSandboxReleased(ctx, sandboxID, ttl)
	require.NoError(t, err, "Failed to mark sandbox as released")

	// Test checking if a sandbox is released
	isReleased, err := store.IsSandboxReleased(ctx, sandboxID)
	require.NoError(t, err, "Failed to check if sandbox is released")
	assert.True(t, isReleased, "Sandbox should be marked as released")

	// Test checking a non-existent sandbox
	isReleased, err = store.IsSandboxReleased(ctx, "non-existent")
	require.NoError(t, err)
	assert.False(t, isReleased, "Non-existent sandbox should not be marked as released")

	// Test unmarking a sandbox
	err = store.UnmarkSandboxReleased(ctx, sandboxID)
	require.NoError(t, err, "Failed to unmark sandbox")

	// Verify it's no longer marked as released
	isReleased, err = store.IsSandboxReleased(ctx, sandboxID)
	require.NoError(t, err)
	assert.False(t, isReleased, "Sandbox should no longer be marked as released")

	// Test unmarking a non-existent sandbox (should not error)
	err = store.UnmarkSandboxReleased(ctx, "non-existent")
	assert.NoError(t, err, "Unmarking non-existent sandbox should not error")
}
