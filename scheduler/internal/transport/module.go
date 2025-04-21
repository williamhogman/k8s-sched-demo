package transport

import (
	"go.uber.org/fx"

	transporthttp "github.com/williamhogman/k8s-sched-demo/scheduler/internal/transport/http"
)

// Module exports all transport modules
var Module = fx.Options(
	transporthttp.Module,
)
