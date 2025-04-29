package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
	"go.uber.org/fx"
)

// Config holds all configuration for the activator service
type Config struct {
	// Server settings
	Server ServerConfig `envconfig:"SERVER"`

	// Scheduler settings
	Scheduler SchedulerConfig `envconfig:"SCHEDULER"`

	// Logging settings
	Logging LoggingConfig `envconfig:"LOGGING"`

	// HTTP client settings
	HTTPClient HTTPClientConfig `envconfig:"HTTP_CLIENT"`
}

// ServerConfig contains server-specific configuration
type ServerConfig struct {
	Port int `envconfig:"PORT" default:"8080"`
}

// SchedulerConfig contains scheduler client configuration
type SchedulerConfig struct {
	Address string        `envconfig:"ADDRESS" default:"localhost:50051"`
	Timeout time.Duration `envconfig:"TIMEOUT" default:"10s"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Development bool `envconfig:"DEV" default:"false"` // Whether to use development logger (more verbose)
}

// HTTPClientConfig contains HTTP client configuration
type HTTPClientConfig struct {
	Timeout        time.Duration `envconfig:"TIMEOUT" default:"30s"`
	MaxRetries     int           `envconfig:"MAX_RETRIES" default:"3"`
	InitialBackoff time.Duration `envconfig:"INITIAL_BACKOFF" default:"500ms"`
	MaxBackoff     time.Duration `envconfig:"MAX_BACKOFF" default:"5s"`
	BackoffFactor  float64       `envconfig:"BACKOFF_FACTOR" default:"2.0"`
}

// Module provides configuration to the fx container
var Module = fx.Options(
	fx.Provide(LoadConfig),
)

// LoadConfig loads configuration from environment variables using envconfig
func LoadConfig() (*Config, error) {
	var config Config

	// Process environment variables with "ACTIVATOR" prefix
	if err := envconfig.Process("ACTIVATOR", &config); err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	return &config, nil
}

// PrintConfig prints the current configuration for debugging
func PrintConfig(config *Config) {
	fmt.Println("Activator Configuration:")
	fmt.Println("------------------------")
	fmt.Printf("Server Port: %d\n", config.Server.Port)
	fmt.Printf("Scheduler Address: %s\n", config.Scheduler.Address)
	fmt.Printf("Scheduler Timeout: %s\n", config.Scheduler.Timeout)
	fmt.Printf("Development Mode: %t\n", config.Logging.Development)
	fmt.Printf("HTTP Client Timeout: %s\n", config.HTTPClient.Timeout)
	fmt.Printf("HTTP Client Max Retries: %d\n", config.HTTPClient.MaxRetries)
	fmt.Printf("HTTP Client Initial Backoff: %s\n", config.HTTPClient.InitialBackoff)
	fmt.Printf("HTTP Client Max Backoff: %s\n", config.HTTPClient.MaxBackoff)
	fmt.Printf("HTTP Client Backoff Factor: %f\n", config.HTTPClient.BackoffFactor)
	fmt.Println("------------------------")
}
