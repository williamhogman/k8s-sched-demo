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
	eventType := string(event.EventType)

	// Define bad events that should trigger sandbox release
	isBadEvent := event.EventType == types.PodEventFailed ||
		event.EventType == types.PodEventUnschedulable ||
		event.EventType == types.PodEventTerminated

	// Quick return for non-actionable events
	if !isBadEvent {
		return
	}

	// Check if we've already handled this event
	shouldHandle, _ := s.shouldHandlePodEvent(s.eventCtx, sandboxID)
	if !shouldHandle {
		// Only log once for duplicate events
		s.logger.Debug("Skipping already processed event",
			zap.String("sandboxID", sandboxID),
			zap.String("eventType", eventType),
			zap.String("reason", event.Reason))
		return
	}

	// Create a log context that we can build up
	logContext := []zap.Field{
		zap.String("sandboxID", sandboxID),
		zap.String("eventType", eventType),
		zap.String("reason", event.Reason),
	}
	if event.Message != "" {
		logContext = append(logContext, zap.String("message", event.Message))
	}

	_, err := s.ReleaseSandbox(s.eventCtx, sandboxID)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Error("Failed to release sandbox after bad pod event", logContext...)
	} else {
		s.logger.Info("Released sandbox because of bad pod event", logContext...)
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

	// Create log context that we'll build up throughout the function
	logContext := []zap.Field{zap.String("idempotenceKey", idempotenceKey)}

	// Check if we already have a sandbox for this idempotence key
	sandboxID, err := s.store.GetSandboxID(ctx, idempotenceKey)
	if err == nil && sandboxID != "" {
		// We found an existing sandbox for this idempotence key
		logContext = append(logContext, zap.String("sandboxID", sandboxID), zap.Bool("reused", true))
		s.logger.Info("Found existing sandbox for idempotence key", logContext...)
		return sandboxID, true, nil // Return the existing sandbox, not newly created
	} else if err != nil && err != persistence.ErrNotFound {
		// Only log actual errors, not key not found
		logContext = append(logContext, zap.Error(err))
		s.logger.Warn("Error checking idempotence key", logContext...)
	}

	// Try to mark this idempotence key as pending creation to handle concurrent requests
	marked, err := s.store.MarkPendingCreation(ctx, idempotenceKey, s.idempotenceKeyTTL)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Warn("Error marking pending creation", logContext...)
		// Continue anyway, worst case we create multiple sandboxes
	} else if !marked {
		// Someone else is already creating a sandbox for this key
		s.logger.Debug("Concurrent creation detected", logContext...)

		// Wait for a short time and check if the sandbox ID is available
		for i := 0; i < 10; i++ { // Try up to 10 times
			time.Sleep(200 * time.Millisecond) // Wait 200ms between checks

			sandboxID, err := s.store.GetSandboxID(ctx, idempotenceKey)
			if err == nil && sandboxID != "" {
				logContext = append(logContext,
					zap.String("sandboxID", sandboxID),
					zap.Int("attempt", i+1),
					zap.Bool("concurrent", true))
				s.logger.Info("Found sandbox created by concurrent request", logContext...)
				return sandboxID, false, nil
			}
		}

		logContext = append(logContext, zap.Int("waitAttempts", 10))
		s.logger.Info("Timed out waiting for concurrent creation, proceeding with own creation", logContext...)
	}

	// Generate a unique pod name
	podName := generatePodName()
	logContext = append(logContext, zap.String("podName", podName))

	// Schedule the sandbox on Kubernetes
	podName, err = s.k8sClient.ScheduleSandbox(ctx, podName, metadata)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Error("Failed to schedule sandbox", logContext...)

		// Clean up the pending marker since creation failed
		if cleanupErr := s.store.ReleaseIdempotenceKey(ctx, idempotenceKey); cleanupErr != nil {
			s.logger.Warn("Failed to clean up pending marker",
				append(logContext, zap.Error(cleanupErr))...)
		}

		return "", false, fmt.Errorf("failed to schedule: %v", err)
	}

	// Keep track of any non-fatal issues during the completion process
	var issues []string

	// Store the mapping from idempotence key to sandbox ID and complete the pending operation
	if err := s.store.CompletePendingCreation(ctx, idempotenceKey, podName, s.idempotenceKeyTTL); err != nil {
		issues = append(issues, fmt.Sprintf("failed to store idempotence mapping: %v", err))
	}

	// Set the initial expiration time for the sandbox
	expirationTime := time.Now().Add(s.sandboxTTL)
	logContext = append(logContext, zap.Time("expirationTime", expirationTime))

	if err := s.store.SetSandboxExpiration(ctx, podName, expirationTime); err != nil {
		issues = append(issues, fmt.Sprintf("failed to set expiration: %v", err))
	}

	// Add issues to log context if any occurred
	if len(issues) > 0 {
		logContext = append(logContext, zap.Strings("issues", issues))
		s.logger.Warn("Created sandbox with non-fatal issues", logContext...)
	} else {
		s.logger.Info("Successfully scheduled sandbox", logContext...)
	}

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

	// Create log context with sandbox ID
	logContext := []zap.Field{zap.String("sandboxID", sandboxID)}

	// Keep track of any non-fatal issues
	var issues []string

	// Remove from the expiration tracking
	if err := s.store.RemoveSandboxExpiration(ctx, sandboxID); err != nil {
		issues = append(issues, fmt.Sprintf("failed to remove expiration: %v", err))
	}

	// Release the sandbox on Kubernetes
	err := s.k8sClient.ReleaseSandbox(ctx, sandboxID)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Error("Failed to release sandbox in Kubernetes", logContext...)
		return false, fmt.Errorf("failed to release: %v", err)
	}

	// Mark the sandbox as released to avoid duplicate operations
	if err := s.store.MarkSandboxReleased(ctx, sandboxID, 5*time.Minute); err != nil {
		issues = append(issues, fmt.Sprintf("failed to mark as released: %v", err))
	}

	// Broadcast the terminated event
	broadcastEvent := events.Event{
		SandboxID: sandboxID,
		Type:      schedulerv1.SandboxEventType_SANDBOX_EVENT_TYPE_TERMINATED,
	}

	if err := s.eventBroadcaster.BroadcastEvent(ctx, broadcastEvent); err != nil {
		issues = append(issues, fmt.Sprintf("failed to broadcast event: %v", err))
	}

	// Add issues to log context if any occurred
	if len(issues) > 0 {
		logContext = append(logContext, zap.Strings("issues", issues))
		s.logger.Warn("Released sandbox with non-fatal issues", logContext...)
	} else {
		s.logger.Info("Successfully released sandbox", logContext...)
	}

	return true, nil
}

// RetainSandbox extends the expiration time of a sandbox
func (s *SchedulerService) RetainSandbox(ctx context.Context, sandboxID string) (time.Time, bool, error) {
	if sandboxID == "" {
		return time.Time{}, false, fmt.Errorf("sandbox ID cannot be empty")
	}

	// Create base log context
	logContext := []zap.Field{zap.String("sandboxID", sandboxID)}

	// Check if the sandbox was already released
	released, err := s.store.IsSandboxReleased(ctx, sandboxID)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Warn("Error checking if sandbox is released", logContext...)
		// Continue anyway, worst case we try to extend a released sandbox
	} else if released {
		s.logger.Info("Cannot retain sandbox as it has already been released", logContext...)
		return time.Time{}, false, fmt.Errorf("sandbox has already been released")
	}

	// Extend the expiration time
	newExpiration, err := s.store.ExtendSandboxExpiration(ctx, sandboxID, s.sandboxTTL)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Error("Failed to extend expiration for sandbox", logContext...)
		return time.Time{}, false, fmt.Errorf("failed to extend expiration: %v", err)
	}

	// Log success with updated expiration time
	logContext = append(logContext, zap.Time("newExpiration", newExpiration))
	s.logger.Info("Successfully extended sandbox expiration", logContext...)

	return newExpiration, true, nil
}

// CleanupExpiredSandboxes finds and cleans up sandboxes that have expired
func (s *SchedulerService) CleanupExpiredSandboxes(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 10 // Default batch size
	}

	// Get sandboxes that have expired
	now := time.Now()
	expiredSandboxes, err := s.store.GetExpiredSandboxes(ctx, now, batchSize)
	if err != nil {
		return 0, fmt.Errorf("failed to get expired sandboxes: %v", err)
	}

	// Track counts for summary
	totalCount := len(expiredSandboxes)
	releasedCount := 0
	skippedCount := 0
	failedCount := 0
	var failedIDs []string

	// If there are expired sandboxes to process
	if totalCount > 0 {
		// Process each expired sandbox
		for _, sandboxID := range expiredSandboxes {
			success, err := s.ReleaseSandbox(ctx, sandboxID)
			if err != nil {
				failedCount++
				failedIDs = append(failedIDs, sandboxID)
			} else if success {
				releasedCount++
			}
		}
	}

	// Log a single summary line with all the stats
	logContext := []zap.Field{
		zap.Int("batchSize", batchSize),
		zap.Int("expiredTotal", totalCount),
		zap.Int("released", releasedCount),
		zap.Int("skipped", skippedCount),
		zap.Int("failed", failedCount),
	}

	// Only add failed IDs if there were any
	if failedCount > 0 {
		logContext = append(logContext, zap.Strings("failedIDs", failedIDs))
		s.logger.Warn("Expired sandboxes cleanup completed with some failures", logContext...)
	} else if totalCount > 0 {
		s.logger.Info("Expired sandboxes cleanup completed successfully", logContext...)
	} else {
		// For zero expired sandboxes, log at debug level
		s.logger.Debug("No expired sandboxes to clean up", logContext...)
	}

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
