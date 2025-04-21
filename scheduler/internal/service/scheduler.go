package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/events"
	persistence "github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
	"go.uber.org/zap"
)

// K8sClientInterface defines the interface for Kubernetes client
type K8sClientInterface interface {
	ScheduleSandbox(ctx context.Context, podName string, metadata map[string]string) (string, error)
	// We don't define the template-based methods here to avoid import cycles
	// Template-based pod creation should be used directly with the k8sclient package
	ReleaseSandbox(ctx context.Context, sandboxID string) error
	StartWatchers()
	StopWatchers()
	// GetEventChannel returns the channel for pod events
	GetEventChannel() <-chan types.PodEvent
}

// SchedulerService implements the scheduling service logic
type SchedulerService struct {
	k8sClient         K8sClientInterface
	store             persistence.Store
	idempotenceKeyTTL time.Duration
	sandboxTTL        time.Duration
	logger            *zap.Logger
	// Context for event processing
	eventCtx    context.Context
	eventCancel context.CancelFunc
	// Flag to indicate if event processing is running
	eventProcessingRunning bool
	// Event broadcaster
	eventBroadcaster events.BroadcasterInterface
}

// SchedulerServiceConfig contains configuration for the scheduler service
type SchedulerServiceConfig struct {
	IdempotenceKeyTTL time.Duration
	SandboxTTL        time.Duration
}

// NewSchedulerService creates a new scheduler service
func NewSchedulerService(
	k8sClient K8sClientInterface,
	store persistence.Store,
	config SchedulerServiceConfig,
	logger *zap.Logger,
	eventBroadcaster events.BroadcasterInterface,
) *SchedulerService {
	ctx, cancel := context.WithCancel(context.Background())
	svc := &SchedulerService{
		k8sClient:         k8sClient,
		store:             store,
		idempotenceKeyTTL: config.IdempotenceKeyTTL,
		sandboxTTL:        config.SandboxTTL,
		logger:            logger.Named("scheduler-service"),
		eventCtx:          ctx,
		eventCancel:       cancel,
		eventBroadcaster:  eventBroadcaster,
	}

	// Start processing pod events
	svc.startEventProcessing()

	return svc
}

// startEventProcessing begins listening for pod events from the K8s client
func (s *SchedulerService) startEventProcessing() {
	if s.eventProcessingRunning {
		return // Already running
	}

	s.logger.Info("Starting pod event processing")
	s.eventProcessingRunning = true

	go func() {
		eventChan := s.k8sClient.GetEventChannel()

		for {
			select {
			case event, ok := <-eventChan:
				if !ok {
					// Channel closed, exit
					s.logger.Info("Event channel closed, stopping event processing")
					s.eventProcessingRunning = false
					return
				}

				// Process the event
				s.handlePodEvent(event)

			case <-s.eventCtx.Done():
				// Context canceled, exit
				s.logger.Info("Event processing context canceled, stopping event processing")
				s.eventProcessingRunning = false
				return
			}
		}
	}()
}

// handlePodEvent processes pod events from the k8s client
func (s *SchedulerService) handlePodEvent(event types.PodEvent) {
	sandboxID := event.PodName

	// Define bad events that should trigger sandbox release
	isBadEvent := event.EventType == types.PodEventFailed ||
		event.EventType == types.PodEventUnschedulable ||
		event.EventType == types.PodEventTerminated

	if !isBadEvent {
		return
	}

	// Check if we've already handled this event
	shouldHandle, _ := s.shouldHandlePodEvent(s.eventCtx, sandboxID)
	if !shouldHandle {
		s.logger.Info("Already processed event for this sandbox",
			zap.String("sandboxID", sandboxID))
		return
	}

	// Log action about to be taken
	s.logger.Info("Bad event detected - releasing sandbox",
		zap.String("sandboxID", sandboxID),
		zap.String("eventType", string(event.EventType)),
		zap.String("reason", event.Reason))

	// Release the sandbox
	_, err := s.ReleaseSandbox(s.eventCtx, sandboxID)
	if err != nil {
		// Log error but continue to broadcasting
		s.logger.Error("Failed to release sandbox",
			zap.String("sandboxID", sandboxID),
			zap.Error(err))
	}
}

// shouldHandlePodEvent checks if we should handle this pod event or if it's already been processed
// Returns true if we should handle the event, false if we've already handled it
func (s *SchedulerService) shouldHandlePodEvent(ctx context.Context, sandboxID string) (bool, error) {
	// We use a short TTL for the failure tracking since we only need to avoid duplicates
	// during event storms, not long-term tracking
	const failureTrackingTTL = 10 * time.Minute

	// Try to mark this sandbox as having a processed event
	// Use the specialized MarkPodEventProcessed method which encapsulates the pod event tracking logic
	return s.store.MarkPodEventProcessed(ctx, sandboxID, failureTrackingTTL)
}

// ScheduleSandbox handles scheduling a sandbox
func (s *SchedulerService) ScheduleSandbox(ctx context.Context, idempotenceKey string, metadata map[string]string) (string, bool, error) {
	if idempotenceKey == "" {
		return "", false, fmt.Errorf("idempotence key cannot be empty")
	}

	s.logger.Info("Scheduling sandbox", zap.String("idempotenceKey", idempotenceKey))

	// Check if we already have a sandbox for this idempotence key
	sandboxID, err := s.store.GetSandboxID(ctx, idempotenceKey)
	if err == nil && sandboxID != "" {
		// We found an existing sandbox for this idempotence key
		s.logger.Info("Found existing sandbox",
			zap.String("idempotenceKey", idempotenceKey),
			zap.String("sandboxID", sandboxID))
		return sandboxID, true, nil // Return the existing sandbox, not newly created
	} else if err != nil && err != persistence.ErrNotFound {
		// Only log actual errors, not key not found
		s.logger.Warn("Error when checking idempotence key",
			zap.String("idempotenceKey", idempotenceKey),
			zap.Error(err))
	}

	// Try to mark this idempotence key as pending creation to handle concurrent requests
	marked, err := s.store.MarkPendingCreation(ctx, idempotenceKey, s.idempotenceKeyTTL)
	if err != nil {
		s.logger.Warn("Error marking pending creation",
			zap.String("idempotenceKey", idempotenceKey),
			zap.Error(err))
		// Continue anyway, worst case we create multiple sandboxes
	} else if !marked {
		// Someone else is already creating a sandbox for this key
		s.logger.Info("Concurrent creation detected",
			zap.String("idempotenceKey", idempotenceKey))

		// Wait for a short time and check if the sandbox ID is available
		for i := 0; i < 10; i++ { // Try up to 10 times
			time.Sleep(200 * time.Millisecond) // Wait 200ms between checks

			sandboxID, err := s.store.GetSandboxID(ctx, idempotenceKey)
			if err == nil && sandboxID != "" {
				s.logger.Info("Found sandbox created by concurrent request",
					zap.String("sandboxID", sandboxID))
				return sandboxID, false, nil
			}
		}

		// If we still don't have a sandbox after waiting, proceed with creation
		s.logger.Info("Timed out waiting for concurrent creation, proceeding with own creation",
			zap.String("idempotenceKey", idempotenceKey))
	}

	// Generate a unique pod name
	podName := generatePodName()
	s.logger.Info("Generated pod name",
		zap.String("podName", podName),
		zap.String("idempotenceKey", idempotenceKey))

	// Schedule the sandbox on Kubernetes
	podName, err = s.k8sClient.ScheduleSandbox(ctx, podName, metadata)
	if err != nil {
		s.logger.Error("Failed to schedule sandbox",
			zap.String("idempotenceKey", idempotenceKey),
			zap.Error(err))
		// Clean up the pending marker since creation failed
		if cleanupErr := s.store.ReleaseIdempotenceKey(ctx, idempotenceKey); cleanupErr != nil {
			s.logger.Warn("Failed to clean up pending marker",
				zap.String("idempotenceKey", idempotenceKey),
				zap.Error(cleanupErr))
		}
		return "", false, fmt.Errorf("failed to schedule: %v", err)
	}

	// Store the mapping from idempotence key to sandbox ID and complete the pending operation
	if err := s.store.CompletePendingCreation(ctx, idempotenceKey, podName, s.idempotenceKeyTTL); err != nil {
		// Just log the error, don't fail the request
		s.logger.Warn("Failed to store idempotence mapping",
			zap.Error(err))
	} else {
		s.logger.Info("Stored idempotence mapping",
			zap.String("idempotenceKey", idempotenceKey),
			zap.String("podName", podName),
			zap.Duration("ttl", s.idempotenceKeyTTL))
	}

	// Set the initial expiration time for the sandbox
	expirationTime := time.Now().Add(s.sandboxTTL)
	if err := s.store.SetSandboxExpiration(ctx, podName, expirationTime); err != nil {
		s.logger.Warn("Failed to set initial expiration for sandbox",
			zap.String("podName", podName),
			zap.Error(err))
		// Don't fail the request, just log the warning
	} else {
		s.logger.Info("Set expiration for sandbox",
			zap.String("podName", podName),
			zap.Time("expirationTime", expirationTime))
	}

	s.logger.Info("Successfully scheduled sandbox",
		zap.String("idempotenceKey", idempotenceKey),
		zap.String("podName", podName))
	return podName, true, nil
}

// generatePodName creates a pod name with a standard prefix and a random suffix
func generatePodName() string {
	// Use a simple prefix plus a shortened UUID for uniqueness
	// We'll use a shorter UUID to avoid potential truncation issues in processing
	// and ensure it's a fixed length that's compatible with all client components
	prefix := "sandbox"

	// Generate a full UUID
	fullUuid := uuid.New()

	// Convert to a more compact representation - 8 chars should be unique enough
	// for the demo purposes and avoids any truncation issues
	shortUuid := fmt.Sprintf("%08x", fullUuid.ID())

	return fmt.Sprintf("%s-%s", prefix, shortUuid)
}

// ReleaseSandbox handles deleting a sandbox
func (s *SchedulerService) ReleaseSandbox(ctx context.Context, sandboxID string) (bool, error) {
	if sandboxID == "" {
		return false, fmt.Errorf("sandbox ID cannot be empty")
	}

	s.logger.Info("Releasing sandbox", zap.String("sandboxID", sandboxID))

	// Remove from the expiration tracking
	if err := s.store.RemoveSandboxExpiration(ctx, sandboxID); err != nil {
		s.logger.Warn("Failed to remove expiration for sandbox",
			zap.String("sandboxID", sandboxID),
			zap.Error(err))
		// Continue with the release even if we fail to remove the expiration
	}

	// Release the sandbox on Kubernetes
	err := s.k8sClient.ReleaseSandbox(ctx, sandboxID)
	if err != nil {
		s.logger.Error("Failed to release sandbox",
			zap.String("sandboxID", sandboxID),
			zap.Error(err))
		return false, fmt.Errorf("failed to release: %v", err)
	}

	// Mark the sandbox as released to avoid duplicate operations
	if err := s.store.MarkSandboxReleased(ctx, sandboxID, 5*time.Minute); err != nil {
		s.logger.Warn("Failed to mark sandbox as released",
			zap.String("sandboxID", sandboxID),
			zap.Error(err))
		// This is not critical, just a warning
	}

	// Broadcast the terminated event
	broadcastEvent := events.Event{
		SandboxID: sandboxID,
		Type:      schedulerv1.SandboxEventType_SANDBOX_EVENT_TYPE_TERMINATED,
	}

	if err := s.eventBroadcaster.BroadcastEvent(ctx, broadcastEvent); err != nil {
		s.logger.Warn("Failed to broadcast sandbox termination event",
			zap.String("sandboxID", sandboxID),
			zap.Error(err))
		// Don't fail the request just because we couldn't broadcast the event
	}

	s.logger.Info("Successfully released sandbox", zap.String("sandboxID", sandboxID))
	return true, nil
}

// RetainSandbox extends the expiration time of a sandbox
func (s *SchedulerService) RetainSandbox(ctx context.Context, sandboxID string) (time.Time, bool, error) {
	if sandboxID == "" {
		return time.Time{}, false, fmt.Errorf("sandbox ID cannot be empty")
	}

	s.logger.Info("Retaining sandbox", zap.String("sandboxID", sandboxID))

	// Check if the sandbox was already released
	released, err := s.store.IsSandboxReleased(ctx, sandboxID)
	if err != nil {
		s.logger.Warn("Error checking if sandbox is released",
			zap.String("sandboxID", sandboxID),
			zap.Error(err))
		// Continue anyway, worst case we try to extend a released sandbox
	} else if released {
		s.logger.Info("Cannot retain sandbox as it has already been released",
			zap.String("sandboxID", sandboxID))
		return time.Time{}, false, fmt.Errorf("sandbox has already been released")
	}

	// Extend the expiration time
	newExpiration, err := s.store.ExtendSandboxExpiration(ctx, sandboxID, s.sandboxTTL)
	if err != nil {
		s.logger.Error("Failed to extend expiration for sandbox",
			zap.String("sandboxID", sandboxID),
			zap.Error(err))
		return time.Time{}, false, fmt.Errorf("failed to extend expiration: %v", err)
	}

	s.logger.Info("Successfully extended expiration for sandbox",
		zap.String("sandboxID", sandboxID),
		zap.Time("newExpiration", newExpiration))
	return newExpiration, true, nil
}

// CleanupExpiredSandboxes finds and cleans up sandboxes that have expired
func (s *SchedulerService) CleanupExpiredSandboxes(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 10 // Default batch size
	}

	s.logger.Info("Looking for expired sandboxes", zap.Int("batchSize", batchSize))

	// Get sandboxes that have expired
	now := time.Now()
	expiredSandboxes, err := s.store.GetExpiredSandboxes(ctx, now, batchSize)
	if err != nil {
		s.logger.Error("Error getting expired sandboxes", zap.Error(err))
		return 0, fmt.Errorf("failed to get expired sandboxes: %v", err)
	}

	if len(expiredSandboxes) == 0 {
		s.logger.Info("No expired sandboxes found")
		return 0, nil
	}

	s.logger.Info("Found expired sandboxes to clean up", zap.Int("count", len(expiredSandboxes)))

	releasedCount := 0
	for _, sandboxID := range expiredSandboxes {
		// Check if already marked as released to avoid duplicate work
		released, err := s.store.IsSandboxReleased(ctx, sandboxID)
		if err != nil {
			s.logger.Warn("Error checking if sandbox is released",
				zap.String("sandboxID", sandboxID),
				zap.Error(err))
			// Continue to next sandbox
			continue
		}

		if released {
			s.logger.Info("Sandbox is already marked as released, skipping",
				zap.String("sandboxID", sandboxID))

			// Remove from expiration tracking since it's already released
			if err := s.store.RemoveSandboxExpiration(ctx, sandboxID); err != nil {
				s.logger.Warn("Failed to remove expiration for released sandbox",
					zap.String("sandboxID", sandboxID),
					zap.Error(err))
			}

			continue
		}

		// Release the sandbox
		s.logger.Info("Releasing expired sandbox", zap.String("sandboxID", sandboxID))
		success, err := s.ReleaseSandbox(ctx, sandboxID)
		if err != nil {
			s.logger.Error("Failed to release expired sandbox",
				zap.String("sandboxID", sandboxID),
				zap.Error(err))

			// Even if the release failed, still try to broadcast the terminated event
			// as the upstream systems should know this sandbox is no longer usable
			terminatedEvent := events.Event{
				SandboxID: sandboxID,
				Type:      schedulerv1.SandboxEventType_SANDBOX_EVENT_TYPE_TERMINATED,
			}

			if broadcastErr := s.eventBroadcaster.BroadcastEvent(ctx, terminatedEvent); broadcastErr != nil {
				s.logger.Warn("Failed to broadcast sandbox termination event",
					zap.String("sandboxID", sandboxID),
					zap.Error(broadcastErr))
			}

			// Continue to next sandbox
			continue
		}

		if success {
			releasedCount++
			s.logger.Info("Successfully released expired sandbox",
				zap.String("sandboxID", sandboxID))
		}
	}

	s.logger.Info("Cleanup completed",
		zap.Int("releasedCount", releasedCount),
		zap.Int("totalExpired", len(expiredSandboxes)))
	return releasedCount, nil
}

// Close cleans up resources used by the service
func (s *SchedulerService) Close() error {
	// Cancel the event processing context
	s.eventCancel()

	var errors []error

	// Close the store
	if s.store != nil {
		if err := s.store.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	// Close the event broadcaster
	if s.eventBroadcaster != nil {
		if err := s.eventBroadcaster.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors while closing: %v", errors)
	}

	return nil
}
