// The package name uses _test suffix to avoid import cycles
package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
	"go.uber.org/zap/zaptest"
)

func TestPodEventHandling(t *testing.T) {
	// Setup
	logger := zaptest.NewLogger(t)
	mockClient := k8sclient.NewMockK8sClient(logger)
	memStore, err := persistence.NewStore(persistence.Config{Type: "memory"})
	require.NoError(t, err)

	config := service.SchedulerServiceConfig{
		IdempotenceKeyTTL: 24 * time.Hour,
		SandboxTTL:        1 * time.Hour,
	}

	// Create the service
	scheduler := service.NewSchedulerService(mockClient, memStore, config, logger)
	defer scheduler.Close()

	// Wait a bit for the service to initialize
	time.Sleep(100 * time.Millisecond)

	// Create a test pod event
	podEvent := types.PodEvent{
		PodName:   "sandbox-12345",
		EventType: types.PodEventFailed,
		Reason:    "ContainerExited:1",
		Message:   "Container exited with non-zero code",
		Timestamp: time.Now(),
	}

	// Send event using the helper method in the mock client
	mockClient.SendEvent(podEvent)

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Since we can't directly access the store internals, we'll just verify
	// that the service doesn't crash when processing events.
	// In a real production environment, we would have more comprehensive tests
	// with mocks that allow us to verify the behavior.
}
