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

	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/gen/will/scheduler/v1/schedulerv1connect"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
)

var (
	port       = flag.Int("port", 50052, "The server port")
	kubeconfig = flag.String("kubeconfig", "", "Path to kubeconfig file (for testing outside the cluster)")
	mockMode   = flag.Bool("mock", true, "Use mock K8s client for testing")
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
	podName, err := s.schedulerService.ScheduleSandbox(
		ctx,
		req.Msg.SandboxId,
		req.Msg.Namespace,
		req.Msg.Configuration,
	)
	if err != nil {
		log.Printf("Failed to schedule sandbox %s: %v", req.Msg.SandboxId, err)
		resp := &schedulerv1.ScheduleResponse{
			Status:       schedulerv1.ScheduleStatus_SCHEDULE_STATUS_FAILED,
			ErrorMessage: err.Error(),
		}
		return connect.NewResponse(resp), nil
	}

	// Return success response
	resp := &schedulerv1.ScheduleResponse{
		Status:       schedulerv1.ScheduleStatus_SCHEDULE_STATUS_SCHEDULED,
		ResourceName: podName,
	}

	// Build the Connect response
	return connect.NewResponse(resp), nil
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

	// Create the scheduler service
	schedulerService := service.NewSchedulerService(k8sClient)

	// Create the server
	server := &SchedulerServer{
		schedulerService: schedulerService,
	}

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
