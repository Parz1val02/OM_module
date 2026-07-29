package config

import "os"

// Config holds all runtime configuration for the O&M module.
type Config struct {
	Port           string // HTTP server port
	DockerSocket   string // path to the Docker daemon socket
	ComposeProject string // Compose project name used to filter testbed containers
	TempoEndpoint  string // OTLP/HTTP base URL for Grafana Tempo

	CaptureEnabled   bool   // whether the live packet capture pipeline runs
	CaptureInterface string // bridge interface to capture on, or "auto" to discover it

	// MCC and MNC reconstruct full 5G IMSI values from the SUCI MSIN in
	// NGAP Registration Request packets; must match the values in .env.
	MCC string
	MNC string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Port:             getEnv("OM_PORT", "8080"),
		DockerSocket:     getEnv("DOCKER_SOCKET", "/var/run/docker.sock"),
		ComposeProject:   getEnv("COMPOSE_PROJECT", "om_module"),
		TempoEndpoint:    getEnv("TEMPO_ENDPOINT", "tempo:4318"),
		CaptureEnabled:   getEnv("CAPTURE_ENABLED", "true") == "true",
		CaptureInterface: getEnv("CAPTURE_INTERFACE", "auto"),
		MCC:              getEnv("MCC", "001"),
		MNC:              getEnv("MNC", "01"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
