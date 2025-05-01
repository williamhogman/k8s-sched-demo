package http

import (
	"fmt"
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/williamhogman/k8s-sched-demo/gen/will/scheduler/v1/schedulerv1connect"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"go.uber.org/zap"
)

// StartServer starts the HTTP server with Connect API handlers
func StartServer(cfg *config.Config, projectServer *ProjectServer, logger *zap.Logger) error {
	// Set up the HTTP routes with Connect handlers
	mux := http.NewServeMux()
	path, handler := schedulerv1connect.NewProjectServiceHandler(projectServer)
	mux.Handle(path, handler)

	// Start the server
	addr := fmt.Sprintf(":%d", cfg.Port)
	logger.Info("Starting Project server with Connect API",
		zap.String("address", addr),
		zap.Int("port", cfg.Port))

	return http.ListenAndServe(
		addr,
		// Use h2c so we can serve HTTP/2 without TLS
		h2c.NewHandler(mux, &http2.Server{}),
	)
}
