package persistence

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrNotFound is returned when a key is not found in the store
var ErrNotFound = errors.New("key not found")

// memoryStore provides an in-memory implementation of Store
type memoryStore struct {
	mu                sync.RWMutex
	store             map[string]string    // idempotence key -> sandbox ID
	pendingKeys       map[string]bool      // pending idempotence keys
	releasedSandboxes map[string]bool      // recently released sandbox IDs
	expirations       map[string]time.Time // sandbox ID -> expiration time
}

// newMemoryStore creates a new in-memory idempotence store
func newMemoryStore() *memoryStore {
	return &memoryStore{
		store:             make(map[string]string),
		pendingKeys:       make(map[string]bool),
		releasedSandboxes: make(map[string]bool),
		expirations:       make(map[string]time.Time),
	}
}

// GetSandboxID returns the sandbox ID for a given idempotence key
func (m *memoryStore) GetSandboxID(ctx context.Context, idempotenceKey string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sandboxID, exists := m.store[idempotenceKey]
	if !exists {
		return "", ErrNotFound
	}
	return sandboxID, nil
}

// StoreSandboxID stores the sandbox ID for a given idempotence key
func (m *memoryStore) StoreSandboxID(ctx context.Context, idempotenceKey, sandboxID string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// For simplicity, we don't implement TTL in memory store
	m.store[idempotenceKey] = sandboxID
	return nil
}

// MarkPendingCreation marks that a sandbox is being created for this idempotence key
func (m *memoryStore) MarkPendingCreation(ctx context.Context, idempotenceKey string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// For simplicity, we don't implement TTL in memory store
	if _, exists := m.pendingKeys[idempotenceKey]; exists {
		return false, nil
	}
	m.pendingKeys[idempotenceKey] = true
	return true, nil
}

// CompletePendingCreation updates a pending sandbox creation with its final sandboxID
func (m *memoryStore) CompletePendingCreation(ctx context.Context, idempotenceKey, sandboxID string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.pendingKeys, idempotenceKey)
	m.store[idempotenceKey] = sandboxID
	return nil
}

// ReleaseIdempotenceKey removes the idempotence key
func (m *memoryStore) ReleaseIdempotenceKey(ctx context.Context, idempotenceKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.store, idempotenceKey)
	delete(m.pendingKeys, idempotenceKey)
	return nil
}

// MarkSandboxReleased marks a sandbox as recently released
func (m *memoryStore) MarkSandboxReleased(ctx context.Context, sandboxID string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.releasedSandboxes[sandboxID] = true
	return nil
}

// IsSandboxReleased checks if a sandbox was recently released
func (m *memoryStore) IsSandboxReleased(ctx context.Context, sandboxID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.releasedSandboxes[sandboxID]
	return exists, nil
}

// UnmarkSandboxReleased removes the released mark for a sandbox
func (m *memoryStore) UnmarkSandboxReleased(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.releasedSandboxes, sandboxID)
	return nil
}

// SetSandboxExpiration sets the expiration time for a sandbox
func (m *memoryStore) SetSandboxExpiration(ctx context.Context, sandboxID string, expiration time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.expirations[sandboxID] = expiration
	return nil
}

// ExtendSandboxExpiration extends the expiration time for a sandbox
func (m *memoryStore) ExtendSandboxExpiration(ctx context.Context, sandboxID string, extension time.Duration) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get current expiration time or use current time if not set
	currentExpiration, exists := m.expirations[sandboxID]
	if !exists {
		currentExpiration = time.Now()
	}

	// Calculate new expiration time
	newExpiration := currentExpiration.Add(extension)
	m.expirations[sandboxID] = newExpiration

	return newExpiration, nil
}

// GetExpiredSandboxes returns a list of sandbox IDs that have expired
func (m *memoryStore) GetExpiredSandboxes(ctx context.Context, now time.Time, limit int) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []string
	for sandboxID, expiration := range m.expirations {
		if expiration.Before(now) || expiration.Equal(now) {
			result = append(result, sandboxID)
			if len(result) >= limit {
				break
			}
		}
	}

	return result, nil
}

// RemoveSandboxExpiration removes the expiration tracking for a sandbox
func (m *memoryStore) RemoveSandboxExpiration(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.expirations, sandboxID)
	return nil
}

// Close is a no-op for in-memory store
func (m *memoryStore) Close() error {
	return nil
}
