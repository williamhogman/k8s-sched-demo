package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	k8sclient "github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	persistence "github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
	"go.uber.org/zap"
)

// SchedulerService implements the scheduling service logic
type SchedulerService struct {
	k8sClient         k8sclient.K8sClientInterface
	store             persistence.Store
	idempotenceKeyTTL time.Duration
	sandboxTTL        time.Duration
	logger            *zap.Logger
	// Context for event processing
	eventCtx    context.Context
	eventCancel context.CancelFunc
	// Flag to indicate if event processing is running
	eventProcessingRunning bool
}

// SchedulerServiceConfig contains configuration for the scheduler service
type SchedulerServiceConfig struct {
	IdempotenceKeyTTL time.Duration
	SandboxTTL        time.Duration
}

// NewSchedulerService creates a new scheduler service
func NewSchedulerService(
	k8sClient k8sclient.K8sClientInterface,
	store persistence.Store,
	config SchedulerServiceConfig,
	logger *zap.Logger,
) *SchedulerService {
	// Create a context for event processing
	eventCtx, eventCancel := context.WithCancel(context.Background())

	s := &SchedulerService{
		k8sClient:         k8sClient,
		store:             store,
		idempotenceKeyTTL: config.IdempotenceKeyTTL,
		sandboxTTL:        config.SandboxTTL,
		logger:            logger,
		eventCtx:          eventCtx,
		eventCancel:       eventCancel,
	}
	return s
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

	// Extract pod name from the event
	podName := event.PodName
	if podName == "" {
		s.logger.Warn("received pod event without pod name",
			zap.String("event_reason", event.Reason),
			zap.String("event_type", string(event.EventType)),
		)
		return
	}

	// Check if this is a sandbox pod
	if !strings.HasPrefix(podName, "sandbox-") {
		return
	}

	// Extract sandbox ID from pod name
	sandboxID := strings.TrimPrefix(podName, "sandbox-")

	// Use the new RemoveSandboxMapping function to clean up all mappings
	if err := s.store.RemoveSandboxMapping(context.Background(), sandboxID); err != nil {
		s.logger.Error("failed to remove sandbox mappings",
			zap.String("sandbox_id", sandboxID),
			zap.Error(err),
		)
	} else {
		s.logger.Info("removed all sandbox mappings",
			zap.String("sandbox_id", sandboxID),
		)
	}

	// Release the sandbox
	_, err := s.ReleaseSandbox(context.Background(), podName)
	if err != nil {
		s.logger.Error("failed to release sandbox after pod deletion event",
			zap.String("sandbox_id", sandboxID),
			zap.Error(err),
		)
		return
	}

	s.logger.Info("released sandbox after pod deletion event",
		zap.String("sandbox_id", sandboxID),
	)
}

// findProjectForSandbox finds the project ID associated with a sandbox
func (s *SchedulerService) findProjectForSandbox(ctx context.Context, sandboxID string) (string, error) {
	// Use the store to find the project ID
	return s.store.FindProjectForSandbox(ctx, sandboxID)
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

// generatePodName creates a pod name with a standard prefix and a random base36 encoded number
// that is valid for Kubernetes naming requirements
func generatePodName() string {
	// Simple prefix + random number approach
	prefix := "sandbox"

	// Generate a random number
	randomNum := time.Now().UnixNano() + int64(uuid.New()[0])

	// Encode the random number in base36
	id := encodeBase36(randomNum)

	// Truncate to keep the name reasonably short
	if len(id) > 20 {
		id = id[:20]
	}

	return fmt.Sprintf("%s-%s", prefix, id)
}

// encodeBase36 encodes an integer as a base36 string (0-9a-z)
func encodeBase36(n int64) string {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return "0"
	}

	var result strings.Builder
	for n > 0 {
		result.WriteByte(charset[n%36])
		n /= 36
	}

	// Reverse the string
	runes := []rune(result.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
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

	// Find the idempotence key associated with this sandbox
	idempotenceKey, err := s.store.FindIdempotenceKeyForSandbox(ctx, sandboxID)
	if err != nil {
		issues = append(issues, fmt.Sprintf("failed to find idempotence key for sandbox: %v", err))
	} else if idempotenceKey != "" {
		// Release the idempotence key
		if err := s.store.ReleaseIdempotenceKey(ctx, idempotenceKey); err != nil {
			issues = append(issues, fmt.Sprintf("failed to release idempotence key: %v", err))
		}
	}

	// Find and remove any project-sandbox mapping
	projectID, err := s.findProjectForSandbox(ctx, sandboxID)
	if err != nil {
		issues = append(issues, fmt.Sprintf("failed to find project for sandbox: %v", err))
	} else if projectID != "" {
		if err := s.store.RemoveProjectSandbox(ctx, projectID); err != nil {
			issues = append(issues, fmt.Sprintf("failed to remove project-sandbox mapping: %v", err))
		} else {
			s.logger.Info("removed project-sandbox mapping",
				zap.String("sandbox_id", sandboxID),
				zap.String("project_id", projectID),
			)
		}
	}

	// Remove from the expiration tracking
	if err := s.store.RemoveSandboxExpiration(ctx, sandboxID); err != nil {
		issues = append(issues, fmt.Sprintf("failed to remove expiration: %v", err))
	}

	// Release the sandbox on Kubernetes
	err = s.k8sClient.ReleaseSandbox(ctx, sandboxID)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Error("Failed to release sandbox in Kubernetes", logContext...)
		return false, fmt.Errorf("failed to release: %v", err)
	}

	// Mark the sandbox as released to avoid duplicate operations
	if err := s.store.MarkSandboxReleased(ctx, sandboxID, 5*time.Minute); err != nil {
		issues = append(issues, fmt.Sprintf("failed to mark as released: %v", err))
	}

	// Use the new RemoveSandboxMapping function to clean up all mappings
	if err := s.store.RemoveSandboxMapping(ctx, sandboxID); err != nil {
		issues = append(issues, fmt.Sprintf("failed to remove sandbox mappings: %v", err))
	} else {
		s.logger.Info("removed all sandbox mappings",
			zap.String("sandbox_id", sandboxID),
		)
	}

	if len(issues) > 0 {
		logContext = append(logContext, zap.Strings("issues", issues))
		s.logger.Warn("Released sandbox with non-fatal issues", logContext...)
	} else {
		s.logger.Info("Successfully released sandbox", logContext...)
	}

	return true, nil
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

	if len(errors) > 0 {
		return fmt.Errorf("errors while closing: %v", errors)
	}

	return nil
}

// IsSandboxReady checks if a sandbox is ready according to Kubernetes
func (s *SchedulerService) IsSandboxReady(ctx context.Context, sandboxID string) (bool, error) {
	if sandboxID == "" {
		return false, fmt.Errorf("sandbox ID cannot be empty")
	}

	// Create log context with sandbox ID
	logContext := []zap.Field{zap.String("sandboxID", sandboxID)}

	// Check if the sandbox exists in our store
	_, err := s.store.GetSandboxID(ctx, sandboxID)
	if err != nil {
		if err == persistence.ErrNotFound {
			s.logger.Debug("Sandbox not found in store", logContext...)
			return false, nil
		}
		logContext = append(logContext, zap.Error(err))
		s.logger.Warn("Error checking sandbox in store", logContext...)
		// Continue anyway, we'll check with Kubernetes
	}

	// Check with Kubernetes if the sandbox is ready
	ready, err := s.k8sClient.IsSandboxReady(ctx, sandboxID)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Error("Error checking if sandbox is ready in Kubernetes", logContext...)
		return false, fmt.Errorf("failed to check if sandbox is ready: %v", err)
	}

	if ready {
		s.logger.Info("Sandbox is ready", logContext...)
	} else {
		s.logger.Debug("Sandbox is not ready yet", logContext...)
	}

	return ready, nil
}

// IsSandboxGone checks if a sandbox is gone according to Kubernetes
func (s *SchedulerService) IsSandboxGone(ctx context.Context, sandboxID string) (bool, error) {
	if sandboxID == "" {
		return false, fmt.Errorf("sandbox ID cannot be empty")
	}

	// Create log context with sandbox ID
	logContext := []zap.Field{zap.String("sandboxID", sandboxID)}

	released, err := s.store.IsSandboxReleased(ctx, sandboxID)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Warn("Error checking if sandbox is released in store", logContext...)
	} else if released {
		return true, nil
	}

	// Check with Kubernetes if the sandbox is gone
	gone, err := s.k8sClient.IsSandboxGone(ctx, sandboxID)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Error("Error checking if sandbox is gone in Kubernetes", logContext...)
		return false, fmt.Errorf("failed to check if sandbox is gone: %v", err)
	}
	return gone, nil
}

// WaitForSandboxReady waits for a sandbox to be ready according to Kubernetes
func (s *SchedulerService) WaitForSandboxReady(ctx context.Context, sandboxID string, timeout time.Duration) (bool, error) {
	if sandboxID == "" {
		return false, fmt.Errorf("sandbox ID cannot be empty")
	}

	// Create log context with sandbox ID
	logContext := []zap.Field{
		zap.String("sandboxID", sandboxID),
		zap.Duration("timeout", timeout),
	}
	s.logger.Info("Waiting for sandbox to be ready", logContext...)

	// Wait for the sandbox to be ready in Kubernetes
	ready, err := s.k8sClient.WaitForSandboxReady(ctx, sandboxID, timeout)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Error("Error waiting for sandbox to be ready in Kubernetes", logContext...)
		return false, fmt.Errorf("failed to wait for sandbox to be ready: %v", err)
	}

	if ready {
		s.logger.Info("Sandbox is ready", logContext...)
	} else {
		s.logger.Warn("Sandbox is not ready after timeout", logContext...)
	}

	return ready, nil
}
