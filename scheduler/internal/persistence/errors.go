package persistence

import "errors"

// Common errors for persistence operations
var (
	// ErrNotFound is returned when a key is not found in the store
	ErrNotFound = errors.New("key not found in store")

	// ErrConcurrentCreationFailed is returned when a concurrent creation process was abandoned
	ErrConcurrentCreationFailed = errors.New("concurrent creation failed or was abandoned")
)
