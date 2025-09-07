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

// UpdatedMetricsOrchestrator coordinates all types of metrics collection
type UpdatedMetricsOrchestrator struct {
	discoveryService *discovery.AutoDiscoveryService
	registry         *MetricsRegistry
	configPath       string
	lastTopology     *discovery.NetworkTopology

	// Collectors
	containerCollector *ContainerMetricsCollector
	healthCollector    *HealthCheckCollector
	collectorManager   *CollectorManager

	// Configuration
	containerMetricsPort int
	healthCheckPort      int
}

// NewUpdatedMetricsOrchestrator creates a new comprehensive metrics orchestrator
func NewUpdatedMetricsOrchestrator(discoveryService *discovery.AutoDiscoveryService, configPath string) (*UpdatedMetricsOrchestrator, error) {
	// Initialize container metrics collector
	containerCollector, err := NewContainerMetricsCollector(8080)
	if err != nil {
		return nil, fmt.Errorf("failed to create container metrics collector: %w", err)
	}

	// Initialize health check collector
	healthCollector := NewHealthCheckCollector(8081)

	// Initialize official collector manager
	collectorManager := NewCollectorManager()

	return &UpdatedMetricsOrchestrator{
		discoveryService: discoveryService,
		registry: &MetricsRegistry{
			Targets: make(map[string]*MetricTarget),
		},
		configPath:           configPath,
		containerCollector:   containerCollector,
		healthCollector:      healthCollector,
		collectorManager:     collectorManager,
		containerMetricsPort: 8080,
		healthCheckPort:      8081,
	}, nil
}

// Start begins comprehensive metrics orchestration
func (umo *UpdatedMetricsOrchestrator) Start(ctx context.Context) error {
	log.Printf("🚀 Starting Updated Metrics Orchestrator with Official Collectors...")

	// Initial discovery and configuration
	if err := umo.updateMetricsConfiguration(ctx); err != nil {
		return fmt.Errorf("failed initial metrics configuration: %w", err)
	}

	// Start container metrics collector
	go func() {
		if err := umo.containerCollector.Start(ctx, umo.lastTopology); err != nil && err != context.Canceled {
			log.Printf("❌ Container metrics collector error: %v", err)
		}
	}()

	// Start health check collector
	go func() {
		if err := umo.healthCollector.Start(ctx, umo.lastTopology); err != nil && err != context.Canceled {
			log.Printf("❌ Health check collector error: %v", err)
		}
	}()

	// Start periodic monitoring for topology changes
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("🛑 Stopping Updated Metrics Orchestrator...")

			// Stop all collectors gracefully
			if err := umo.containerCollector.Close(); err != nil {
				log.Printf("⚠️ Failed to stop container collector: %v", err)
			}

			umo.collectorManager.Stop()

			return ctx.Err()
		case <-ticker.C:
			if err := umo.updateMetricsConfiguration(ctx); err != nil {
				log.Printf("⚠️ Failed to update metrics configuration: %v", err)
			}
		}
	}
}

// updateMetricsConfiguration discovers topology and updates all metrics configurations
func (umo *UpdatedMetricsOrchestrator) updateMetricsConfiguration(ctx context.Context) error {
	// Discover current topology
	topology, err := umo.discoveryService.DiscoverTopology(ctx)
	if err != nil {
		return fmt.Errorf("failed to discover topology: %w", err)
	}

	// Check if topology has changed
	if umo.hasTopologyChanged(topology) {
		log.Printf("📊 Topology change detected, updating comprehensive metrics configuration...")

		// Clear existing registry
		umo.registry.Targets = make(map[string]*MetricTarget)

		// Update official collectors
		if err := umo.collectorManager.UpdateTopology(ctx, topology); err != nil {
			log.Printf("⚠️ Failed to update official collectors: %v", err)
		}

		// Register all metric sources
		umo.registerOfficialEndpoints()
		umo.registerContainerMetrics(topology)
		umo.registerHealthChecks(topology)

		// Generate and apply Prometheus configuration
		if err := umo.generatePrometheusConfig(); err != nil {
			return fmt.Errorf("failed to generate Prometheus config: %w", err)
		}

		// Update legacy collectors with new topology
		umo.containerCollector.UpdateTopology(topology)
		umo.healthCollector.UpdateTopology(topology)

		umo.lastTopology = topology

		// Log comprehensive summary
		umo.logComprehensiveMetricsSummary()
	}

	return nil
}

// hasTopologyChanged checks if the topology has changed since last update
func (umo *UpdatedMetricsOrchestrator) hasTopologyChanged(current *discovery.NetworkTopology) bool {
	if umo.lastTopology == nil {
		return true
	}

	// Simple comparison - check if component count or running status changed
	if len(current.Components) != len(umo.lastTopology.Components) {
		return true
	}

	for name, component := range current.Components {
		if lastComponent, exists := umo.lastTopology.Components[name]; !exists {
			return true
		} else if component.IsRunning != lastComponent.IsRunning {
			return true
		}
	}

	return false
}

// registerOfficialEndpoints registers targets from the collector manager
func (umo *UpdatedMetricsOrchestrator) registerOfficialEndpoints() {
	targets := umo.collectorManager.GetMetricsTargets()

	for _, target := range targets {
		umo.registry.Targets[target.JobName] = &target
	}

	log.Printf("📋 Registered %d official endpoint targets", len(targets))
}

// registerContainerMetrics registers container-level metrics for all components
func (umo *UpdatedMetricsOrchestrator) registerContainerMetrics(topology *discovery.NetworkTopology) {
	for name, component := range topology.Components {
		if !component.IsRunning {
			continue
		}

		target := &MetricTarget{
			JobName:     fmt.Sprintf("%s-container", name),
			Target:      fmt.Sprintf("host.docker.internal:%d", umo.containerMetricsPort),
			Source:      SOURCE_CONTAINER_STATS,
			ScrapeePath: "/container/metrics",
			Interval:    "10s",
			ComponentID: name,
			Labels: map[string]string{
				"component":      name,
				"component_type": component.Type,
				"source":         string(SOURCE_CONTAINER_STATS),
				"deployment":     string(topology.Type),
				"container_id":   component.Name,
			},
		}
		umo.registry.Targets[target.JobName] = target
	}
}

// registerHealthChecks registers health check metrics for all components
func (umo *UpdatedMetricsOrchestrator) registerHealthChecks(topology *discovery.NetworkTopology) {
	for name, component := range topology.Components {
		if !component.IsRunning {
			continue
		}

		target := &MetricTarget{
			JobName:     fmt.Sprintf("%s-health", name),
			Target:      fmt.Sprintf("host.docker.internal:%d", umo.healthCheckPort),
			Source:      SOURCE_HEALTH_CHECK,
			ScrapeePath: "/health/metrics",
			Interval:    "15s",
			ComponentID: name,
			Labels: map[string]string{
				"component":      name,
				"component_type": component.Type,
				"source":         string(SOURCE_HEALTH_CHECK),
				"deployment":     string(topology.Type),
			},
		}
		umo.registry.Targets[target.JobName] = target
	}
}

// generatePrometheusConfig generates a comprehensive Prometheus configuration file
func (umo *UpdatedMetricsOrchestrator) generatePrometheusConfig() error {
	configContent := umo.buildPrometheusConfigContent()

	if err := os.WriteFile(umo.configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write Prometheus config: %w", err)
	}

	log.Printf("📄 Generated Prometheus configuration with %d targets", len(umo.registry.Targets))
	return nil
}

// buildPrometheusConfigContent creates the Prometheus configuration content
func (umo *UpdatedMetricsOrchestrator) buildPrometheusConfigContent() string {
	var config strings.Builder

	// Global configuration
	config.WriteString("global:\n")
	config.WriteString("  scrape_interval: 5s\n")
	config.WriteString("  evaluation_interval: 5s\n")
	config.WriteString("  external_labels:\n")
	config.WriteString("    monitor: 'om-module-comprehensive'\n")
	if umo.lastTopology != nil {
		config.WriteString(fmt.Sprintf("    deployment_type: '%s'\n", umo.lastTopology.Type))
	}
	config.WriteString("\n")

	// Rule files
	config.WriteString("rule_files:\n")
	config.WriteString("  - 'rules/*.yml'\n")
	config.WriteString("\n")

	// Scrape configurations
	config.WriteString("scrape_configs:\n")

	// Group targets by source type for better organization
	officialTargets := make([]*MetricTarget, 0)
	containerTargets := make([]*MetricTarget, 0)
	healthTargets := make([]*MetricTarget, 0)

	for _, target := range umo.registry.Targets {
		switch target.Source {
		case SOURCE_OFFICIAL_ENDPOINT:
			officialTargets = append(officialTargets, target)
		case SOURCE_CONTAINER_STATS:
			containerTargets = append(containerTargets, target)
		case SOURCE_HEALTH_CHECK:
			healthTargets = append(healthTargets, target)
		}
	}

	// Official endpoints section
	if len(officialTargets) > 0 {
		config.WriteString("  # Official Network Function Endpoints\n")
		for _, target := range officialTargets {
			config.WriteString(fmt.Sprintf("  - job_name: '%s'\n", target.JobName))
			config.WriteString(fmt.Sprintf("    scrape_interval: %s\n", target.Interval))
			config.WriteString(fmt.Sprintf("    metrics_path: '%s'\n", target.ScrapeePath))
			config.WriteString("    static_configs:\n")
			config.WriteString(fmt.Sprintf("      - targets: ['%s']\n", target.Target))
			config.WriteString("        labels:\n")
			for key, value := range target.Labels {
				config.WriteString(fmt.Sprintf("          %s: '%s'\n", key, value))
			}
			config.WriteString("\n")
		}
	}

	// Container metrics section
	if len(containerTargets) > 0 {
		config.WriteString("  # Container Resource Metrics\n")
		for _, target := range containerTargets {
			config.WriteString(fmt.Sprintf("  - job_name: '%s'\n", target.JobName))
			config.WriteString(fmt.Sprintf("    scrape_interval: %s\n", target.Interval))
			config.WriteString(fmt.Sprintf("    metrics_path: '%s'\n", target.ScrapeePath))
			config.WriteString("    static_configs:\n")
			config.WriteString(fmt.Sprintf("      - targets: ['%s']\n", target.Target))
			config.WriteString("        labels:\n")
			for key, value := range target.Labels {
				config.WriteString(fmt.Sprintf("          %s: '%s'\n", key, value))
			}
			config.WriteString("\n")
		}
	}

	// Health check section
	if len(healthTargets) > 0 {
		config.WriteString("  # Component Health Checks\n")
		for _, target := range healthTargets {
			config.WriteString(fmt.Sprintf("  - job_name: '%s'\n", target.JobName))
			config.WriteString(fmt.Sprintf("    scrape_interval: %s\n", target.Interval))
			config.WriteString(fmt.Sprintf("    metrics_path: '%s'\n", target.ScrapeePath))
			config.WriteString("    static_configs:\n")
			config.WriteString(fmt.Sprintf("      - targets: ['%s']\n", target.Target))
			config.WriteString("        labels:\n")
			for key, value := range target.Labels {
				config.WriteString(fmt.Sprintf("          %s: '%s'\n", key, value))
			}
			config.WriteString("\n")
		}
	}

	return config.String()
}

// logComprehensiveMetricsSummary logs a detailed summary of all metrics sources
func (umo *UpdatedMetricsOrchestrator) logComprehensiveMetricsSummary() {
	log.Printf("📊 =================================")
	log.Printf("📊 COMPREHENSIVE METRICS SUMMARY")
	log.Printf("📊 =================================")

	if umo.lastTopology != nil {
		log.Printf("📊 Deployment Type: %s", umo.lastTopology.Type)
		log.Printf("📊 Total Components: %d", len(umo.lastTopology.Components))
	}

	// Count targets by source
	officialCount := 0
	containerCount := 0
	healthCount := 0

	for _, target := range umo.registry.Targets {
		switch target.Source {
		case SOURCE_OFFICIAL_ENDPOINT:
			officialCount++
		case SOURCE_CONTAINER_STATS:
			containerCount++
		case SOURCE_HEALTH_CHECK:
			healthCount++
		}
	}

	log.Printf("📊 Official Endpoints: %d collectors", officialCount)
	log.Printf("📊 Container Metrics: %d targets", containerCount)
	log.Printf("📊 Health Checks: %d targets", healthCount)
	log.Printf("📊 Total Targets: %d", len(umo.registry.Targets))

	// Show collector details
	collectorInfo := umo.collectorManager.GetCollectorInfo()
	if len(collectorInfo) > 0 {
		log.Printf("📊 Running Official Collectors:")
		for componentName, info := range collectorInfo {
			log.Printf("📊   - %s (%s) on port %d", componentName, info.NFType, info.Port)
		}
	}

	// Validation
	issues := umo.collectorManager.ValidateConfiguration()
	if len(issues) > 0 {
		log.Printf("⚠️  Configuration Issues:")
		for _, issue := range issues {
			log.Printf("⚠️    - %s", issue)
		}
	} else {
		log.Printf("✅ Configuration validation passed")
	}

	log.Printf("📊 =================================")
}

// GetMetricsRegistry returns the current metrics registry
func (umo *UpdatedMetricsOrchestrator) GetMetricsRegistry() *MetricsRegistry {
	return umo.registry
}

// GetCollectorInfo returns information about all collectors
func (umo *UpdatedMetricsOrchestrator) GetCollectorInfo() map[string]any {
	info := map[string]any{
		"orchestrator":  "Updated Metrics Orchestrator v2.0",
		"last_update":   time.Now().Unix(),
		"total_targets": len(umo.registry.Targets),
		"collectors": map[string]any{
			"official": umo.collectorManager.GetCollectorInfo(),
			"container": map[string]any{
				"port":     umo.containerMetricsPort,
				"endpoint": fmt.Sprintf("http://localhost:%d/container/metrics", umo.containerMetricsPort),
				"status":   "running",
			},
			"health": map[string]any{
				"port":     umo.healthCheckPort,
				"endpoint": fmt.Sprintf("http://localhost:%d/health/metrics", umo.healthCheckPort),
				"status":   "running",
			},
		},
		"educational": umo.collectorManager.GetEducationalInfo(),
	}

	if umo.lastTopology != nil {
		info["topology"] = map[string]any{
			"type":       string(umo.lastTopology.Type),
			"components": len(umo.lastTopology.Components),
		}
	}

	return info
}

// GetPrometheusTargets returns formatted Prometheus targets for external use
func (umo *UpdatedMetricsOrchestrator) GetPrometheusTargets() []map[string]any {
	var targets []map[string]any

	for _, target := range umo.registry.Targets {
		targets = append(targets, map[string]any{
			"job_name":    target.JobName,
			"target":      target.Target,
			"scrape_path": target.ScrapeePath,
			"interval":    target.Interval,
			"source":      string(target.Source),
			"labels":      target.Labels,
		})
	}

	return targets
}
