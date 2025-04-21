package http

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"go.uber.org/zap"
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
	logger           *zap.Logger
}

// NewSchedulerServer creates a new instance of SchedulerServer
func NewSchedulerServer(schedulerService *service.SchedulerService, logger *zap.Logger) *SchedulerServer {
	return &SchedulerServer{
		schedulerService: schedulerService,
		logger:           logger.Named("scheduler-server"),
	}
}

// ScheduleSandbox implements the ScheduleSandbox method from the SandboxScheduler service
func (s *SchedulerServer) ScheduleSandbox(
	ctx context.Context,
	req *connect.Request[schedulerv1.ScheduleRequest],
) (*connect.Response[schedulerv1.ScheduleResponse], error) {
	s.logger.Debug("Request received", zap.Any("headers", req.Header()))

	// Call the scheduler service
	podName, success, err := s.schedulerService.ScheduleSandbox(
		ctx,
		req.Msg.IdempotenceKey,
		req.Msg.Metadata,
	)

	// Prepare response
	resp := &schedulerv1.ScheduleResponse{
		Success: success,
	}

	if err != nil {
		s.logger.Error("Failed to schedule sandbox",
			zap.String("idempotenceKey", req.Msg.IdempotenceKey),
			zap.Error(err))
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
	s.logger.Debug("Request received", zap.Any("headers", req.Header()))

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
		s.logger.Error("Failed to release sandbox",
			zap.String("sandboxId", req.Msg.SandboxId),
			zap.Error(err))
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
	s.logger.Debug("Request received", zap.Any("headers", req.Header()))

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
		s.logger.Error("Failed to retain sandbox",
			zap.String("sandboxId", req.Msg.SandboxId),
			zap.Error(err))
		resp.Error = err.Error()
	} else {
		resp.ExpirationTime = expirationTime.Unix()
	}

	// Build the Connect response
	return connect.NewResponse(resp), nil
}

// StartServer starts the HTTP server with Connect API handlers
func StartServer(cfg *config.Config, server *SchedulerServer, logger *zap.Logger) error {
	// Set up the HTTP routes with Connect handlers
	mux := http.NewServeMux()
	path, handler := schedulerv1connect.NewSandboxSchedulerHandler(server)
	mux.Handle(path, handler)

	// Start the server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("Starting Scheduler server with Connect API",
		zap.String("address", addr),
		zap.Int("port", cfg.Server.Port))

	return http.ListenAndServe(
		addr,
		// Use h2c so we can serve HTTP/2 without TLS
		h2c.NewHandler(mux, &http2.Server{}),
	)
}
