package logger

import (
	"fmt"

	"github.com/williamhogman/k8s-sched-demo/activator/internal/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// provideLogger creates a logger based on configuration
func provideLogger(cfg *config.Config) (*zap.Logger, error) {
	var logger *zap.Logger
	var err error

	if cfg.Logging.Development {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	return logger, nil
}

var Module = fx.Options(
	fx.Provide(provideLogger),
)
