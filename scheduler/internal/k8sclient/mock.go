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
func (m *MockK8sClient) ScheduleSandbox(ctx context.Context, sandboxID, namespace string, config map[string]string) (string, error) {
	// Print the scheduling details for debugging
	log.Printf("[MOCK] Scheduling sandbox %s in namespace %s", sandboxID, namespace)
	log.Printf("[MOCK] Configuration: %v", config)
	log.Printf("[MOCK] Using default resources - CPU: %s, Memory: %s", DefaultCPU, DefaultMemory)

	// Simulate creating a pod
	podName := fmt.Sprintf("sandbox-%s", sandboxID)

	// Simulate a delay for pod creation
	log.Printf("[MOCK] Created pod %s in namespace %s", podName, namespace)

	return podName, nil
}
