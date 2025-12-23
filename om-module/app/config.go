package app

import "os"

// Config holds application configuration
type Config struct {
	// Mode specifies the operation mode (orchestrator or discovery)
	Mode string

	// EnvFile is the path to the environment file
	EnvFile string

	// DebugMode enables detailed debug output
	DebugMode bool

	// InDocker indicates if running in Docker environment
	InDocker bool
}

// NewConfig creates a new configuration from command line arguments
func NewConfig(args []string) *Config {
	cfg := &Config{
		Mode:      "discovery",
		EnvFile:   "../.env",
		DebugMode: false,
		InDocker:  isRunningInDocker(),
	}

	// Adjust env file path for Docker
	if cfg.InDocker {
		cfg.EnvFile = ".env"
	}

	// Parse command line arguments
	if len(args) > 1 {
		cfg.Mode = args[1]
	}
	if len(args) > 2 {
		if args[2] == "--debug" {
			cfg.DebugMode = true
		} else {
			cfg.EnvFile = args[2]
		}
	}

	return cfg
}

// isRunningInDocker detects if running in Docker environment
func isRunningInDocker() bool {
	// Check for /.dockerenv file
	_, err := os.Stat("/.dockerenv")
	return err == nil
}
