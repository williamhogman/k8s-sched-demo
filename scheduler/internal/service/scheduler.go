package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	persistence "github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
)

// K8sClientInterface defines the interface for Kubernetes client
type K8sClientInterface interface {
	ScheduleSandbox(ctx context.Context, podName string, metadata map[string]string) (string, error)
	// We don't define the template-based methods here to avoid import cycles
	// Template-based pod creation should be used directly with the k8sclient package
	ReleaseSandbox(ctx context.Context, sandboxID string) error
}

// SchedulerService implements the scheduling service logic
type SchedulerService struct {
	k8sClient         K8sClientInterface
	store             persistence.Store
	idempotenceKeyTTL time.Duration
	sandboxTTL        time.Duration
}

// SchedulerServiceConfig contains configuration for the scheduler service
type SchedulerServiceConfig struct {
	IdempotenceKeyTTL time.Duration
	SandboxTTL        time.Duration
}

// NewSchedulerService creates a new scheduler service
func NewSchedulerService(k8sClient K8sClientInterface, store persistence.Store, config SchedulerServiceConfig) *SchedulerService {
	return &SchedulerService{
		k8sClient:         k8sClient,
		store:             store,
		idempotenceKeyTTL: config.IdempotenceKeyTTL,
		sandboxTTL:        config.SandboxTTL,
	}
}

// ScheduleSandbox handles scheduling a sandbox
func (s *SchedulerService) ScheduleSandbox(ctx context.Context, idempotenceKey string, metadata map[string]string) (string, bool, error) {
	if idempotenceKey == "" {
		return "", false, fmt.Errorf("idempotence key cannot be empty")
	}

	log.Printf("Scheduling sandbox with key %s", idempotenceKey)

	// Check if we already have a sandbox for this idempotence key
	sandboxID, err := s.store.GetSandboxID(ctx, idempotenceKey)
	if err == nil && sandboxID != "" {
		// We found an existing sandbox for this idempotence key
		log.Printf("Found existing sandbox with idempotence key %s: %s", idempotenceKey, sandboxID)
		return sandboxID, true, nil // Return the existing sandbox, not newly created
	} else if err != nil && err != persistence.ErrNotFound {
		// Only log actual errors, not key not found
		log.Printf("Error when checking idempotence key %s: %v", idempotenceKey, err)
	}

	// Try to mark this idempotence key as pending creation to handle concurrent requests
	marked, err := s.store.MarkPendingCreation(ctx, idempotenceKey, s.idempotenceKeyTTL)
	if err != nil {
		log.Printf("Error marking pending creation for key %s: %v", idempotenceKey, err)
		// Continue anyway, worst case we create multiple sandboxes
	} else if !marked {
		// Someone else is already creating a sandbox for this key
		log.Printf("Concurrent creation detected for key %s, waiting...", idempotenceKey)

		// Wait for a short time and check if the sandbox ID is available
		for i := 0; i < 10; i++ { // Try up to 10 times
			time.Sleep(200 * time.Millisecond) // Wait 200ms between checks

			sandboxID, err := s.store.GetSandboxID(ctx, idempotenceKey)
			if err == nil && sandboxID != "" {
				log.Printf("Found sandbox created by concurrent request: %s", sandboxID)
				return sandboxID, false, nil
			}
		}

		// If we still don't have a sandbox after waiting, proceed with creation
		log.Printf("Timed out waiting for concurrent creation, proceeding with own creation for key %s", idempotenceKey)
	}

	// Generate a unique pod name
	podName := generatePodName()
	log.Printf("Generated pod name: %s for idempotence key: %s", podName, idempotenceKey)

	// Schedule the sandbox on Kubernetes
	podName, err = s.k8sClient.ScheduleSandbox(ctx, podName, metadata)
	if err != nil {
		log.Printf("Failed to schedule sandbox with key %s: %v", idempotenceKey, err)
		// Clean up the pending marker since creation failed
		if cleanupErr := s.store.ReleaseIdempotenceKey(ctx, idempotenceKey); cleanupErr != nil {
			log.Printf("Warning: failed to clean up pending marker: %v", cleanupErr)
		}
		return "", false, fmt.Errorf("failed to schedule: %v", err)
	}

	// Store the mapping from idempotence key to sandbox ID and complete the pending operation
	if err := s.store.CompletePendingCreation(ctx, idempotenceKey, podName, s.idempotenceKeyTTL); err != nil {
		// Just log the error, don't fail the request
		log.Printf("Failed to store idempotence mapping: %v", err)
	} else {
		log.Printf("Stored idempotence mapping: %s -> %s (TTL: %v)",
			idempotenceKey, podName, s.idempotenceKeyTTL)
	}

	// Set the initial expiration time for the sandbox
	expirationTime := time.Now().Add(s.sandboxTTL)
	if err := s.store.SetSandboxExpiration(ctx, podName, expirationTime); err != nil {
		log.Printf("Warning: failed to set initial expiration for sandbox %s: %v", podName, err)
		// Don't fail the request, just log the warning
	} else {
		log.Printf("Set expiration for sandbox %s to %v", podName, expirationTime)
	}

	log.Printf("Successfully scheduled sandbox with key %s as pod %s", idempotenceKey, podName)
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

	log.Printf("Releasing sandbox with ID %s", sandboxID)

	// Remove from the expiration tracking
	if err := s.store.RemoveSandboxExpiration(ctx, sandboxID); err != nil {
		log.Printf("Warning: failed to remove expiration for sandbox %s: %v", sandboxID, err)
		// Continue with the release even if we fail to remove the expiration
	}

	// Release the sandbox on Kubernetes
	err := s.k8sClient.ReleaseSandbox(ctx, sandboxID)
	if err != nil {
		log.Printf("Failed to release sandbox %s: %v", sandboxID, err)
		return false, fmt.Errorf("failed to release: %v", err)
	}

	// Mark the sandbox as released to avoid duplicate operations
	if err := s.store.MarkSandboxReleased(ctx, sandboxID, 5*time.Minute); err != nil {
		log.Printf("Warning: failed to mark sandbox %s as released: %v", sandboxID, err)
		// This is not critical, just a warning
	}

	log.Printf("Successfully released sandbox %s", sandboxID)
	return true, nil
}

// RetainSandbox extends the expiration time of a sandbox
func (s *SchedulerService) RetainSandbox(ctx context.Context, sandboxID string) (time.Time, bool, error) {
	if sandboxID == "" {
		return time.Time{}, false, fmt.Errorf("sandbox ID cannot be empty")
	}

	log.Printf("Retaining sandbox with ID %s", sandboxID)

	// Check if the sandbox was already released
	released, err := s.store.IsSandboxReleased(ctx, sandboxID)
	if err != nil {
		log.Printf("Error checking if sandbox %s is released: %v", sandboxID, err)
		// Continue anyway, worst case we try to extend a released sandbox
	} else if released {
		log.Printf("Cannot retain sandbox %s as it has already been released", sandboxID)
		return time.Time{}, false, fmt.Errorf("sandbox has already been released")
	}

	// Extend the expiration time
	newExpiration, err := s.store.ExtendSandboxExpiration(ctx, sandboxID, s.sandboxTTL)
	if err != nil {
		log.Printf("Failed to extend expiration for sandbox %s: %v", sandboxID, err)
		return time.Time{}, false, fmt.Errorf("failed to extend expiration: %v", err)
	}

	log.Printf("Successfully extended expiration for sandbox %s to %v", sandboxID, newExpiration)
	return newExpiration, true, nil
}

// CleanupExpiredSandboxes finds and cleans up sandboxes that have expired
func (s *SchedulerService) CleanupExpiredSandboxes(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 10 // Default batch size
	}

	log.Printf("Looking for expired sandboxes (batch size: %d)", batchSize)

	// Get sandboxes that have expired
	now := time.Now()
	expiredSandboxes, err := s.store.GetExpiredSandboxes(ctx, now, batchSize)
	if err != nil {
		log.Printf("Error getting expired sandboxes: %v", err)
		return 0, fmt.Errorf("failed to get expired sandboxes: %v", err)
	}

	if len(expiredSandboxes) == 0 {
		log.Printf("No expired sandboxes found")
		return 0, nil
	}

	log.Printf("Found %d expired sandboxes to clean up", len(expiredSandboxes))

	releasedCount := 0
	for _, sandboxID := range expiredSandboxes {
		// Check if already marked as released to avoid duplicate work
		released, err := s.store.IsSandboxReleased(ctx, sandboxID)
		if err != nil {
			log.Printf("Error checking if sandbox %s is released: %v", sandboxID, err)
			// Continue to next sandbox
			continue
		}

		if released {
			log.Printf("Sandbox %s is already marked as released, skipping", sandboxID)

			// Remove from expiration tracking since it's already released
			if err := s.store.RemoveSandboxExpiration(ctx, sandboxID); err != nil {
				log.Printf("Warning: failed to remove expiration for released sandbox %s: %v", sandboxID, err)
			}

			continue
		}

		// Release the sandbox
		log.Printf("Releasing expired sandbox %s", sandboxID)
		success, err := s.ReleaseSandbox(ctx, sandboxID)
		if err != nil {
			log.Printf("Failed to release expired sandbox %s: %v", sandboxID, err)
			// Continue to next sandbox
			continue
		}

		if success {
			releasedCount++
			log.Printf("Successfully released expired sandbox %s", sandboxID)
		}
	}

	log.Printf("Cleanup completed: released %d expired sandboxes out of %d", releasedCount, len(expiredSandboxes))
	return releasedCount, nil
}

// Close cleans up resources used by the service
func (s *SchedulerService) Close() error {
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}
