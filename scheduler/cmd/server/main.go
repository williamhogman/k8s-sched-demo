package main

import (
	"log"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	persistence "github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
)

func main() {
	// Parse command line flags
	cfg := ParseFlags()

	// Create the Kubernetes client
	var k8sClient service.K8sClientInterface

	if cfg.MockMode {
		log.Println("Using mock Kubernetes client")
		k8sClient = k8sclient.NewMockK8sClient()
	} else {
		log.Println("Using real Kubernetes client")
		realClient, err := k8sclient.NewK8sClient()
		if err != nil {
			log.Fatalf("Failed to create Kubernetes client: %v", err)
		}
		k8sClient = realClient
	}

	// Create the idempotence store
	log.Printf("Using '%s' idempotence store with TTL=%v", cfg.IdempotenceType, cfg.IdempotenceTTL)
	idempotenceStore, err := persistence.NewStore(persistence.Config{
		Type:     cfg.IdempotenceType,
		RedisURI: cfg.RedisURI,
	})

	if err != nil {
		log.Fatalf("Failed to create idempotence store: %v", err)
	}

	// Ensure idempotence store is closed properly
	defer func() {
		if err := idempotenceStore.Close(); err != nil {
			log.Printf("Error closing idempotence store: %v", err)
		}
	}()

	// Create the scheduler service
	schedulerService := service.NewSchedulerService(
		k8sClient,
		idempotenceStore,
		service.SchedulerServiceConfig{
			IdempotenceKeyTTL: cfg.IdempotenceTTL,
			SandboxTTL:        cfg.SandboxTTL,
		},
	)

	// Create the server
	server := NewSchedulerServer(schedulerService)

	// Start the cleanup job in the background
	cleanupManager := NewCleanupManager(
		schedulerService,
		cfg.CleanupIntervalSecs,
		cfg.CleanupBatchSize,
	)
	cleanupManager.Start()
	defer cleanupManager.Stop()

	// Start the HTTP server
	if err := StartServer(cfg, server); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
