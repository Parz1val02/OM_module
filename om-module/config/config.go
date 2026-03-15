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
	// Default: "http://tempo:4318"
	TempoEndpoint string

	// LokiURL is the base URL of the Loki HTTP query API.
	// Used by the trace reconstructor to fetch NF log lines.
	// Default: "http://loki:3100"
	LokiURL string

	// TraceQueryWindow controls how far back the reconstructor searches
	// Loki for log events matching a given IMSI (default: "10m").
	TraceQueryWindow string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Port:             getEnv("OM_PORT", "8080"),
		DockerSocket:     getEnv("DOCKER_SOCKET", "/var/run/docker.sock"),
		ComposeProject:   getEnv("COMPOSE_PROJECT", "om_module"),
		TempoEndpoint:    getEnv("TEMPO_ENDPOINT", "tempo:4318"),
		LokiURL:          getEnv("LOKI_URL", "http://loki:3100"),
		TraceQueryWindow: getEnv("TRACE_QUERY_WINDOW", "60m"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
