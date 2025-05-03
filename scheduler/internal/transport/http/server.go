package http

import (
	"fmt"
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/williamhogman/k8s-sched-demo/gen/will/scheduler/v1/schedulerv1connect"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type serverParams struct {
	fx.In

	Config        *config.Config
	ProjectServer *ProjectServer
	Logger        *zap.Logger
}

// StartServer starts the HTTP server with Connect API handlers
func StartServer(p serverParams) error {
	// Set up the HTTP routes with Connect handlers
	mux := http.NewServeMux()
	path, handler := schedulerv1connect.NewProjectServiceHandler(p.ProjectServer)

	mux.Handle(path, handler)

	// Start the server
	addr := fmt.Sprintf(":%d", p.Config.Port)
	p.Logger.Info("Starting Project server with Connect API",
		zap.String("address", addr),
		zap.Int("port", p.Config.Port))

	return http.ListenAndServe(
		addr,
		// Use h2c so we can serve HTTP/2 without TLS
		h2c.NewHandler(mux, &http2.Server{}),
	)
}
