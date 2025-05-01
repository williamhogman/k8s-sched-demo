package service

import (
	"context"
	"fmt"
	"time"

	k8sclient "github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	persistence "github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
	"go.uber.org/zap"
)

// SchedulerServiceConfig contains configuration for the scheduler service
type SchedulerServiceConfig struct {
	SandboxTTL time.Duration
}

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
		k8sClient:   k8sClient,
		store:       store,
		sandboxTTL:  config.SandboxTTL,
		logger:      logger,
		eventCtx:    eventCtx,
		eventCancel: eventCancel,
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
	s.ReleaseSandbox(context.Background(), event.SandboxID)
	s.logger.Info("released sandbox after pod deletion event",
		event.SandboxID.ZapField(),
	)
}

// ScheduleSandbox handles scheduling a sandbox
func (s *SchedulerService) ScheduleSandbox(ctx context.Context, idempotenceKey string) (types.SandboxID, error) {
	if idempotenceKey == "" {
		return "", fmt.Errorf("idempotence key cannot be empty")
	}

	// Create log context that we'll build up throughout the function
	logContext := []zap.Field{zap.String("idempotenceKey", idempotenceKey)}

	// Create a context with timeout for the claim operation
	claimCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Try to claim or wait for the idempotence key
	result := s.store.ClaimOrWait(claimCtx, idempotenceKey)
	defer result.Drop(ctx)

	// Handle existing sandbox case
	if result.HasExistingSandbox() {
		logContext = append(logContext, result.SandboxID.ZapField(), zap.Bool("reused", true))
		s.logger.Info("Using existing sandbox", logContext...)
		return result.SandboxID, nil
	}

	// Handle timeout errors specifically
	if result.IsTimeout() {
		return "", fmt.Errorf("timeout waiting for idempotence claim: %v", result.Error())
	} else if result.HasError() {
		logContext = append(logContext, zap.String("error", result.Error()))
		s.logger.Warn("Error during idempotence claim, proceeding with creation", logContext...)
	} else if result.Claimed {
		s.logger.Debug("Successfully claimed idempotence key", logContext...)
	}

	// At this point, we need to create a new sandbox
	sandboxID, err := s.k8sClient.ScheduleSandbox(ctx)
	logContext = append(logContext, sandboxID.ZapField())
	if err != nil {
		logContext = append(logContext, zap.NamedError("k8sError", err))
		s.logger.Error("Failed to schedule sandbox", logContext...)
		return "", fmt.Errorf("failed to schedule: %v", err)
	}

	idempotenceErr := result.Complete(ctx, sandboxID)
	logContext = append(logContext, zap.NamedError("idempotenceError", idempotenceErr))
	s.logger.Info("Successfully scheduled sandbox", logContext...)

	return sandboxID, nil
}

// ReleaseSandbox handles deleting a sandbox
func (s *SchedulerService) ReleaseSandbox(ctx context.Context, sandboxID types.SandboxID) {
	errRedis := s.store.RemoveSandboxMapping(ctx, sandboxID)
	errK8s := s.k8sClient.ReleaseSandbox(ctx, sandboxID)

	level := zap.InfoLevel
	if errRedis != nil || errK8s != nil {
		level = zap.WarnLevel
	}
	s.logger.Log(level, "Released sandbox", sandboxID.ZapField(), zap.NamedError("redisError", errRedis), zap.NamedError("k8sError", errK8s))
}

// CleanupExpiredSandboxes finds and cleans up sandboxes that have been running for too long
func (s *SchedulerService) CleanupExpiredSandboxes(ctx context.Context) (int, error) {
	maxAge := s.sandboxTTL

	// Calculate the cutoff time for old pods
	cutoffTime := time.Now().Add(-maxAge)

	// Stats for summary
	totalReleased := 0
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
			s.ReleaseSandbox(ctx, sandboxID)
			totalReleased++
			s.logger.Info("Released old sandbox", zap.String("podName", podName))
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
	}

	// Only add failed IDs if there were any
	if totalReleased > 0 {
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
