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

// LoadConfig loads configuration from environment variables and command line flags
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: 50051,
		},
		Kubernetes: KubernetesConfig{
			MockMode:     false, // Default to real Kubernetes mode
			Namespace:    "sandbox",
			UseGvisor:    false,
			SandboxImage: "mock-sandbox:latest",
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
			Development: false,
		},
	}

	// Parse command-line flags
	flag.IntVar(&cfg.Server.Port, "port", cfg.Server.Port, "The server port")

	flag.StringVar(&cfg.Kubernetes.Kubeconfig, "kubeconfig", cfg.Kubernetes.Kubeconfig, "Path to kubeconfig file")
	flag.BoolVar(&cfg.Kubernetes.MockMode, "mock", cfg.Kubernetes.MockMode, "Use mock K8s client")
	flag.StringVar(&cfg.Kubernetes.Namespace, "namespace", cfg.Kubernetes.Namespace, "Kubernetes namespace")
	flag.BoolVar(&cfg.Kubernetes.UseGvisor, "use-gvisor", cfg.Kubernetes.UseGvisor, "Use gVisor runtime")
	flag.StringVar(&cfg.Kubernetes.SandboxImage, "sandbox-image", cfg.Kubernetes.SandboxImage, "Sandbox image to use")

	flag.IntVar(&cfg.Sandbox.CleanupIntervalSecs, "cleanup-interval", cfg.Sandbox.CleanupIntervalSecs, "Cleanup interval in seconds")
	flag.IntVar(&cfg.Sandbox.CleanupBatchSize, "cleanup-batch-size", cfg.Sandbox.CleanupBatchSize, "Cleanup batch size")
	flag.DurationVar(&cfg.Sandbox.TTL, "sandbox-ttl", cfg.Sandbox.TTL, "Sandbox TTL")

	flag.StringVar(&cfg.Idempotence.Type, "idempotence", cfg.Idempotence.Type, "Idempotence store type")
	flag.StringVar(&cfg.Idempotence.RedisURI, "redis-uri", cfg.Idempotence.RedisURI, "Redis URI")
	flag.DurationVar(&cfg.Idempotence.TTL, "idempotence-ttl", cfg.Idempotence.TTL, "Idempotence TTL")

	flag.BoolVar(&cfg.Logging.Development, "dev-logging", cfg.Logging.Development, "Use development logging")

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

	if env := os.Getenv("SCHEDULER_SANDBOX_IMAGE"); env != "" {
		cfg.Kubernetes.SandboxImage = env
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

	// Parse flags after setting environment variables to allow flags to override env vars
	flag.Parse()

	return cfg, nil
}

// Module provides the config dependency to the fx container
var Module = fx.Options(
	fx.Provide(LoadConfig),
)
