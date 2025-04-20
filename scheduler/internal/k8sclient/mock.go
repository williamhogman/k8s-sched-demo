package k8sclient

import (
	"context"
	"fmt"
	"log"
)

// MockK8sClient is a mock implementation of the Kubernetes client for testing
type MockK8sClient struct {
	namespace string
}

// NewMockK8sClient creates a new mock K8s client
func NewMockK8sClient() *MockK8sClient {
	return &MockK8sClient{
		namespace: DefaultNamespace,
	}
}

// SetNamespace changes the default namespace
func (m *MockK8sClient) SetNamespace(namespace string) {
	if namespace != "" {
		m.namespace = namespace
	}
}

// ScheduleSandbox simulates scheduling a sandbox on Kubernetes
func (m *MockK8sClient) ScheduleSandbox(ctx context.Context, podName string, metadata map[string]string) (string, error) {
	// Use the client's namespace
	namespace := m.namespace

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
	// Check if the sandbox ID starts with "sandbox-" prefix, if not, prepend it
	podName := sandboxID
	if len(podName) > 8 && podName[:8] != "sandbox-" {
		podName = fmt.Sprintf("sandbox-%s", sandboxID)
	}

	// Print the release details for debugging
	log.Printf("[MOCK] Releasing sandbox with ID %s in namespace %s", podName, m.namespace)

	// Simulate deleting a pod
	log.Printf("[MOCK] Deleted pod %s from namespace %s", podName, m.namespace)

	return nil
}
