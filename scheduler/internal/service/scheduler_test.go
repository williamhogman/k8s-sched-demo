package service

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeneratePodName(t *testing.T) {
	// Test basic properties of generated pod names
	t.Run("Basic pod name format", func(t *testing.T) {
		podName := generatePodName()

		// Should start with "sandbox-"
		assert.True(t, strings.HasPrefix(podName, "sandbox-"),
			"Pod name should start with 'sandbox-'")

		// Should have a UUID part after the prefix that's exactly 8 chars
		parts := strings.Split(podName, "-")
		assert.Equal(t, 2, len(parts), "Pod name should have exactly one hyphen")
		assert.Equal(t, "sandbox", parts[0])
		assert.Equal(t, 8, len(parts[1]), "UUID part should be exactly 8 characters")

		// Format should be valid hex
		_, err := strconv.ParseUint(parts[1], 16, 64)
		assert.NoError(t, err, "UUID part should be valid hexadecimal")

		// Total length should be reasonable for K8s (DNS-1123 label format)
		assert.LessOrEqual(t, len(podName), 63,
			"Pod name should not exceed K8s limit of 63 characters")
	})

	t.Run("Uniqueness of pod names", func(t *testing.T) {
		// Generate multiple pod names and ensure they're unique
		podNames := make(map[string]bool)
		iterations := 100

		for i := 0; i < iterations; i++ {
			name := generatePodName()
			assert.False(t, podNames[name], "Generated pod names should be unique")
			podNames[name] = true
		}

		// We should have the same number of unique names as iterations
		assert.Equal(t, iterations, len(podNames),
			"Should generate unique pod names for each call")
	})

	t.Run("Always different", func(t *testing.T) {
		// Different calls should result in different pod names
		name1 := generatePodName()
		name2 := generatePodName()

		// Both should follow the same format
		assert.True(t, strings.HasPrefix(name1, "sandbox-"))
		assert.True(t, strings.HasPrefix(name2, "sandbox-"))

		// They should be different names
		assert.NotEqual(t, name1, name2)
	})
}

func TestSandboxIDValidation(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		isValid bool
	}{
		{
			name:    "Valid sandbox ID",
			id:      "sandbox-12345678",
			isValid: true,
		},
		{
			name:    "Valid sandbox ID with hex chars",
			id:      "sandbox-abcdef01",
			isValid: true,
		},
		{
			name:    "Invalid - too short",
			id:      "sandbox-1234567",
			isValid: false,
		},
		{
			name:    "Invalid - too long",
			id:      "sandbox-123456789",
			isValid: false,
		},
		{
			name:    "Invalid - wrong prefix",
			id:      "pod-12345678",
			isValid: false,
		},
		{
			name:    "Invalid - no prefix",
			id:      "12345678",
			isValid: false,
		},
		{
			name:    "Invalid - empty",
			id:      "",
			isValid: false,
		},
		{
			name:    "Invalid - non-hex characters",
			id:      "sandbox-abcdxyz1",
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidSandboxID(tt.id)
			assert.Equal(t, tt.isValid, result)
		})
	}
}

func TestEnsureValidSandboxID(t *testing.T) {
	t.Run("Already valid ID is unchanged", func(t *testing.T) {
		validID := "sandbox-12345678"
		result := ensureValidSandboxID(validID)
		assert.Equal(t, validID, result)
	})

	t.Run("Fixes invalid ID", func(t *testing.T) {
		invalidIDs := []string{
			"sandbox-123",            // Too short
			"sandbox-12345678901234", // Too long
			"sandbox-xyz!@#",         // Invalid chars
		}

		for _, invalidID := range invalidIDs {
			result := ensureValidSandboxID(invalidID)
			assert.True(t, isValidSandboxID(result), "Fixed ID should be valid")
			assert.True(t, strings.HasPrefix(result, "sandbox-"), "Should keep sandbox prefix")
			assert.Equal(t, 16, len(result), "Should have correct length")
		}
	})

	t.Run("Generates new ID for completely wrong format", func(t *testing.T) {
		wrongIDs := []string{
			"pod-12345678",
			"container123",
			"",
		}

		for _, wrongID := range wrongIDs {
			result := ensureValidSandboxID(wrongID)
			assert.True(t, isValidSandboxID(result), "Generated ID should be valid")
			assert.True(t, strings.HasPrefix(result, "sandbox-"), "Should have sandbox prefix")
			assert.Equal(t, 16, len(result), "Should have correct length")
		}
	})
}
