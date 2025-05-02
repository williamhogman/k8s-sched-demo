package sandboxpool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/scheduler"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

// ServiceParams contains dependencies for the sandboxpool service
type ServiceParams struct {
	fx.In

	Config           *config.Config
	PoolStore        persistence.SandboxPoolStore
	SchedulerService *scheduler.Service
	Logger           *zap.Logger
}

// Service implements the sandbox pool management
type Service struct {
	poolStore        persistence.SandboxPoolStore
	schedulerService *scheduler.Service
	config           *config.Config
	logger           *zap.Logger

	// Context for background tasks
	ctx        context.Context
	cancelFunc context.CancelFunc

	// Mutex for concurrent operations
	mu sync.Mutex
}

// New creates a new sandbox pool service
func New(params ServiceParams) *Service {
	ctx, cancel := context.WithCancel(context.Background())

	return &Service{
		poolStore:        params.PoolStore,
		schedulerService: params.SchedulerService,
		config:           params.Config,
		logger:           params.Logger.Named("sandboxpool"),
		ctx:              ctx,
		cancelFunc:       cancel,
	}
}

// Start begins the background pool maintenance job
func (s *Service) Start() {
	if !s.config.PoolEnabled {
		s.logger.Info("Sandbox pool is disabled")
		return
	}

	s.logger.Info("Starting sandbox pool maintenance",
		zap.Int("poolSize", s.config.PoolSize),
		zap.Int("refreshInterval", s.config.PoolRefreshIntervalSecs),
		zap.Duration("sandboxTimeout", s.config.PoolSandboxTimeout))

	go s.runMaintenanceJob()
}

// Stop stops the background pool maintenance job
func (s *Service) Stop() {
	s.logger.Info("Stopping sandbox pool maintenance")
	s.cancelFunc()
}

// GetSandbox gets a sandbox from the pool or creates a new one if pool is empty
func (s *Service) GetSandbox(ctx context.Context, idempotenceKey string) (types.SandboxID, error) {
	// Try to get a sandbox from the pool
	sandboxID, err := s.poolStore.GetReadySandbox(ctx)
	go s.triggerMaintenance()
	return sandboxID, err
}

// runMaintenanceJob periodically checks and maintains the sandbox pool
func (s *Service) runMaintenanceJob() {
	ticker := time.NewTicker(time.Duration(s.config.PoolRefreshIntervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("Stopping pool maintenance job")
			return
		case <-ticker.C:
			if err := s.maintainPool(); err != nil {
				s.logger.Error("Error maintaining sandbox pool", zap.Error(err))
			}
		}
	}
}

// triggerMaintenance triggers an immediate pool maintenance
func (s *Service) triggerMaintenance() {
	if err := s.maintainPool(); err != nil {
		s.logger.Error("Error during triggered maintenance", zap.Error(err))
	}
}

// maintainPool checks the pool size and schedules new sandboxes if needed
func (s *Service) maintainPool() error {
	// Use a non-blocking lock attempt to prevent multiple maintenance operations
	// from running concurrently while avoiding deadlocks
	if !s.mu.TryLock() {
		s.logger.Debug("Pool maintenance already in progress")
		return nil
	}
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// Check current pool size
	readyCount, err := s.poolStore.CountReadySandboxes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get ready sandboxes count: %v", err)
	}

	pendingCount, err := s.poolStore.CountPendingSandboxes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending sandboxes count: %v", err)
	}

	s.logger.Debug("Current pool status",
		zap.Int64("readyCount", readyCount),
		zap.Int64("pendingCount", pendingCount),
		zap.Int("targetSize", s.config.PoolSize))

	// Check and move ready sandboxes from pending to ready
	if pendingCount > 0 {
		if err := s.checkPendingSandboxes(ctx); err != nil {
			s.logger.Error("Error checking pending sandboxes", zap.Error(err))
			// Continue to create new sandboxes anyway
		}
	}

	// Recalculate pool size after checking pending
	readyCount, _ = s.poolStore.CountReadySandboxes(ctx)
	pendingCount, _ = s.poolStore.CountPendingSandboxes(ctx)

	// Calculate how many new sandboxes to create
	needed := s.config.PoolSize - int(readyCount) - int(pendingCount)

	// Check if we need to create more sandboxes
	if needed <= 0 {
		s.logger.Debug("Pool is adequately sized, no new sandboxes needed")
		return nil
	}

	// Check if we can create more pending sandboxes
	if pendingCount >= int64(s.config.PoolMaxPending) {
		s.logger.Warn("Too many pending sandboxes, waiting for them to become ready",
			zap.Int64("pendingCount", pendingCount),
			zap.Int("maxPending", s.config.PoolMaxPending))
		return nil
	}

	// Create new sandboxes up to the needed amount or max pending limit
	maxToCreate := min(needed, s.config.PoolMaxPending-int(pendingCount))
	s.logger.Info("Creating new sandboxes for pool", zap.Int("count", maxToCreate))

	for i := 0; i < maxToCreate; i++ {
		if err := s.createPoolSandbox(ctx); err != nil {
			s.logger.Error("Failed to create pool sandbox", zap.Error(err), zap.Int("index", i))
			// Continue with creating other sandboxes
			continue
		}
	}

	return nil
}

// checkPendingSandboxes checks if any pending sandboxes are ready and moves them to the ready pool
func (s *Service) checkPendingSandboxes(ctx context.Context) error {
	pendingSandboxes, err := s.poolStore.GetPendingSandboxes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending sandboxes: %v", err)
	}

	s.logger.Debug("Checking pending sandboxes", zap.Int("count", len(pendingSandboxes)))

	for _, sandboxID := range pendingSandboxes {
		ready, err := s.schedulerService.IsSandboxReady(ctx, sandboxID)
		if err != nil {
			s.logger.Warn("Error checking if sandbox is ready",
				sandboxID.ZapField(), zap.Error(err))
			continue
		}

		if ready {
			// Sandbox is ready, move it to the ready pool
			if err := s.poolStore.RemovePendingSandbox(ctx, sandboxID); err != nil {
				s.logger.Error("Failed to remove sandbox from pending",
					sandboxID.ZapField(), zap.Error(err))
				continue
			}

			if err := s.poolStore.AddReadySandbox(ctx, sandboxID); err != nil {
				s.logger.Error("Failed to add sandbox to ready pool",
					sandboxID.ZapField(), zap.Error(err))
				continue
			}

			s.logger.Info("Moved sandbox from pending to ready pool", sandboxID.ZapField())
		}
	}

	return nil
}

// createPoolSandbox creates a new sandbox for the pool
func (s *Service) createPoolSandbox(ctx context.Context) error {
	// Generate a unique key for this pool sandbox
	idempotenceKey := fmt.Sprintf("pool:%d", time.Now().UnixNano())

	// Schedule a new sandbox
	sandboxID, err := s.schedulerService.ScheduleSandbox(ctx, idempotenceKey)
	if err != nil {
		return fmt.Errorf("failed to schedule sandbox: %v", err)
	}

	// Add to pending sandboxes
	if err := s.poolStore.AddPendingSandbox(ctx, sandboxID); err != nil {
		return fmt.Errorf("failed to add to pending pool: %v", err)
	}

	s.logger.Info("Created new sandbox for pool", sandboxID.ZapField())
	return nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
