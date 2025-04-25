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
	fmt.Println("------------------------")
}
