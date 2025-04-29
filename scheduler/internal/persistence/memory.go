package persistence

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrNotFound is returned when a key is not found in the store
var ErrNotFound = errors.New("key not found")

// memoryStore implements the Store interface using in-memory maps
type memoryStore struct {
	mu sync.RWMutex
	// idempotenceKey -> sandboxID mapping
	idempotenceMap map[string]string
	// sandboxID -> idempotenceKey mapping (reverse mapping)
	sandboxIdempotenceMap map[string]string
	// sandboxID -> projectID mapping
	sandboxProjectMap map[string]string
	// projectID -> sandboxID mapping
	projectSandboxMap map[string]string
	// pending idempotence keys
	pendingKeys map[string]struct{}
	// expiration times for sandboxes
	expirationTimes map[string]time.Time
}

// NewMemoryStore creates a new memory store
func NewMemoryStore() Store {
	return &memoryStore{
		idempotenceMap:        make(map[string]string),
		sandboxIdempotenceMap: make(map[string]string),
		sandboxProjectMap:     make(map[string]string),
		projectSandboxMap:     make(map[string]string),
		pendingKeys:           make(map[string]struct{}),
		expirationTimes:       make(map[string]time.Time),
	}
}

// GetSandboxID returns the sandbox ID for a given idempotence key
func (m *memoryStore) GetSandboxID(ctx context.Context, idempotenceKey string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sandboxID, exists := m.idempotenceMap[idempotenceKey]
	if !exists {
		return "", ErrNotFound
	}
	return sandboxID, nil
}

// StoreSandboxID stores the sandbox ID for a given idempotence key
func (m *memoryStore) StoreSandboxID(ctx context.Context, idempotenceKey, sandboxID string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store the idempotence key -> sandbox mapping
	m.idempotenceMap[idempotenceKey] = sandboxID
	// Store the sandbox -> idempotence key mapping (reverse mapping)
	m.sandboxIdempotenceMap[sandboxID] = idempotenceKey
	// Set expiration time
	m.expirationTimes[sandboxID] = time.Now().Add(ttl)

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
	m.pendingKeys[idempotenceKey] = struct{}{}
	return true, nil
}

// CompletePendingCreation updates a pending sandbox creation with its final sandboxID
func (m *memoryStore) CompletePendingCreation(ctx context.Context, idempotenceKey, sandboxID string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store the sandbox ID with the idempotence key
	m.idempotenceMap[idempotenceKey] = sandboxID
	// Store the sandbox -> idempotence key mapping (reverse mapping)
	m.sandboxIdempotenceMap[sandboxID] = idempotenceKey
	// Set expiration time
	m.expirationTimes[sandboxID] = time.Now().Add(ttl)
	// Remove the pending key
	delete(m.pendingKeys, idempotenceKey)

	return nil
}

// ReleaseIdempotenceKey removes the idempotence key
func (m *memoryStore) ReleaseIdempotenceKey(ctx context.Context, idempotenceKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get the sandbox ID
	sandboxID, exists := m.idempotenceMap[idempotenceKey]
	if !exists {
		return nil // No mapping exists, nothing to remove
	}

	// Remove the idempotence key -> sandbox mapping
	delete(m.idempotenceMap, idempotenceKey)
	// Remove the sandbox -> idempotence key mapping
	delete(m.sandboxIdempotenceMap, sandboxID)
	// Remove the pending key
	delete(m.pendingKeys, idempotenceKey)
	// Remove the expiration time
	delete(m.expirationTimes, sandboxID)

	return nil
}

// MarkSandboxReleased marks a sandbox as recently released
func (m *memoryStore) MarkSandboxReleased(ctx context.Context, sandboxID string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.expirationTimes[sandboxID] = time.Now().Add(ttl)
	return nil
}

// IsSandboxReleased checks if a sandbox was recently released
func (m *memoryStore) IsSandboxReleased(ctx context.Context, sandboxID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.expirationTimes[sandboxID]
	return exists, nil
}

// UnmarkSandboxReleased removes the released mark for a sandbox
func (m *memoryStore) UnmarkSandboxReleased(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.expirationTimes, sandboxID)
	return nil
}

// SetSandboxExpiration sets the expiration time for a sandbox
func (m *memoryStore) SetSandboxExpiration(ctx context.Context, sandboxID string, expiration time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.expirationTimes[sandboxID] = expiration
	return nil
}

// ExtendSandboxExpiration extends the expiration time for a sandbox
func (m *memoryStore) ExtendSandboxExpiration(ctx context.Context, sandboxID string, extension time.Duration) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get current expiration time or use current time if not set
	currentExpiration, exists := m.expirationTimes[sandboxID]
	if !exists {
		currentExpiration = time.Now()
	}

	// Calculate new expiration time
	newExpiration := currentExpiration.Add(extension)
	m.expirationTimes[sandboxID] = newExpiration

	return newExpiration, nil
}

// GetExpiredSandboxes returns a list of sandbox IDs that have expired
func (m *memoryStore) GetExpiredSandboxes(ctx context.Context, now time.Time, limit int) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []string
	for sandboxID, expiration := range m.expirationTimes {
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

	delete(m.expirationTimes, sandboxID)
	return nil
}

// Close is a no-op for in-memory store
func (m *memoryStore) Close() error {
	return nil
}

// setIfNotExists stores a value for a key only if the key doesn't exist
func (m *memoryStore) setIfNotExists(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if key exists in the store
	if _, exists := m.idempotenceMap[key]; exists {
		return false, nil
	}

	// Set the key if it doesn't exist
	m.idempotenceMap[key] = value
	return true, nil
}

// GetProjectSandbox returns the sandbox ID for a given project
func (m *memoryStore) GetProjectSandbox(ctx context.Context, projectID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sandboxID, exists := m.projectSandboxMap[projectID]
	if !exists {
		return "", ErrNotFound
	}
	return sandboxID, nil
}

// SetProjectSandbox stores the sandbox ID for a given project
func (m *memoryStore) SetProjectSandbox(ctx context.Context, projectID, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set both mappings
	m.projectSandboxMap[projectID] = sandboxID
	m.sandboxProjectMap[sandboxID] = projectID
	return nil
}

// RemoveProjectSandbox removes the project-sandbox mapping
func (m *memoryStore) RemoveProjectSandbox(ctx context.Context, projectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get the sandbox ID first
	sandboxID, exists := m.projectSandboxMap[projectID]
	if !exists {
		return nil // No mapping exists, nothing to remove
	}

	// Remove both mappings
	delete(m.projectSandboxMap, projectID)
	delete(m.sandboxProjectMap, sandboxID)
	return nil
}

// GetSandboxExpiration returns the expiration time for a sandbox
func (m *memoryStore) GetSandboxExpiration(ctx context.Context, sandboxID string) (time.Time, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	expiration, exists := m.expirationTimes[sandboxID]
	if !exists {
		return time.Time{}, fmt.Errorf("sandbox %s not found", sandboxID)
	}

	return expiration, nil
}

// IsSandboxValid checks if a sandbox is still valid (not expired)
func (m *memoryStore) IsSandboxValid(ctx context.Context, sandboxID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	expiration, exists := m.expirationTimes[sandboxID]
	if !exists {
		return false, fmt.Errorf("sandbox %s not found", sandboxID)
	}

	return time.Now().Before(expiration), nil
}

// FindProjectForSandbox finds the project ID associated with a sandbox
func (m *memoryStore) FindProjectForSandbox(ctx context.Context, sandboxID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	projectID, exists := m.sandboxProjectMap[sandboxID]
	if !exists {
		return "", nil // Not found, return empty string
	}
	return projectID, nil
}

// FindIdempotenceKeyForSandbox finds the idempotence key associated with a sandbox
func (m *memoryStore) FindIdempotenceKeyForSandbox(ctx context.Context, sandboxID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	idempotenceKey, exists := m.sandboxIdempotenceMap[sandboxID]
	if !exists {
		return "", nil // Not found, return empty string
	}
	return idempotenceKey, nil
}

// RemoveSandboxMapping removes all mappings for a sandbox (idempotence key and project)
func (m *memoryStore) RemoveSandboxMapping(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get the idempotence key
	idempotenceKey, exists := m.sandboxIdempotenceMap[sandboxID]
	if exists {
		// Remove the idempotence key -> sandbox mapping
		delete(m.idempotenceMap, idempotenceKey)
		// Remove the sandbox -> idempotence key mapping
		delete(m.sandboxIdempotenceMap, sandboxID)
	}

	// Get the project ID
	projectID, exists := m.sandboxProjectMap[sandboxID]
	if exists {
		// Remove the sandbox -> project mapping
		delete(m.sandboxProjectMap, sandboxID)
		// Remove the project -> sandbox mapping
		delete(m.projectSandboxMap, projectID)
	}

	// Remove the expiration time
	delete(m.expirationTimes, sandboxID)

	return nil
}
