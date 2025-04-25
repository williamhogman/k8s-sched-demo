package service

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/williamhogman/k8s-sched-demo/activator/internal/client"
	"go.uber.org/zap"
)

// ActivatorService handles HTTP traffic and forwards it to sandboxes
type ActivatorService struct {
	schedulerClient *client.SchedulerClient
	logger          *zap.Logger
	httpClient      *http.Client
}

// NewActivatorService creates a new activator service
func NewActivatorService(schedulerClient *client.SchedulerClient, logger *zap.Logger) *ActivatorService {
	return &ActivatorService{
		schedulerClient: schedulerClient,
		logger:          logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// HandleRequest processes an incoming HTTP request by:
// 1. Getting a sandbox from the scheduler
// 2. Forwarding the request to the sandbox
// 3. Returning the response to the client
func (s *ActivatorService) HandleRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// project id is found either in a header or in the host
	projectID := r.Header.Get("Project-Id")
	if projectID == "" {
		projectID = strings.Split(r.Host, ".")[0]
	}
	if projectID == "" {
		s.logger.Error("no project ID found",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
		)
		http.Error(w, "No project ID found", http.StatusBadRequest)
		return
	}

	// Get a sandbox from the scheduler
	sandboxID, err := s.schedulerClient.GetSandbox(ctx, projectID)
	if err != nil {
		s.logger.Error("failed to get sandbox",
			zap.Error(err),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
		)
		http.Error(w, "Failed to get sandbox", http.StatusInternalServerError)
		return
	}

	// Create the target URL for the sandbox
	targetURL := fmt.Sprintf("http://%s%s", sandboxID, r.URL.Path)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Create a new request to forward to the sandbox
	req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, r.Body)
	if err != nil {
		s.logger.Error("failed to create request",
			zap.Error(err),
			zap.String("sandbox_id", sandboxID),
		)
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Copy headers from the original request
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Forward the request to the sandbox
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error("failed to forward request to sandbox",
			zap.Error(err),
			zap.String("sandbox_id", sandboxID),
		)
		http.Error(w, "Failed to forward request", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Copy headers from the sandbox response
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set the status code
	w.WriteHeader(resp.StatusCode)

	// Copy the response body
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		s.logger.Error("failed to copy response body",
			zap.Error(err),
			zap.String("sandbox_id", sandboxID),
		)
		// At this point we've already started sending the response,
		// so we can't send an error status
		return
	}
}
