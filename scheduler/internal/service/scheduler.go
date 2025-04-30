package service

import (
	"context"
	"fmt"
	"strings"
	"time"

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

	// Check if this is a sandbox pod
	if !strings.HasPrefix(podName, "sandbox-") {
		return
	}
	sandboxID, err := types.NewSandboxID(podName)
	if err != nil {
		s.logger.Error("failed to parse sandbox ID", zap.Error(err))
		return
	}

	// Release the sandbox
	_, err = s.ReleaseSandbox(context.Background(), sandboxID)
	if err != nil {
		s.logger.Error("failed to release sandbox after pod deletion event",
			sandboxID.ZapField(),
			zap.Error(err),
		)
		return
	}

	s.logger.Info("released sandbox after pod deletion event",
		sandboxID.ZapField(),
	)
}

// ScheduleSandbox handles scheduling a sandbox
func (s *SchedulerService) ScheduleSandbox(ctx context.Context, idempotenceKey string) (types.SandboxID, error) {
	if idempotenceKey == "" {
		return "", fmt.Errorf("idempotence key cannot be empty")
	}

	// Create log context that we'll build up throughout the function
	logContext := []zap.Field{zap.String("idempotenceKey", idempotenceKey)}

	// Check if we already have a sandbox for this idempotence key
	sandboxID, err := s.store.GetSandboxID(ctx, idempotenceKey)
	if err == nil {
		// We found an existing sandbox for this idempotence key
		logContext = append(logContext, sandboxID.ZapField(), zap.Bool("reused", true))
		s.logger.Info("Found existing sandbox for idempotence key", logContext...)
		return sandboxID, nil // Return the existing sandbox, not newly created
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
					sandboxID.ZapField(),
					zap.Int("attempt", i+1),
					zap.Bool("concurrent", true))
				s.logger.Info("Found sandbox created by concurrent request", logContext...)
				return sandboxID, nil
			}
		}

		logContext = append(logContext, zap.Int("waitAttempts", 10))
		s.logger.Info("Timed out waiting for concurrent creation, proceeding with own creation", logContext...)
	}

	// Schedule the sandbox on Kubernetes
	sandboxID, err = s.k8sClient.ScheduleSandbox(ctx)
	logContext = append(logContext, sandboxID.ZapField())
	if err != nil {
		removeMappingErr := s.store.RemoveSandboxMapping(ctx, sandboxID)
		logContext = append(logContext, zap.NamedError("k8sError", err), zap.NamedError("removeMappingError", removeMappingErr))
		s.logger.Error("Failed to schedule sandbox", logContext...)
		return "", fmt.Errorf("failed to schedule: %v", err)
	}

	errIdempotenceStore := s.store.CompletePendingCreation(ctx, idempotenceKey, sandboxID, s.idempotenceKeyTTL)
	logContext = append(logContext, zap.NamedError("idempotenceStoreError", errIdempotenceStore))
	s.logger.Info("Successfully scheduled sandbox", logContext...)

	return sandboxID, nil
}

// ReleaseSandbox handles deleting a sandbox
func (s *SchedulerService) ReleaseSandbox(ctx context.Context, sandboxID types.SandboxID) (bool, error) {
	errRedis := s.store.RemoveSandboxMapping(ctx, sandboxID)
	errK8s := s.k8sClient.ReleaseSandbox(ctx, sandboxID)

	level := zap.InfoLevel
	if errRedis != nil || errK8s != nil {
		level = zap.WarnLevel
	}
	s.logger.Log(level, "Released sandbox", sandboxID.ZapField(), zap.NamedError("redisError", errRedis), zap.NamedError("k8sError", errK8s))

	return true, nil
}

// CleanupExpiredSandboxes finds and cleans up sandboxes that have been running for too long
func (s *SchedulerService) CleanupExpiredSandboxes(ctx context.Context) (int, error) {
	maxAge := s.sandboxTTL

	// Calculate the cutoff time for old pods
	cutoffTime := time.Now().Add(-maxAge)

	// Stats for summary
	totalReleased := 0
	totalFailed := 0
	var failedIDs []string

	// Paginate through all pods older than the cutoff time
	var continueToken string
	for {
		// Get the next batch of pods older than the cutoff time
		oldPods, nextToken, err := s.k8sClient.GetPodsOlderThan(ctx, cutoffTime, continueToken)
		if err != nil {
			return totalReleased, fmt.Errorf("failed to get old pods: %v", err)
		}

		s.logger.Info("Processing batch of old pods",
			zap.Int("batchSize", len(oldPods)),
			zap.Time("cutoffTime", cutoffTime))

		// Process each old pod
		for _, podName := range oldPods {
			sandboxID, err := types.NewSandboxID(podName)
			if err != nil {
				s.logger.Error("failed to parse sandbox ID", zap.Error(err))
				continue
			}
			success, err := s.ReleaseSandbox(ctx, sandboxID)
			if err != nil {
				totalFailed++
				failedIDs = append(failedIDs, podName)
				s.logger.Warn("Failed to release sandbox",
					zap.String("podName", podName),
					zap.Error(err))
			} else if success {
				totalReleased++
				s.logger.Info("Released old sandbox",
					zap.String("podName", podName))
			}
		}

		// If there's no continuation token, we're done
		if nextToken == "" {
			break
		}

		// Use the next token for the next batch
		continueToken = nextToken
	}

	// Log a summary of the cleanup
	logContext := []zap.Field{
		zap.Time("cutoffTime", cutoffTime),
		zap.Duration("maxAge", maxAge),
		zap.Int("totalReleased", totalReleased),
		zap.Int("totalFailed", totalFailed),
	}

	// Only add failed IDs if there were any
	if totalFailed > 0 {
		logContext = append(logContext, zap.Strings("failedIDs", failedIDs))
		s.logger.Warn("Old sandboxes cleanup completed with some failures", logContext...)
	} else if totalReleased > 0 {
		s.logger.Info("Old sandboxes cleanup completed successfully", logContext...)
	} else {
		// For zero old sandboxes, log at debug level
		s.logger.Debug("No old sandboxes to clean up", logContext...)
	}

	return totalReleased, nil
}

func (s *SchedulerService) Close() error {
	s.eventCancel()
	return nil
}

// IsSandboxReady checks if a sandbox is ready according to Kubernetes
func (s *SchedulerService) IsSandboxReady(ctx context.Context, sandboxID types.SandboxID) (bool, error) {
	if sandboxID == "" {
		return false, fmt.Errorf("sandbox ID cannot be empty")
	}
	// Create log context with sandbox ID
	logContext := []zap.Field{sandboxID.ZapField()}

	// Check with Kubernetes if the sandbox is ready
	ready, err := s.k8sClient.IsSandboxReady(ctx, sandboxID)
	if err != nil {
		logContext = append(logContext, zap.Error(err))
		s.logger.Error("Error checking if sandbox is ready in Kubernetes", logContext...)
		return false, fmt.Errorf("failed to check if sandbox is ready: %v", err)
	}
	return ready, nil
}

// IsSandboxGone checks if a sandbox is gone according to Kubernetes
func (s *SchedulerService) IsSandboxGone(ctx context.Context, sandboxID types.SandboxID) (bool, error) {
	// Create log context with sandbox ID
	logContext := []zap.Field{sandboxID.ZapField()}

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
func (s *SchedulerService) WaitForSandboxReady(ctx context.Context, sandboxID types.SandboxID) (bool, error) {
	ready, err := s.k8sClient.WaitForSandboxReady(ctx, sandboxID)
	if err != nil {
		s.logger.Error("Error waiting for sandbox to be ready in Kubernetes",
			sandboxID.ZapField(),
			zap.Error(err))
		return false, fmt.Errorf("failed to wait for sandbox to be ready: %v", err)
	}
	return ready, nil
}
