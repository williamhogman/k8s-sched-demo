package k8sclient

import (
	"context"
	"fmt"
	"log"
)

// MockK8sClient is a mock implementation of the Kubernetes client for testing
type MockK8sClient struct{}

// NewMockK8sClient creates a new mock K8s client
func NewMockK8sClient() *MockK8sClient {
	return &MockK8sClient{}
}

// ScheduleSandbox simulates scheduling a sandbox on Kubernetes
func (m *MockK8sClient) ScheduleSandbox(ctx context.Context, podName, namespace string, metadata map[string]string) (string, error) {
	// Print the scheduling details for debugging
	log.Printf("[MOCK] Scheduling sandbox with name %s in namespace %s", podName, namespace)
	log.Printf("[MOCK] Metadata: %v", metadata)
	log.Printf("[MOCK] Using default resources - CPU: %s, Memory: %s", DefaultCPU, DefaultMemory)

	// Simulate creating a pod
	log.Printf("[MOCK] Created pod %s in namespace %s", podName, namespace)

	return podName, nil
}

// ReleaseSandbox simulates releasing a sandbox on Kubernetes
func (m *MockK8sClient) ReleaseSandbox(ctx context.Context, sandboxID string) error {
	// Print the release details for debugging
	log.Printf("[MOCK] Releasing sandbox with ID %s", sandboxID)

	// Check if the sandbox ID starts with "sandbox-" prefix, if not, prepend it
	podName := sandboxID
	if len(podName) > 8 && podName[:8] != "sandbox-" {
		podName = fmt.Sprintf("sandbox-%s", sandboxID)
	}

	// Simulate deleting a pod
	log.Printf("[MOCK] Deleted pod %s", podName)

	return nil
}
