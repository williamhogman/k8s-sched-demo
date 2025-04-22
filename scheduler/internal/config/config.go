package config

import (
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/fx"
)

// Config holds all configuration for the scheduler service
type Config struct {
	// Server settings
	Server ServerConfig

	// Kubernetes settings
	Kubernetes KubernetesConfig

	// Sandbox settings
	Sandbox SandboxConfig

	// Idempotence store settings
	Idempotence IdempotenceConfig

	// Logging settings
	Logging LoggingConfig

	// Event Broadcaster settings
	EventBroadcaster EventBroadcasterConfig
}

// ServerConfig contains server-specific configuration
type ServerConfig struct {
	Port int
}

// KubernetesConfig contains Kubernetes client configuration
type KubernetesConfig struct {
	Kubeconfig   string
	MockMode     bool
	Namespace    string
	UseGvisor    bool   // Whether to enforce gVisor runtime for sandboxes
	SandboxImage string // The sandbox image to use
}

// SandboxConfig contains sandbox management configuration
type SandboxConfig struct {
	CleanupIntervalSecs int
	CleanupBatchSize    int
	TTL                 time.Duration
}

// IdempotenceConfig contains configuration for the idempotence store
type IdempotenceConfig struct {
	Type     string
	RedisURI string
	TTL      time.Duration
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Development bool // Whether to use development logger (more verbose)
}

// EventBroadcasterConfig contains configuration for the event broadcaster
type EventBroadcasterConfig struct {
	Enabled    bool          // Whether to enable event broadcasting
	Endpoint   string        // Endpoint to send events to (host:port)
	MaxRetries int           // Maximum number of retries for sending events
	RetryDelay time.Duration // Delay between retries
	BufferSize int           // Size of the event buffer channel
}

// LoadConfig loads configuration from environment variables and command line flags
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: 50052,
		},
		Kubernetes: KubernetesConfig{
			MockMode:  true,
			Namespace: "sandbox", // Default namespace
			UseGvisor: false,     // Default to not using gVisor
		},
		Sandbox: SandboxConfig{
			CleanupIntervalSecs: 60,
			CleanupBatchSize:    50,
			TTL:                 15 * time.Minute,
		},
		Idempotence: IdempotenceConfig{
			Type:     "memory",
			RedisURI: "redis://localhost:6379/0",
			TTL:      1 * time.Hour,
		},
		Logging: LoggingConfig{
			Development: false, // Default to production logging
		},
		EventBroadcaster: EventBroadcasterConfig{
			Enabled:    true,              // Disabled by default
			Endpoint:   "localhost:50053", // Default endpoint
			MaxRetries: 3,
			RetryDelay: 500 * time.Millisecond,
			BufferSize: 100,
		},
	}

	// Parse command-line flags
	flag.IntVar(&cfg.Server.Port, "port", cfg.Server.Port, "The server port")

	flag.StringVar(&cfg.Kubernetes.Kubeconfig, "kubeconfig", cfg.Kubernetes.Kubeconfig, "Path to kubeconfig file (for testing outside the cluster)")
	flag.BoolVar(&cfg.Kubernetes.MockMode, "mock", cfg.Kubernetes.MockMode, "Use mock K8s client for testing")
	flag.StringVar(&cfg.Kubernetes.Namespace, "namespace", cfg.Kubernetes.Namespace, "Kubernetes namespace to use for sandboxes")
	flag.BoolVar(&cfg.Kubernetes.UseGvisor, "use-gvisor", cfg.Kubernetes.UseGvisor, "Use gVisor runtime for isolation")
	flag.StringVar(&cfg.Kubernetes.SandboxImage, "sandbox-image", cfg.Kubernetes.SandboxImage, "Docker image to use for sandboxes")

	flag.IntVar(&cfg.Sandbox.CleanupIntervalSecs, "cleanup-interval", cfg.Sandbox.CleanupIntervalSecs, "Interval in seconds between expired sandbox cleanup runs")
	flag.IntVar(&cfg.Sandbox.CleanupBatchSize, "cleanup-batch-size", cfg.Sandbox.CleanupBatchSize, "Maximum number of expired sandboxes to clean up in each batch")
	flag.DurationVar(&cfg.Sandbox.TTL, "sandbox-ttl", cfg.Sandbox.TTL, "Default TTL for sandboxes")

	flag.StringVar(&cfg.Idempotence.Type, "idempotence", cfg.Idempotence.Type, "Idempotence store type: 'memory' or 'redis'")
	flag.StringVar(&cfg.Idempotence.RedisURI, "redis-uri", cfg.Idempotence.RedisURI, "Redis connection URI for idempotence store")
	flag.DurationVar(&cfg.Idempotence.TTL, "idempotence-ttl", cfg.Idempotence.TTL, "TTL for idempotence keys")

	flag.BoolVar(&cfg.Logging.Development, "dev-logging", cfg.Logging.Development, "Use development logging mode (more verbose)")

	flag.BoolVar(&cfg.EventBroadcaster.Enabled, "event-broadcast", cfg.EventBroadcaster.Enabled, "Enable event broadcasting")
	flag.StringVar(&cfg.EventBroadcaster.Endpoint, "event-endpoint", cfg.EventBroadcaster.Endpoint, "Endpoint for broadcasting events")
	flag.IntVar(&cfg.EventBroadcaster.MaxRetries, "event-max-retries", cfg.EventBroadcaster.MaxRetries, "Maximum retries for sending events")
	flag.DurationVar(&cfg.EventBroadcaster.RetryDelay, "event-retry-delay", cfg.EventBroadcaster.RetryDelay, "Delay between retries for sending events")
	flag.IntVar(&cfg.EventBroadcaster.BufferSize, "event-buffer-size", cfg.EventBroadcaster.BufferSize, "Size of the event buffer channel")

	// Override with environment variables if present
	if env := os.Getenv("SCHEDULER_PORT"); env != "" {
		var port int
		if _, err := fmt.Sscanf(env, "%d", &port); err == nil {
			cfg.Server.Port = port
		}
	}

	if env := os.Getenv("SCHEDULER_MOCK"); env != "" {
		cfg.Kubernetes.MockMode = env == "true" || env == "1"
	}

	if env := os.Getenv("SCHEDULER_KUBECONFIG"); env != "" {
		cfg.Kubernetes.Kubeconfig = env
	}

	if env := os.Getenv("SCHEDULER_NAMESPACE"); env != "" {
		cfg.Kubernetes.Namespace = env
	}

	if env := os.Getenv("SCHEDULER_USE_GVISOR"); env != "" {
		cfg.Kubernetes.UseGvisor = env == "true" || env == "1"
	}

	if env := os.Getenv("SCHEDULER_CLEANUP_INTERVAL"); env != "" {
		var interval int
		if _, err := fmt.Sscanf(env, "%d", &interval); err == nil {
			cfg.Sandbox.CleanupIntervalSecs = interval
		}
	}

	if env := os.Getenv("SCHEDULER_CLEANUP_BATCH_SIZE"); env != "" {
		var batchSize int
		if _, err := fmt.Sscanf(env, "%d", &batchSize); err == nil {
			cfg.Sandbox.CleanupBatchSize = batchSize
		}
	}

	if env := os.Getenv("SCHEDULER_IDEMPOTENCE_TYPE"); env != "" {
		cfg.Idempotence.Type = env
	}

	if env := os.Getenv("SCHEDULER_REDIS_URI"); env != "" {
		cfg.Idempotence.RedisURI = env
	}

	if env := os.Getenv("SCHEDULER_DEV_LOGGING"); env != "" {
		cfg.Logging.Development = env == "true" || env == "1"
	}

	if env := os.Getenv("SCHEDULER_EVENT_BROADCAST"); env != "" {
		cfg.EventBroadcaster.Enabled = env == "true" || env == "1"
	}

	if env := os.Getenv("SCHEDULER_EVENT_ENDPOINT"); env != "" {
		cfg.EventBroadcaster.Endpoint = env
	}

	if env := os.Getenv("SCHEDULER_EVENT_MAX_RETRIES"); env != "" {
		var maxRetries int
		if _, err := fmt.Sscanf(env, "%d", &maxRetries); err == nil {
			cfg.EventBroadcaster.MaxRetries = maxRetries
		}
	}

	if env := os.Getenv("SCHEDULER_EVENT_RETRY_DELAY"); env != "" {
		var retryDelay time.Duration
		if _, err := fmt.Sscanf(env, "%s", &retryDelay); err == nil {
			cfg.EventBroadcaster.RetryDelay = retryDelay
		}
	}

	if env := os.Getenv("SCHEDULER_EVENT_BUFFER_SIZE"); env != "" {
		var bufferSize int
		if _, err := fmt.Sscanf(env, "%d", &bufferSize); err == nil {
			cfg.EventBroadcaster.BufferSize = bufferSize
		}
	}

	// Parse flags after setting environment variables to allow flags to override env vars
	flag.Parse()

	return cfg, nil
}

// Module provides the config dependency to the fx container
var Module = fx.Options(
	fx.Provide(LoadConfig),
)
