package persistence

import "errors"

// Common errors for persistence operations
var (
	// ErrNotFound is returned when no entry is found for a key
	ErrNotFound = errors.New("not found")

	// ErrConcurrentCreationFailed is returned when a concurrent sandbox creation fails
	ErrConcurrentCreationFailed = errors.New("concurrent sandbox creation failed")

	// ErrPoolEmpty is returned when the sandbox pool is empty
	ErrPoolEmpty = errors.New("pool is empty")
)
