package main

import (
	"context"
	"log"
	"time"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
)

// CleanupManager handles the periodic cleanup of expired sandboxes
type CleanupManager struct {
	service      *service.SchedulerService
	intervalSecs int
	batchSize    int
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewCleanupManager creates a new cleanup manager
func NewCleanupManager(service *service.SchedulerService, intervalSecs, batchSize int) *CleanupManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &CleanupManager{
		service:      service,
		intervalSecs: intervalSecs,
		batchSize:    batchSize,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start begins the cleanup job in a goroutine
func (cm *CleanupManager) Start() {
	log.Printf("Starting sandbox cleanup job with interval=%ds, batch size=%d", cm.intervalSecs, cm.batchSize)
	go cm.runCleanupJob()
}

// Stop cancels the cleanup job
func (cm *CleanupManager) Stop() {
	log.Println("Stopping cleanup job")
	cm.cancel()
}

// runCleanupJob periodically runs the cleanup job to remove expired sandboxes
func (cm *CleanupManager) runCleanupJob() {
	ticker := time.NewTicker(time.Duration(cm.intervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.ctx.Done():
			log.Println("Cleanup job shutting down")
			return
		case <-ticker.C:
			// Run the cleanup job
			count, err := cm.service.CleanupExpiredSandboxes(cm.ctx, cm.batchSize)
			if err != nil {
				log.Printf("Error during sandbox cleanup: %v", err)
			} else if count > 0 {
				log.Printf("Cleanup job released %d expired sandboxes", count)
			}
		}
	}
}
