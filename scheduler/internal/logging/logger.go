package logging

import (
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// ProvideLogger creates a zap logger based on configuration
// Uses production logger by default, but can use development logger if configured
func ProvideLogger(cfg *config.Config) (*zap.Logger, error) {
	var logger *zap.Logger
	var err error

	if cfg.Logging.Development {
		cfg := zap.NewDevelopmentConfig()
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
		logger, err = cfg.Build()
	} else {
		logger, err = zap.NewProduction()
	}

	if err != nil {
		return nil, err
	}
	return logger, nil
}

// ProvideLoggerSugared creates a sugared logger from the standard zap logger
func ProvideLoggerSugared(logger *zap.Logger) *zap.SugaredLogger {
	return logger.Sugar()
}

// Module provides the logger dependencies to the fx container
var Module = fx.Options(
	fx.Provide(ProvideLogger),
	fx.Provide(ProvideLoggerSugared),
)
