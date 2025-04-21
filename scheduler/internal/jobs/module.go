package jobs

import (
	"go.uber.org/fx"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/jobs/cleanup"
)

// Module exports all job modules
var Module = fx.Options(
	cleanup.Module,
)
