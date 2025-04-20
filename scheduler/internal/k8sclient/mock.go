package k8sclient

import (
	"context"
	"log"
)

// Default values for mock client
const (
	DefaultNamespace = "sandbox"
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
	log.Printf("[MOCK] Pod %s scheduled", podName)
	return podName, nil
}

// ReleaseSandbox simulates releasing a sandbox on Kubernetes
func (m *MockK8sClient) ReleaseSandbox(ctx context.Context, sandboxID string) error {
	log.Printf("[MOCK] Pod %s released", sandboxID)
	return nil
}
