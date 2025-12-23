package config

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
	"github.com/Parz1val02/OM_module/metrics"
)

// GenerateDockerPrometheusConfig generates Prometheus configuration for Docker environment
func GenerateDockerPrometheusConfig(orchestrator *metrics.RealCollectorOrchestrator, topology *discovery.NetworkTopology, inDocker bool) error {
	configPath := "../prometheus/configs/prometheus.yml"
	if inDocker {
		configPath = "/etc/prometheus/configs/prometheus.yml"
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil && inDocker {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	var config strings.Builder

	// Global configuration
	config.WriteString("global:\n")
	config.WriteString("  scrape_interval: 5s\n")
	config.WriteString("  evaluation_interval: 5s\n")
	config.WriteString("  external_labels:\n")
	if inDocker {
		config.WriteString("    monitor: 'om-module-docker'\n")
	} else {
		config.WriteString("    monitor: 'om-module-standalone'\n")
	}
	if topology != nil {
		config.WriteString(fmt.Sprintf("    deployment_type: '%s'\n", topology.Type))
	}
	config.WriteString("\n")

	// Rule files
	config.WriteString("rule_files:\n")
	config.WriteString("  - 'rules/*.yml'\n")
	config.WriteString("\n")

	// Scrape configurations
	config.WriteString("scrape_configs:\n")

	// Real Open5GS metrics
	endpoints := orchestrator.GetMetricsEndpoints()
	if len(endpoints) > 0 {
		config.WriteString("  # Real Open5GS Network Function Endpoints\n")
		for componentName, endpoint := range endpoints {
			// Extract port from endpoint (e.g., http://localhost:9091/metrics -> 9091)
			port := extractPortFromEndpoint(endpoint)

			// Use Docker service name or localhost based on environment
			target := fmt.Sprintf("localhost:%s", port)
			if inDocker {
				target = fmt.Sprintf("om-module:%s", port)
			}

			config.WriteString(fmt.Sprintf("  - job_name: '%s-real'\n", componentName))
			config.WriteString("    scrape_interval: 5s\n")
			config.WriteString("    metrics_path: '/metrics'\n")
			config.WriteString("    static_configs:\n")
			config.WriteString(fmt.Sprintf("      - targets: ['%s']\n", target))
			config.WriteString("        labels:\n")
			config.WriteString(fmt.Sprintf("          component: '%s'\n", componentName))
			config.WriteString(fmt.Sprintf("          source: 'real_open5gs'\n"))
			if topology != nil {
				config.WriteString(fmt.Sprintf("          deployment: '%s'\n", topology.Type))
			}
			config.WriteString("\n")
		}
	}

	// Container metrics
	containerTarget := "localhost:8080"
	if inDocker {
		containerTarget = "om-module:8080"
	}

	config.WriteString("  # Container Resource Metrics\n")
	config.WriteString("  - job_name: 'container-metrics'\n")
	config.WriteString("    scrape_interval: 10s\n")
	config.WriteString("    metrics_path: '/container/metrics'\n")
	config.WriteString("    static_configs:\n")
	config.WriteString(fmt.Sprintf("      - targets: ['%s']\n", containerTarget))
	config.WriteString("        labels:\n")
	config.WriteString("          source: 'container_stats'\n")
	if topology != nil {
		config.WriteString(fmt.Sprintf("          deployment: '%s'\n", topology.Type))
	}
	config.WriteString("\n")

	// Health check metrics
	healthTarget := "localhost:8081"
	if inDocker {
		healthTarget = "om-module:8081"
	}

	config.WriteString("  # Component Health Checks\n")
	config.WriteString("  - job_name: 'health-checks'\n")
	config.WriteString("    scrape_interval: 15s\n")
	config.WriteString("    metrics_path: '/health/metrics'\n")
	config.WriteString("    static_configs:\n")
	config.WriteString(fmt.Sprintf("      - targets: ['%s']\n", healthTarget))
	config.WriteString("        labels:\n")
	config.WriteString("          source: 'health_check'\n")
	if topology != nil {
		config.WriteString(fmt.Sprintf("          deployment: '%s'\n", topology.Type))
	}
	config.WriteString("\n")

	// Write configuration
	if err := os.WriteFile(configPath, []byte(config.String()), 0644); err != nil {
		return fmt.Errorf("failed to write Prometheus config: %w", err)
	}

	log.Printf("📄 Generated Prometheus configuration: %s", configPath)

	// Reload Prometheus if running in Docker
	if inDocker {
		go ReloadPrometheus()
	}

	return nil
}

// extractPortFromEndpoint extracts port number from endpoint URL
func extractPortFromEndpoint(endpoint string) string {
	// Parse URL like "http://localhost:9091/metrics" -> "9091"
	parts := strings.Split(endpoint, ":")
	if len(parts) >= 3 {
		portPart := parts[2]
		return strings.Split(portPart, "/")[0]
	}
	return "9091" // default
}

// ReloadPrometheus reloads Prometheus configuration
func ReloadPrometheus() {
	time.Sleep(5 * time.Second) // Give Prometheus time to start and config to be written

	// Try to reload Prometheus configuration via Docker network
	metricsIP := os.Getenv("METRICS_IP")
	if metricsIP == "" {
		metricsIP = "prometheus" // Use Docker service name as fallback
	}

	reloadURL := fmt.Sprintf("http://%s:9090/-/reload", metricsIP)
	log.Printf("🔄 Attempting to reload Prometheus config at: %s", reloadURL)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(reloadURL, "", nil)
	if err != nil {
		log.Printf("⚠️ Failed to reload Prometheus config: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("✅ Prometheus configuration reloaded successfully")
	} else {
		log.Printf("⚠️ Prometheus reload returned status: %d", resp.StatusCode)
	}
}
