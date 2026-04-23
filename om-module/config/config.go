package config

import "os"

// Config holds all runtime configuration for the O&M module.
type Config struct {
	// Port the HTTP server listens on (default: 8080)
	Port string

	// DockerSocket is the path to the Docker daemon socket.
	DockerSocket string

	// ComposeProject is the Docker Compose project name used to filter
	// containers that belong to the testbed (default: docker_open5gs)
	ComposeProject string

	// TempoEndpoint is the OTLP/HTTP base URL for Grafana Tempo.
	// The tracing package POSTs to <TempoEndpoint>/v1/traces.
	// Default: "tempo:4318"
	TempoEndpoint string

	// CaptureEnabled controls whether the live packet capture pipeline
	// is started. Set to "false" to disable without redeployment.
	// Default: "true"
	CaptureEnabled bool

	// CaptureInterface is the Linux bridge interface to capture on.
	// Set to "auto" for dynamic discovery via Docker network inspection.
	// Set to an explicit name (e.g. "br-abc123") to bypass discovery.
	// Default: "auto"
	CaptureInterface string

	// MCC and MNC are used to reconstruct full 5G IMSI values from the
	// SUCI MSIN extracted from NGAP Registration Request packets.
	// These should match the values in .env.
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
