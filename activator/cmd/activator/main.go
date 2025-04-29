package main

import (
	"github.com/williamhogman/k8s-sched-demo/activator/internal/client"
	"github.com/williamhogman/k8s-sched-demo/activator/internal/config"
	"github.com/williamhogman/k8s-sched-demo/activator/internal/logger"
	"github.com/williamhogman/k8s-sched-demo/activator/internal/service"
	"github.com/williamhogman/k8s-sched-demo/activator/internal/transport"
	"go.uber.org/fx"
)

var Everything = fx.Options(
	config.Module,
	logger.Module,
	client.Module,
	transport.Module,
	service.Module,
)

func main() {
	app := fx.New(Everything)
	app.Run()
}
