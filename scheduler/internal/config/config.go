package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
	"go.uber.org/fx"
)

// Config holds all configuration for the scheduler service in a flat structure
type Config struct {
	// Server settings
	Port    int `envconfig:"PORT" default:"50051"`
	XDSPort int `envconfig:"XDS_PORT" default:"50052"` // Dedicated port for XDS server

	// Kubernetes settings
	MockMode     bool   `envconfig:"MOCK" default:"false"`
	Namespace    string `envconfig:"NAMESPACE" default:"sandbox"`
	UseGvisor    bool   `envconfig:"USE_GVISOR" default:"false"`                  // Whether to enforce gVisor runtime for sandboxes
	SandboxImage string `envconfig:"SANDBOX_IMAGE" default:"mock-sandbox:latest"` // The sandbox image to use

	// Sandbox settings
	CleanupIntervalSecs int           `envconfig:"CLEANUP_INTERVAL_SECS" default:"60"`
	CleanupBatchSize    int           `envconfig:"CLEANUP_BATCH_SIZE" default:"50"`
	SandboxTTL          time.Duration `envconfig:"SANDBOX_TTL" default:"15m"`

	// Sandbox pool settings
	PoolEnabled             bool          `envconfig:"POOL_ENABLED" default:"true"`             // Whether to use the sandbox pool
	PoolSize                int           `envconfig:"POOL_SIZE" default:"5"`                   // Desired number of sandboxes in the pool
	PoolRefreshIntervalSecs int           `envconfig:"POOL_REFRESH_INTERVAL_SECS" default:"15"` // How often to check and replenish the pool
	PoolMaxPending          int           `envconfig:"POOL_MAX_PENDING" default:"10"`           // Maximum number of pending sandboxes
	PoolSandboxTimeout      time.Duration `envconfig:"POOL_SANDBOX_TIMEOUT" default:"5m"`       // How long to wait for a sandbox to be ready

	// Idempotence store settings
	RedisURI string `envconfig:"REDIS_URI" default:"redis://localhost:6379/0"`

	// Logging settings
	DevelopmentLogging bool `envconfig:"DEVELOPMENT_LOGGING" default:"false"` // Whether to use development logger (more verbose)
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to process environment config: %w", err)
	}

	return &cfg, nil
}

// Module provides the config dependency to the fx container
var Module = fx.Options(
	fx.Provide(LoadConfig),
)
