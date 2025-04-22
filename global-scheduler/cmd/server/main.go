package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	selectorv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/global-scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/gen/will/global-scheduler/v1/selectorv1connect"

	"github.com/williamhogman/k8s-sched-demo/global-scheduler/internal/client"
	"github.com/williamhogman/k8s-sched-demo/global-scheduler/internal/models"
	"github.com/williamhogman/k8s-sched-demo/global-scheduler/internal/service"
	"github.com/williamhogman/k8s-sched-demo/global-scheduler/internal/storage"
)

var (
	port = flag.Int("port", 50051, "The server port")
)

// GlobalSchedulerServer implements the ConnectRPC ClusterSelector service
type GlobalSchedulerServer struct {
	selectorService *service.SelectorService
}

// GetSandbox implements the GetSandbox method from the GlobalScheduler service
func (s *GlobalSchedulerServer) GetSandbox(
	ctx context.Context,
	req *connect.Request[selectorv1.GetSandboxRequest],
) (*connect.Response[selectorv1.GetSandboxResponse], error) {
	// Pass the request directly to the service
	resp, err := s.selectorService.GetSandbox(ctx, req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("service error: %v", err))
	}

	// Build the Connect response
	connectResp := connect.NewResponse(resp)
	connectResp.Header().Set("Global-Scheduler-Version", "v1")

	return connectResp, nil
}

// ReleaseSandbox implements the ReleaseSandbox method from the GlobalScheduler service
func (s *GlobalSchedulerServer) ReleaseSandbox(
	ctx context.Context,
	req *connect.Request[selectorv1.ReleaseSandboxRequest],
) (*connect.Response[selectorv1.ReleaseSandboxResponse], error) {
	// Pass the request directly to the service
	resp, err := s.selectorService.ReleaseSandbox(ctx, req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("service error: %v", err))
	}

	// Build the Connect response
	connectResp := connect.NewResponse(resp)
	connectResp.Header().Set("Global-Scheduler-Version", "v1")

	return connectResp, nil
}

// RetainSandbox implements the RetainSandbox method from the GlobalScheduler service
func (s *GlobalSchedulerServer) RetainSandbox(
	ctx context.Context,
	req *connect.Request[selectorv1.RetainSandboxRequest],
) (*connect.Response[selectorv1.RetainSandboxResponse], error) {
	// Pass the request directly to the service
	resp, err := s.selectorService.RetainSandbox(ctx, req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("service error: %v", err))
	}

	// Build the Connect response
	connectResp := connect.NewResponse(resp)
	connectResp.Header().Set("Global-Scheduler-Version", "v1")

	return connectResp, nil
}

func main() {
	flag.Parse()

	// Create a new memory storage
	memStorage := storage.NewMemoryStorage()

	// Add some sample clusters
	addSampleClusters(memStorage)

	// Convert storage clusters to a map for the service
	clusters := make(map[string]*models.Cluster)
	for _, cluster := range memStorage.ListClusters() {
		clusters[cluster.ClusterID] = cluster
	}

	// Create a scheduler client for connecting to scheduler services
	var schedulerClient client.SchedulerClientInterface
	schedulerClient = client.NewSchedulerClient()

	// Create the selector service
	selectorService := service.NewSelectorService(clusters, schedulerClient)

	// Create the server
	server := &GlobalSchedulerServer{
		selectorService: selectorService,
	}

	// Set up the HTTP routes with Connect handlers
	mux := http.NewServeMux()
	path, handler := selectorv1connect.NewClusterSelectorHandler(server)
	mux.Handle(path, handler)

	// Start the server
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Global Scheduler server started on %s", addr)
	if err := http.ListenAndServe(
		addr,
		// Use h2c so we can serve HTTP/2 without TLS
		h2c.NewHandler(mux, &http2.Server{}),
	); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// addSampleClusters adds sample clusters for development/testing
func addSampleClusters(storage *storage.MemoryStorage) {
	clusters := []*models.Cluster{
		models.NewCluster("cluster-1", 10, 5, "us-west", "localhost:50052"),
		models.NewCluster("cluster-2", 10, 3, "us-west", "localhost:50052"),
		models.NewCluster("cluster-3", 5, 10, "us-east", "localhost:50052"),
	}

	for _, cluster := range clusters {
		err := storage.AddCluster(cluster)
		if err != nil {
			log.Printf("Failed to add cluster %s: %v", cluster.ClusterID, err)
		}
	}
}
