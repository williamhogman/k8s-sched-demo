package types

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Constants for ID prefixes
const (
	SandboxPrefix = "sandbox-"
	ProjectPrefix = "project-"
)

// Common errors for ID validation
var (
	ErrEmptyID = errors.New("ID cannot be empty")
)

// SandboxID is a typed wrapper for sandbox identifiers
type SandboxID string

// ProjectID is a typed wrapper for project identifiers
type ProjectID string

// NewSandboxID creates a new SandboxID from a string, removing the prefix if present
// Returns an error if the resulting ID would be empty
func NewSandboxID(id string) (SandboxID, error) {
	// Remove the prefix if present
	cleanID := strings.TrimPrefix(id, SandboxPrefix)

	// Validate that the ID is not empty
	if cleanID == "" {
		return "", ErrEmptyID
	}

	return SandboxID(cleanID), nil
}

// IsValid returns true if the sandbox ID is valid (not empty)
func (s SandboxID) IsValid() bool {
	return s != ""
}

// String returns the raw sandbox ID without prefix
func (s SandboxID) String() string {
	return string(s)
}

// WithPrefix returns the sandbox ID with the 'sandbox-' prefix
func (s SandboxID) WithPrefix() string {
	if !s.IsValid() {
		return ""
	}
	return SandboxPrefix + string(s)
}

func (s SandboxID) ZapField() zap.Field {
	if !s.IsValid() {
		return zap.Skip()
	}
	return zap.String("sandboxID", string(s))
}

// GenerateSandboxID creates a new random sandbox ID
func GenerateSandboxID() SandboxID {
	// Generate a random number
	randomNum := time.Now().UnixNano() + int64(uuid.New()[0])

	// Encode the random number in base36
	id := encodeBase36(randomNum)

	// Truncate to keep the name reasonably short
	if len(id) > 20 {
		id = id[:20]
	}

	return SandboxID(id)
}

// NewProjectID creates a new ProjectID from a string, removing the prefix if present
// Returns an error if the resulting ID would be empty
func NewProjectID(id string) (ProjectID, error) {
	// Remove the prefix if present
	cleanID := strings.TrimPrefix(id, ProjectPrefix)

	// Validate that the ID is not empty
	if cleanID == "" {
		return "", ErrEmptyID
	}

	return ProjectID(cleanID), nil
}

func NewProjectIDWithoutPrefix(id string) (ProjectID, error) {
	return NewProjectID(ProjectPrefix + id)
}

func (p ProjectID) IsValid() bool {
	return p != ""
}

// String returns the raw project ID without prefix
func (p ProjectID) String() string {
	return string(p)
}

func (p ProjectID) ZapField() zap.Field {
	if !p.IsValid() {
		return zap.Skip()
	}
	return zap.String("projectID", string(p))
}

// WithPrefix returns the project ID with the 'project-' prefix
func (p ProjectID) WithPrefix() string {
	if !p.IsValid() {
		return ""
	}
	return ProjectPrefix + string(p)
}

// encodeBase36 encodes an integer as a base36 string (0-9a-z)
func encodeBase36(n int64) string {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return "0"
	}

	var result strings.Builder
	for n > 0 {
		result.WriteByte(charset[n%36])
		n /= 36
	}

	// Reverse the string
	runes := []rune(result.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
}
