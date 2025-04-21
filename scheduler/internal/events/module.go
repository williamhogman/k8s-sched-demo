package events

import (
	"net/http"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// ProvideBroadcaster creates an event broadcaster with the given dependencies
func ProvideBroadcaster(
	cfg *config.Config,
	logger *zap.Logger,
) BroadcasterInterface {
	return NewBroadcaster(cfg, logger)
}

// EventHandlerResult represents a service handler and its path
type EventHandlerResult struct {
	Path    string
	Handler http.Handler
}

// Module provides the event broadcaster and service dependencies to the fx container
var Module = fx.Options(
	fx.Provide(ProvideBroadcaster),
)
