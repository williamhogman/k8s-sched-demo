package persistence

import "errors"

// Common errors that can be returned by persistence store implementations
var (
	// ErrNotFound is returned when an item is not found in the store
	ErrNotFound = errors.New("not found")
)
