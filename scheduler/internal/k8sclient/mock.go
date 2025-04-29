package k8sclient

import (
	"context"
	"fmt"
	"time"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
	"go.uber.org/zap"
)

// Default values for mock client
const (
	DefaultNamespace = "sandbox"
)

// MockK8sClient provides a simple logging implementation of K8sClientInterface
type MockK8sClient struct {
	logger    *zap.Logger
	eventChan chan types.PodEvent
}

// NewMockK8sClient creates a new mock K8s client
func NewMockK8sClient(logger *zap.Logger) *MockK8sClient {
	return &MockK8sClient{
		logger:    logger.Named("mock-k8sclient"),
		eventChan: make(chan types.PodEvent, 100),
	}
}

// ScheduleSandbox creates a mock sandbox pod
func (m *MockK8sClient) ScheduleSandbox(ctx context.Context, podName string, metadata map[string]string) (string, error) {
	m.logger.Info("Mock ScheduleSandbox called",
		zap.String("podName", podName),
		zap.Any("metadata", metadata))
	return podName, nil
}

// ReleaseSandbox deletes a mock sandbox pod
func (m *MockK8sClient) ReleaseSandbox(ctx context.Context, sandboxID string) error {
	m.logger.Info("Mock ReleaseSandbox called",
		zap.String("sandboxID", sandboxID))
	return nil
}

// GetEventChannel returns the mock event channel
func (m *MockK8sClient) GetEventChannel() <-chan types.PodEvent {
	return m.eventChan
}

// StartWatchers starts mock pod watchers
func (m *MockK8sClient) StartWatchers() {
	m.logger.Info("Mock StartWatchers called")
}

// StopWatchers stops mock pod watchers
func (m *MockK8sClient) StopWatchers() {
	m.logger.Info("Mock StopWatchers called")
	close(m.eventChan)
}

// CreateOrUpdateProjectService creates or updates a mock project service
func (m *MockK8sClient) CreateOrUpdateProjectService(ctx context.Context, projectID string, sandboxID string) error {
	m.logger.Info("Mock CreateOrUpdateProjectService called",
		zap.String("projectID", projectID),
		zap.String("sandboxID", sandboxID))
	return nil
}

// DeleteProjectService deletes a mock project service
func (m *MockK8sClient) DeleteProjectService(ctx context.Context, projectID string) error {
	m.logger.Info("Mock DeleteProjectService called",
		zap.String("projectID", projectID))
	return nil
}

// GetProjectServiceHostname returns a mock project service hostname
func (m *MockK8sClient) GetProjectServiceHostname(projectID string) string {
	hostname := fmt.Sprintf("project-%s.%s.svc.cluster.local", projectID, DefaultNamespace)
	m.logger.Info("Mock GetProjectServiceHostname called",
		zap.String("projectID", projectID),
		zap.String("hostname", hostname))
	return hostname
}

// SendMockEvent sends a mock pod event to the event channel
func (m *MockK8sClient) SendMockEvent(eventType types.PodEventType, podName string) {
	event := types.PodEvent{
		PodName:   podName,
		EventType: eventType,
	}
	select {
	case m.eventChan <- event:
		m.logger.Info("Mock event sent",
			zap.String("podName", podName),
			zap.String("eventType", string(eventType)))
	default:
		m.logger.Warn("Mock event channel full, dropping event")
	}
}

// IsSandboxReady checks if a mock sandbox is ready
func (m *MockK8sClient) IsSandboxReady(ctx context.Context, sandboxID string) (bool, error) {
	m.logger.Info("Mock IsSandboxReady called",
		zap.String("sandboxID", sandboxID))

	// For mock purposes, we'll consider all sandboxes ready
	// In a real implementation, this would check the pod status
	return true, nil
}

// IsSandboxGone checks if a mock sandbox is gone
func (m *MockK8sClient) IsSandboxGone(ctx context.Context, sandboxID string) (bool, error) {
	m.logger.Info("Mock IsSandboxGone called",
		zap.String("sandboxID", sandboxID))

	// For mock purposes, we'll consider all sandboxes as existing
	// In a real implementation, this would check if the pod exists
	return false, nil
}

// WaitForSandboxReady waits for a mock sandbox to be ready
func (m *MockK8sClient) WaitForSandboxReady(ctx context.Context, sandboxID string, timeout time.Duration) (bool, error) {
	m.logger.Info("Mock WaitForSandboxReady called",
		zap.String("sandboxID", sandboxID),
		zap.Duration("timeout", timeout))

	// For mock purposes, we'll simulate a short delay and then return success
	// In a real implementation, this would poll until the pod is ready or timeout
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(100 * time.Millisecond): // Simulate a short delay
		return true, nil
	}
}
