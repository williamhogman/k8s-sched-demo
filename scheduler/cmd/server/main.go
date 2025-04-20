package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/gen/will/scheduler/v1/schedulerv1connect"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/idempotence"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
)

var (
	port                = flag.Int("port", 50052, "The server port")
	kubeconfig          = flag.String("kubeconfig", "", "Path to kubeconfig file (for testing outside the cluster)")
	mockMode            = flag.Bool("mock", true, "Use mock K8s client for testing")
	cleanupIntervalSecs = flag.Int("cleanup-interval", 60, "Interval in seconds between expired sandbox cleanup runs")
	batchSize           = flag.Int("cleanup-batch-size", 50, "Maximum number of expired sandboxes to clean up in each batch")
	sandboxTTL          = flag.Duration("sandbox-ttl", 15*time.Minute, "Default TTL for sandboxes")

	// Idempotence store settings
	idempotenceType = flag.String("idempotence", "memory", "Idempotence store type: 'memory' or 'redis'")
	redisURI        = flag.String("redis-uri", "redis://localhost:6379/0", "Redis connection URI for idempotence store")
	idempotenceTTL  = flag.Duration("idempotence-ttl", 24*time.Hour, "TTL for idempotence keys")
)

// SchedulerServer implements the ConnectRPC SandboxScheduler service
type SchedulerServer struct {
	schedulerService *service.SchedulerService
}

// ScheduleSandbox implements the ScheduleSandbox method from the SandboxScheduler service
func (s *SchedulerServer) ScheduleSandbox(
	ctx context.Context,
	req *connect.Request[schedulerv1.ScheduleRequest],
) (*connect.Response[schedulerv1.ScheduleResponse], error) {
	log.Println("Request headers:", req.Header())

	// Call the scheduler service
	podName, success, err := s.schedulerService.ScheduleSandbox(
		ctx,
		req.Msg.IdempotenceKey,
		req.Msg.Namespace,
		req.Msg.Metadata,
	)

	// Prepare response
	resp := &schedulerv1.ScheduleResponse{
		Success: success,
	}

	if err != nil {
		log.Printf("Failed to schedule sandbox with key %s: %v", req.Msg.IdempotenceKey, err)
		resp.Error = err.Error()
	} else {
		resp.SandboxId = podName
	}

	// Build the Connect response
	return connect.NewResponse(resp), nil
}

// ReleaseSandbox implements the ReleaseSandbox method from the SandboxScheduler service
func (s *SchedulerServer) ReleaseSandbox(
	ctx context.Context,
	req *connect.Request[schedulerv1.ReleaseSandboxRequest],
) (*connect.Response[schedulerv1.ReleaseSandboxResponse], error) {
	log.Println("Request headers:", req.Header())

	// Call the scheduler service
	success, err := s.schedulerService.ReleaseSandbox(
		ctx,
		req.Msg.SandboxId,
	)

	// Prepare response
	resp := &schedulerv1.ReleaseSandboxResponse{
		Success: success,
	}

	if err != nil {
		log.Printf("Failed to release sandbox %s: %v", req.Msg.SandboxId, err)
		resp.Error = err.Error()
	}

	// Build the Connect response
	return connect.NewResponse(resp), nil
}

// RetainSandbox implements the RetainSandbox method from the SandboxScheduler service
func (s *SchedulerServer) RetainSandbox(
	ctx context.Context,
	req *connect.Request[schedulerv1.RetainSandboxRequest],
) (*connect.Response[schedulerv1.RetainSandboxResponse], error) {
	log.Println("Request headers:", req.Header())

	// Call the scheduler service
	expirationTime, success, err := s.schedulerService.RetainSandbox(
		ctx,
		req.Msg.SandboxId,
	)

	// Prepare response
	resp := &schedulerv1.RetainSandboxResponse{
		Success: success,
	}

	if err != nil {
		log.Printf("Failed to retain sandbox %s: %v", req.Msg.SandboxId, err)
		resp.Error = err.Error()
	} else {
		resp.ExpirationTime = expirationTime.Unix()
	}

	// Build the Connect response
	return connect.NewResponse(resp), nil
}

// runCleanupJob periodically runs the cleanup job to remove expired sandboxes
func runCleanupJob(ctx context.Context, schedulerService *service.SchedulerService, intervalSecs int, batchSize int) {
	ticker := time.NewTicker(time.Duration(intervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Cleanup job shutting down")
			return
		case <-ticker.C:
			// Run the cleanup job
			count, err := schedulerService.CleanupExpiredSandboxes(ctx, batchSize)
			if err != nil {
				log.Printf("Error during sandbox cleanup: %v", err)
			} else if count > 0 {
				log.Printf("Cleanup job released %d expired sandboxes", count)
			}
		}
	}
}

func main() {
	flag.Parse()

	// Create the Kubernetes client
	var k8sClient service.K8sClientInterface

	if *mockMode {
		log.Println("Using mock Kubernetes client")
		k8sClient = k8sclient.NewMockK8sClient()
	} else {
		log.Println("Using real Kubernetes client")
		realClient, err := k8sclient.NewK8sClient()
		if err != nil {
			log.Fatalf("failed to create Kubernetes client: %v", err)
		}
		k8sClient = realClient
	}

	// Create the idempotence store
	log.Printf("Using '%s' idempotence store with TTL=%v", *idempotenceType, *idempotenceTTL)
	idempotenceStore, err := idempotence.NewStore(idempotence.Config{
		Type:     *idempotenceType,
		RedisURI: *redisURI,
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
			IdempotenceKeyTTL: *idempotenceTTL,
			SandboxTTL:        *sandboxTTL,
		},
	)

	// Create the server
	server := &SchedulerServer{
		schedulerService: schedulerService,
	}

	// Start the cleanup job in the background
	log.Printf("Starting sandbox cleanup job with interval=%ds, batch size=%d", *cleanupIntervalSecs, *batchSize)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure cleanup job stops when main function exits
	go runCleanupJob(ctx, schedulerService, *cleanupIntervalSecs, *batchSize)

	// Set up the HTTP routes with Connect handlers
	mux := http.NewServeMux()
	path, handler := schedulerv1connect.NewSandboxSchedulerHandler(server)
	mux.Handle(path, handler)

	// Start the server
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting Scheduler server with Connect API on %s", addr)
	if err := http.ListenAndServe(
		addr,
		// Use h2c so we can serve HTTP/2 without TLS
		h2c.NewHandler(mux, &http2.Server{}),
	); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
