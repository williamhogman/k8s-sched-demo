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
	log.Println("Request headers:", req.Header())

	// Convert from the new proto definition type to the old one that the service expects
	clusterSelectorReq := &selectorv1.GetSandboxRequest{
		SandboxId:     req.Msg.SandboxId,
		Configuration: req.Msg.Configuration,
	}

	// Call the selector service using the old type
	clusterSelectorResp, err := s.selectorService.GetSandbox(ctx, clusterSelectorReq)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("service error: %v", err))
	}

	// Convert back from old response type to new response type
	resp := &selectorv1.GetSandboxResponse{
		Status:          clusterSelectorResp.Status,
		ErrorMessage:    clusterSelectorResp.ErrorMessage,
		SandboxId:       clusterSelectorResp.SandboxId,
		ResourceName:    clusterSelectorResp.ResourceName,
		ClusterId:       clusterSelectorResp.ClusterId,
		ClusterEndpoint: clusterSelectorResp.ClusterEndpoint,
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
	log.Printf("Starting Global Scheduler server with Connect API on %s", addr)
	if err := http.ListenAndServe(
		addr,
		// Use h2c so we can serve HTTP/2 without TLS
		h2c.NewHandler(mux, &http2.Server{}),
	); err != nil {
		log.Fatalf("failed to serve: %v", err)
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
			log.Printf("Failed to add sample cluster %s: %v", cluster.ClusterID, err)
		} else {
			log.Printf("Added sample cluster: %s (Priority: %d, Weight: %d)",
				cluster.ClusterID, cluster.Priority, cluster.Weight)
		}
	}
}
