package k8sclient

import (
	"context"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
	"go.uber.org/zap"
)

// Default values for mock client
const (
	DefaultNamespace = "sandbox"
)

// MockK8sClient is a mock implementation of the Kubernetes client for testing
type MockK8sClient struct {
	namespace string
	logger    *zap.Logger
	eventChan chan types.PodEvent
}

// NewMockK8sClient creates a new mock K8s client
func NewMockK8sClient(logger *zap.Logger) *MockK8sClient {
	return &MockK8sClient{
		namespace: DefaultNamespace,
		logger:    logger.Named("mock-k8sclient"),
		eventChan: make(chan types.PodEvent, 100),
	}
}

// GetEventChannel returns the channel for pod events
func (m *MockK8sClient) GetEventChannel() <-chan types.PodEvent {
	return m.eventChan
}

// StartWatchers is a no-op in the mock implementation
func (m *MockK8sClient) StartWatchers() {
	m.logger.Info("Mock would start watchers", zap.String("namespace", m.namespace))
}

// StopWatchers is a no-op in the mock implementation
func (m *MockK8sClient) StopWatchers() {
	m.logger.Info("Mock would stop watchers")
	// Close the event channel
	close(m.eventChan)
}

// ScheduleSandbox simulates scheduling a sandbox on Kubernetes
func (m *MockK8sClient) ScheduleSandbox(ctx context.Context, podName string, metadata map[string]string) (string, error) {
	m.logger.Info("Mock pod scheduled", zap.String("pod", podName), zap.Any("metadata", metadata))
	return podName, nil
}

// ReleaseSandbox simulates releasing a sandbox on Kubernetes
func (m *MockK8sClient) ReleaseSandbox(ctx context.Context, sandboxID string) error {
	m.logger.Info("Mock pod released", zap.String("pod", sandboxID))
	return nil
}

// SendEvent sends an event to the pod event channel
// This is a helper method for testing
func (m *MockK8sClient) SendEvent(event types.PodEvent) {
	select {
	case m.eventChan <- event:
		m.logger.Info("Sent mock pod event",
			zap.String("pod", event.PodName),
			zap.String("eventType", string(event.EventType)))
	default:
		m.logger.Warn("Mock event channel full, dropping event",
			zap.String("pod", event.PodName),
			zap.String("eventType", string(event.EventType)))
	}
}
