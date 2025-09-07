package metrics

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
)

// MetricSource represents different types of metric collection sources
type MetricSource string

const (
	SOURCE_OFFICIAL_ENDPOINT MetricSource = "official_endpoint"
	SOURCE_CONTAINER_STATS   MetricSource = "container_stats"
	SOURCE_HEALTH_CHECK      MetricSource = "health_check"
)

// MetricTarget represents a target for Prometheus scraping
type MetricTarget struct {
	JobName     string            `json:"job_name"`
	Target      string            `json:"target"`
	Labels      map[string]string `json:"labels"`
	Source      MetricSource      `json:"source"`
	ScrapePath  string            `json:"scrape_path"`
	Interval    string            `json:"interval"`
	ComponentID string            `json:"component_id"`
}

// MetricsRegistry holds the registry of all available metrics
type MetricsRegistry struct {
	Targets map[string]*MetricTarget `json:"targets"`
}

// MetricsOrchestrator coordinates metrics collection across the topology
type MetricsOrchestrator struct {
	discoveryService *discovery.AutoDiscoveryService
	registry         *MetricsRegistry
	configPath       string
	lastTopology     *discovery.NetworkTopology
}

// NewMetricsOrchestrator creates a new metrics orchestrator
func NewMetricsOrchestrator(discoveryService *discovery.AutoDiscoveryService, configPath string) *MetricsOrchestrator {
	return &MetricsOrchestrator{
		discoveryService: discoveryService,
		registry: &MetricsRegistry{
			Targets: make(map[string]*MetricTarget),
		},
		configPath: configPath,
	}
}

// Start begins the metrics orchestration process
func (mo *MetricsOrchestrator) Start(ctx context.Context) error {
	log.Printf("🚀 Starting Metrics Orchestrator...")

	// Initial discovery and configuration
	if err := mo.updateMetricsConfiguration(ctx); err != nil {
		return fmt.Errorf("failed initial metrics configuration: %w", err)
	}

	// Start periodic monitoring for topology changes
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("🛑 Stopping Metrics Orchestrator...")
			return ctx.Err()
		case <-ticker.C:
			if err := mo.updateMetricsConfiguration(ctx); err != nil {
				log.Printf("⚠️  Failed to update metrics configuration: %v", err)
			}
		}
	}
}

// updateMetricsConfiguration discovers topology and updates metrics configuration
func (mo *MetricsOrchestrator) updateMetricsConfiguration(ctx context.Context) error {
	// Discover current topology
	topology, err := mo.discoveryService.DiscoverTopology(ctx)
	if err != nil {
		return fmt.Errorf("failed to discover topology: %w", err)
	}

	// Check if topology has changed
	if mo.hasTopologyChanged(topology) {
		log.Printf("📊 Topology change detected, updating metrics configuration...")

		// Clear existing registry
		mo.registry.Targets = make(map[string]*MetricTarget)

		// Register metrics from discovered components
		mo.registerOfficialEndpoints(topology)
		mo.registerContainerMetrics(topology)
		mo.registerHealthChecks(topology)

		// Generate and apply Prometheus configuration
		if err := mo.generatePrometheusConfig(); err != nil {
			return fmt.Errorf("failed to generate Prometheus config: %w", err)
		}

		mo.lastTopology = topology

		// Log summary
		mo.logMetricsSummary()
	}

	return nil
}

// hasTopologyChanged checks if the topology has changed since last update
func (mo *MetricsOrchestrator) hasTopologyChanged(current *discovery.NetworkTopology) bool {
	if mo.lastTopology == nil {
		return true
	}

	// Simple comparison - check if component count or running status changed
	if len(current.Components) != len(mo.lastTopology.Components) {
		return true
	}

	for name, component := range current.Components {
		if lastComponent, exists := mo.lastTopology.Components[name]; !exists {
			return true
		} else if component.IsRunning != lastComponent.IsRunning {
			return true
		}
	}

	return false
}

// registerOfficialEndpoints registers components with official metrics endpoints
func (mo *MetricsOrchestrator) registerOfficialEndpoints(topology *discovery.NetworkTopology) {
	// Components with official metrics endpoints
	officialEndpoints := map[string]bool{
		"amf":  true, // 5G
		"smf":  true, // 4G/5G
		"pcf":  true, // 5G
		"upf":  true, // 4G/5G
		"pcrf": true, // 4G
		"mme":  true, // 4G
	}

	for name, component := range topology.Components {
		if !component.IsRunning {
			continue
		}

		// Check if component has official metrics endpoint
		componentType := strings.ToLower(name)
		for endpoint := range officialEndpoints {
			if strings.Contains(componentType, endpoint) {
				target := &MetricTarget{
					JobName:     fmt.Sprintf("%s-official", name),
					Target:      fmt.Sprintf("%s:9091", component.IP),
					Source:      SOURCE_OFFICIAL_ENDPOINT,
					ScrapePath:  "/metrics",
					Interval:    "5s",
					ComponentID: name,
					Labels: map[string]string{
						"component":      name,
						"component_type": component.Type,
						"source":         string(SOURCE_OFFICIAL_ENDPOINT),
						"deployment":     string(topology.Type),
					},
				}
				mo.registry.Targets[target.JobName] = target
				break
			}
		}
	}
}

// registerContainerMetrics registers container-level metrics for all components
func (mo *MetricsOrchestrator) registerContainerMetrics(topology *discovery.NetworkTopology) {
	for name, component := range topology.Components {
		if !component.IsRunning {
			continue
		}

		target := &MetricTarget{
			JobName:     fmt.Sprintf("%s-container", name),
			Target:      fmt.Sprintf("%s:8080", component.IP), // Placeholder for container metrics
			Source:      SOURCE_CONTAINER_STATS,
			ScrapePath:  "/container/metrics",
			Interval:    "10s",
			ComponentID: name,
			Labels: map[string]string{
				"component":      name,
				"component_type": component.Type,
				"source":         string(SOURCE_CONTAINER_STATS),
				"deployment":     string(topology.Type),
				"container_id":   component.Name, // Using name as container identifier
			},
		}
		mo.registry.Targets[target.JobName] = target
	}
}

// registerHealthChecks registers health check metrics for all components
func (mo *MetricsOrchestrator) registerHealthChecks(topology *discovery.NetworkTopology) {
	for name, component := range topology.Components {
		if !component.IsRunning {
			continue
		}

		target := &MetricTarget{
			JobName:     fmt.Sprintf("%s-health", name),
			Target:      fmt.Sprintf("%s:8080", component.IP), // Placeholder for health checks
			Source:      SOURCE_HEALTH_CHECK,
			ScrapePath:  "/health/metrics",
			Interval:    "15s",
			ComponentID: name,
			Labels: map[string]string{
				"component":      name,
				"component_type": component.Type,
				"source":         string(SOURCE_HEALTH_CHECK),
				"deployment":     string(topology.Type),
			},
		}
		mo.registry.Targets[target.JobName] = target
	}
}

// generatePrometheusConfig generates a new Prometheus configuration file
func (mo *MetricsOrchestrator) generatePrometheusConfig() error {
	configContent := mo.buildPrometheusConfigContent()

	if err := os.WriteFile(mo.configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write Prometheus config: %w", err)
	}

	log.Printf("📝 Generated new Prometheus configuration with %d targets", len(mo.registry.Targets))
	return nil
}

// buildPrometheusConfigContent builds the Prometheus configuration content
func (mo *MetricsOrchestrator) buildPrometheusConfigContent() string {
	var builder strings.Builder

	// Global configuration
	builder.WriteString("global:\n")
	builder.WriteString("  scrape_interval: 5s\n")
	builder.WriteString("  external_labels:\n")
	builder.WriteString("    monitor: 'open5gs-om-module'\n\n")

	builder.WriteString("scrape_configs:\n")

	// Add each registered target
	for _, target := range mo.registry.Targets {
		builder.WriteString(fmt.Sprintf("  - job_name: '%s'\n", target.JobName))
		builder.WriteString(fmt.Sprintf("    scrape_interval: %s\n", target.Interval))
		builder.WriteString(fmt.Sprintf("    metrics_path: '%s'\n", target.ScrapePath))
		builder.WriteString("    static_configs:\n")
		builder.WriteString(fmt.Sprintf("      - targets: ['%s']\n", target.Target))

		if len(target.Labels) > 0 {
			builder.WriteString("        labels:\n")
			for key, value := range target.Labels {
				builder.WriteString(fmt.Sprintf("          %s: '%s'\n", key, value))
			}
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

// logMetricsSummary logs a summary of the current metrics configuration
func (mo *MetricsOrchestrator) logMetricsSummary() {
	summary := make(map[MetricSource]int)

	for _, target := range mo.registry.Targets {
		summary[target.Source]++
	}

	log.Printf("📊 Metrics Summary:")
	for source, count := range summary {
		log.Printf("   %s: %d targets", source, count)
	}
	log.Printf("   Total: %d targets", len(mo.registry.Targets))
}

// GetRegistry returns the current metrics registry
func (mo *MetricsOrchestrator) GetRegistry() *MetricsRegistry {
	return mo.registry
}

// GetTargetsBySource returns targets filtered by source type
func (mo *MetricsOrchestrator) GetTargetsBySource(source MetricSource) []*MetricTarget {
	var targets []*MetricTarget
	for _, target := range mo.registry.Targets {
		if target.Source == source {
			targets = append(targets, target)
		}
	}
	return targets
}
