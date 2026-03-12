package config

import "os"

// Config holds all runtime configuration for the O&M module.
type Config struct {
	// Port the HTTP server listens on (default: 8080)
	Port string

	// DockerSocket is the path to the Docker daemon socket
	DockerSocket string

	// ComposeProject is the Docker Compose project name used to filter
	// containers that belong to the testbed (default: docker_open5gs)
	ComposeProject string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Port:           getEnv("OM_PORT", "8080"),
		DockerSocket:   getEnv("DOCKER_SOCKET", "/var/run/docker.sock"),
		ComposeProject: getEnv("COMPOSE_PROJECT", "om_module"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
