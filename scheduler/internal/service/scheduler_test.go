package service

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGeneratePodName tests the pod name generation logic
func TestGeneratePodName(t *testing.T) {
	// Test that generated pod names follow the expected format
	for i := 0; i < 10; i++ {
		podName := generatePodName()

		// Check that it starts with the expected prefix
		assert.True(t, strings.HasPrefix(podName, "sandbox-"), "Pod name should start with 'sandbox-'")

		// Check that it has the correct length (sandbox- + 8 hex chars)
		expectedLength := len("sandbox-") + 8
		assert.Equal(t, expectedLength, len(podName), "Pod name should have the expected length")

		// Extract the ID part and verify it's a valid hex string
		idPart := podName[len("sandbox-"):]
		assert.Equal(t, 8, len(idPart), "ID part should be 8 characters long")

		// Try to parse as hex to verify it's valid
		_, err := strconv.ParseUint(idPart, 16, 32)
		assert.NoError(t, err, "ID part should be a valid hex string")
	}
}
