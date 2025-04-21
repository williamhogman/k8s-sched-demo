package cleanup

import (
	"context"
	"time"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
	"go.uber.org/zap"
)

// Manager handles the periodic cleanup of expired sandboxes
type Manager struct {
	service      *service.SchedulerService
	intervalSecs int
	batchSize    int
	ctx          context.Context
	cancel       context.CancelFunc
	logger       *zap.Logger
}

// NewManager creates a new cleanup manager
func NewManager(service *service.SchedulerService, intervalSecs, batchSize int, logger *zap.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		service:      service,
		intervalSecs: intervalSecs,
		batchSize:    batchSize,
		ctx:          ctx,
		cancel:       cancel,
		logger:       logger.Named("cleanup-job"),
	}
}

// Start begins the cleanup job in a goroutine
func (cm *Manager) Start() {
	cm.logger.Info("Starting cleanup job",
		zap.Int("intervalSeconds", cm.intervalSecs),
		zap.Int("batchSize", cm.batchSize))
	go cm.runCleanupJob()
}

// Stop cancels the cleanup job
func (cm *Manager) Stop() {
	cm.logger.Info("Stopping cleanup job")
	cm.cancel()
}

// runCleanupJob periodically runs the cleanup job to remove expired sandboxes
func (cm *Manager) runCleanupJob() {
	ticker := time.NewTicker(time.Duration(cm.intervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.ctx.Done():
			cm.logger.Info("Cleanup job shutting down")
			return
		case <-ticker.C:
			// Run the cleanup job
			count, err := cm.service.CleanupExpiredSandboxes(cm.ctx, cm.batchSize)
			if err != nil {
				cm.logger.Error("Error during sandbox cleanup", zap.Error(err))
			} else if count > 0 {
				cm.logger.Info("Cleanup completed", zap.Int("releasedCount", count))
			}
		}
	}
}
