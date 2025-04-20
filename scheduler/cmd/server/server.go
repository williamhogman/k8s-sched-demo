package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/gen/will/scheduler/v1/schedulerv1connect"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/service"
)

// SchedulerServer implements the ConnectRPC SandboxScheduler service
type SchedulerServer struct {
	schedulerService *service.SchedulerService
}

// NewSchedulerServer creates a new instance of SchedulerServer
func NewSchedulerServer(schedulerService *service.SchedulerService) *SchedulerServer {
	return &SchedulerServer{
		schedulerService: schedulerService,
	}
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

// StartServer starts the HTTP server with Connect API handlers
func StartServer(cfg *config.Config, server *SchedulerServer) error {
	// Set up the HTTP routes with Connect handlers
	mux := http.NewServeMux()
	path, handler := schedulerv1connect.NewSandboxSchedulerHandler(server)
	mux.Handle(path, handler)

	// Start the server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Starting Scheduler server with Connect API on %s", addr)
	return http.ListenAndServe(
		addr,
		// Use h2c so we can serve HTTP/2 without TLS
		h2c.NewHandler(mux, &http2.Server{}),
	)
}
