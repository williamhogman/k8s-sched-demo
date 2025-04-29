package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/williamhogman/k8s-sched-demo/activator/internal/client"
	"github.com/williamhogman/k8s-sched-demo/activator/internal/config"
	"go.uber.org/zap"
)

// ActivatorService handles HTTP traffic and forwards it to sandboxes
type ActivatorService struct {
	schedulerClient *client.SchedulerClient
	logger          *zap.Logger
	httpClient      *http.Client
	// Retry configuration
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	backoffFactor  float64
}

// NewActivatorService creates a new activator service
func NewActivatorService(schedulerClient *client.SchedulerClient, logger *zap.Logger, cfg *config.Config) *ActivatorService {
	// Create HTTP client with configured timeout
	httpClient := &http.Client{
		Timeout: cfg.HTTPClient.Timeout,
	}

	return &ActivatorService{
		schedulerClient: schedulerClient,
		logger:          logger,
		httpClient:      httpClient,
		// Use retry configuration from config
		maxRetries:     cfg.HTTPClient.MaxRetries,
		initialBackoff: cfg.HTTPClient.InitialBackoff,
		maxBackoff:     cfg.HTTPClient.MaxBackoff,
		backoffFactor:  cfg.HTTPClient.BackoffFactor,
	}
}

// isRetryableError determines if an error should trigger a retry
func (s *ActivatorService) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for DNS resolution errors
	if strings.Contains(err.Error(), "no such host") ||
		strings.Contains(err.Error(), "lookup") ||
		strings.Contains(err.Error(), "dns") {
		return true
	}

	// Check for connection errors
	if strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "connection reset") ||
		strings.Contains(err.Error(), "i/o timeout") {
		return true
	}

	// Check for temporary network errors
	if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
		return true
	}

	return false
}

// doWithRetry performs an HTTP request with retry logic
func (s *ActivatorService) doWithRetry(ctx context.Context, req *http.Request, sandboxHostname string) (*http.Response, error) {
	var resp *http.Response
	var err error

	backoff := s.initialBackoff
	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		// Log retry attempt if not the first try
		if attempt > 0 {
			s.logger.Info("retrying request to sandbox",
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff),
				zap.String("sandbox_hostname", sandboxHostname),
				zap.Error(err),
			)

			// Wait for the backoff period
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				// Continue with the next attempt
			}

			// Increase backoff for next attempt (with jitter)
			backoff = time.Duration(float64(backoff) * s.backoffFactor)
			if backoff > s.maxBackoff {
				backoff = s.maxBackoff
			}
		}

		// Try to make the request
		resp, err = s.httpClient.Do(req)

		// If successful, return the response
		if err == nil {
			return resp, nil
		}

		// Check if the error is retryable
		if !s.isRetryableError(err) {
			return nil, err
		}

		// If we've reached the maximum number of retries, return the error
		if attempt == s.maxRetries {
			s.logger.Error("max retries reached for sandbox request",
				zap.Int("max_retries", s.maxRetries),
				zap.String("sandbox_hostname", sandboxHostname),
				zap.Error(err),
			)
			return nil, fmt.Errorf("max retries reached: %w", err)
		}
	}

	// This should never be reached, but just in case
	return nil, err
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
		s.logger.Error("no project ID found",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
		)
		http.Error(w, "No project ID found", http.StatusBadRequest)
		return
	}
	// Get a sandbox from the scheduler
	sandboxHostname, err := s.schedulerClient.GetSandbox(ctx, projectID)
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
	targetURL := fmt.Sprintf("http://%s:8000%s", sandboxHostname, r.URL.Path)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Create a new request to forward to the sandbox
	req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, r.Body)
	if err != nil {
		s.logger.Error("failed to create request",
			zap.Error(err),
			zap.String("sandbox_hostname", sandboxHostname),
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

	// Forward the request to the sandbox with retry logic
	resp, err := s.doWithRetry(ctx, req, sandboxHostname)
	if err != nil {
		s.logger.Error("failed to forward request to sandbox after retries",
			zap.Error(err),
			zap.String("sandbox_hostname", sandboxHostname),
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
			zap.String("sandbox_hostname", sandboxHostname),
		)
		// At this point we've already started sending the response,
		// so we can't send an error status
		return
	}
}
