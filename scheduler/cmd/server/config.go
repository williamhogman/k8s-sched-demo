package main

import (
	"flag"
	"time"
)

// Config holds all configuration for the scheduler server
type Config struct {
	// Server settings
	Port int

	// Kubernetes settings
	Kubeconfig string
	MockMode   bool

	// Sandbox settings
	CleanupIntervalSecs int
	CleanupBatchSize    int
	SandboxTTL          time.Duration

	// Idempotence store settings
	IdempotenceType string
	RedisURI        string
	IdempotenceTTL  time.Duration
}

// ParseFlags parses command line flags into a Config struct
func ParseFlags() *Config {
	cfg := &Config{}

	// Server settings
	flag.IntVar(&cfg.Port, "port", 50052, "The server port")

	// Kubernetes settings
	flag.StringVar(&cfg.Kubeconfig, "kubeconfig", "", "Path to kubeconfig file (for testing outside the cluster)")
	flag.BoolVar(&cfg.MockMode, "mock", true, "Use mock K8s client for testing")

	// Sandbox settings
	flag.IntVar(&cfg.CleanupIntervalSecs, "cleanup-interval", 60, "Interval in seconds between expired sandbox cleanup runs")
	flag.IntVar(&cfg.CleanupBatchSize, "cleanup-batch-size", 50, "Maximum number of expired sandboxes to clean up in each batch")
	flag.DurationVar(&cfg.SandboxTTL, "sandbox-ttl", 15*time.Minute, "Default TTL for sandboxes")

	// Idempotence store settings
	flag.StringVar(&cfg.IdempotenceType, "idempotence", "memory", "Idempotence store type: 'memory' or 'redis'")
	flag.StringVar(&cfg.RedisURI, "redis-uri", "redis://localhost:6379/0", "Redis connection URI for idempotence store")
	flag.DurationVar(&cfg.IdempotenceTTL, "idempotence-ttl", 24*time.Hour, "TTL for idempotence keys")

	flag.Parse()
	return cfg
}
