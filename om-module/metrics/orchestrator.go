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

// SourceType represents the type of metrics source
type SourceType string

// ModernMetricsOrchestrator coordinates all types of metrics collection with real Open5GS support
type ModernMetricsOrchestrator struct {
	discoveryService *discovery.AutoDiscoveryService
	registry         *MetricsRegistry
	configPath       string
	lastTopology     *discovery.NetworkTopology

	// NEW: Real metrics orchestrator
	realOrchestrator *RealCollectorOrchestrator

	// Existing collectors
	containerCollector *ContainerMetricsCollector
	healthCollector    *HealthCheckCollector

	// Configuration
	containerMetricsPort int
	healthCheckPort      int
}

// NewModernMetricsOrchestrator creates a new comprehensive metrics orchestrator with real Open5GS support
func NewModernMetricsOrchestrator(discoveryService *discovery.AutoDiscoveryService, configPath string) (*ModernMetricsOrchestrator, error) {
	// Initialize container metrics collector
	containerCollector, err := NewContainerMetricsCollector(8080)
	if err != nil {
		return nil, fmt.Errorf("failed to create container metrics collector: %w", err)
	}

	// Initialize health check collector
	healthCollector := NewHealthCheckCollector(8081)

	// NEW: Initialize real Open5GS orchestrator
	realOrchestrator := NewRealCollectorOrchestrator(discoveryService)

	return &ModernMetricsOrchestrator{
		discoveryService: discoveryService,
		registry: &MetricsRegistry{
			Targets: make(map[string]*MetricTarget),
		},
		configPath:           configPath,
		realOrchestrator:     realOrchestrator,
		containerCollector:   containerCollector,
		healthCollector:      healthCollector,
		containerMetricsPort: 8080,
		healthCheckPort:      8081,
	}, nil
}

// Start begins comprehensive metrics orchestration with real Open5GS support
func (mmo *ModernMetricsOrchestrator) Start(ctx context.Context) error {
	log.Printf("🚀 Starting Modern Metrics Orchestrator with Real Open5GS Integration...")

	// Start real Open5GS metrics collection
	go func() {
		if err := mmo.realOrchestrator.Start(); err != nil {
			log.Printf("❌ Real Open5GS orchestrator error: %v", err)
		}
	}()

	// Wait for real collectors to be healthy
	go func() {
		time.Sleep(5 * time.Second) // Give it time to start
		if err := mmo.realOrchestrator.WaitForHealthy(30 * time.Second); err != nil {
			log.Printf("⚠️  Real collectors health warning: %v", err)
		}
	}()

	// Initial discovery and configuration
	if err := mmo.updateMetricsConfiguration(ctx); err != nil {
		return fmt.Errorf("failed initial metrics configuration: %w", err)
	}

	// Start container metrics collector
	go func() {
		if err := mmo.containerCollector.Start(ctx, mmo.lastTopology); err != nil && err != context.Canceled {
			log.Printf("❌ Container metrics collector error: %v", err)
		}
	}()

	// Start health check collector
	go func() {
		if err := mmo.healthCollector.Start(ctx, mmo.lastTopology); err != nil && err != context.Canceled {
			log.Printf("❌ Health check collector error: %v", err)
		}
	}()

	// Start periodic monitoring for topology changes
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("🛑 Stopping Modern Metrics Orchestrator...")

			// Stop real orchestrator
			mmo.realOrchestrator.Stop()

			// Stop other collectors gracefully
			if err := mmo.containerCollector.Close(); err != nil {
				log.Printf("⚠️ Failed to stop container collector: %v", err)
			}

			return ctx.Err()
		case <-ticker.C:
			if err := mmo.updateMetricsConfiguration(ctx); err != nil {
				log.Printf("⚠️ Failed to update metrics configuration: %v", err)
			}
		}
	}
}

// updateMetricsConfiguration discovers topology and updates all metrics configurations
func (mmo *ModernMetricsOrchestrator) updateMetricsConfiguration(ctx context.Context) error {
	// Discover current topology
	topology, err := mmo.discoveryService.DiscoverTopology(ctx)
	if err != nil {
		return fmt.Errorf("failed to discover topology: %w", err)
	}

	// Check if topology has changed
	if mmo.hasTopologyChanged(topology) {
		log.Printf("📊 Topology change detected, updating comprehensive metrics configuration...")

		// Clear existing registry
		mmo.registry.Targets = make(map[string]*MetricTarget)

		// Register all metric sources
		mmo.registerRealOpen5GSEndpoints()
		mmo.registerContainerMetrics(topology)
		mmo.registerHealthChecks(topology)

		// Generate and apply Prometheus configuration
		if err := mmo.generatePrometheusConfig(); err != nil {
			return fmt.Errorf("failed to generate Prometheus config: %w", err)
		}

		// Update legacy collectors with new topology
		mmo.containerCollector.UpdateTopology(topology)
		mmo.healthCollector.UpdateTopology(topology)

		mmo.lastTopology = topology

		// Log comprehensive summary
		mmo.logComprehensiveMetricsSummary()
	}

	return nil
}

// hasTopologyChanged checks if the topology has changed since last update
func (mmo *ModernMetricsOrchestrator) hasTopologyChanged(current *discovery.NetworkTopology) bool {
	if mmo.lastTopology == nil {
		return true
	}

	// Simple comparison - check if component count or running status changed
	if len(current.Components) != len(mmo.lastTopology.Components) {
		return true
	}

	for name, component := range current.Components {
		if lastComponent, exists := mmo.lastTopology.Components[name]; !exists {
			return true
		} else if component.IsRunning != lastComponent.IsRunning {
			return true
		}
	}

	return false
}

// NEW: Register real Open5GS endpoints
func (mmo *ModernMetricsOrchestrator) registerRealOpen5GSEndpoints() {
	// Get endpoints from real orchestrator
	endpoints := mmo.realOrchestrator.GetMetricsEndpoints()
	status := mmo.realOrchestrator.GetStatus()

	var targets []map[string]any
	if statusData, ok := status["targets"].([]map[string]any); ok {
		targets = statusData
	}

	for _, target := range targets {
		labels := make(map[string]string)
		if targetLabels, ok := target["labels"].(map[string]string); ok {
			labels = targetLabels
		}

		jobName := fmt.Sprintf("%v", target["job_name"])
		targetURL := fmt.Sprintf("%v", target["targets"].([]string)[0])

		metricTarget := &MetricTarget{
			JobName:     jobName,
			Target:      targetURL,
			Source:      SOURCE_REAL_OPEN5GS,
			ScrapeePath: "/metrics",
			Interval:    "5s",
			ComponentID: labels["component"],
			Labels:      labels,
		}
		mmo.registry.Targets[metricTarget.JobName] = metricTarget
	}
	log.Printf("Available endpoints: %v\n", endpoints)
	log.Printf("📋 Registered %d real Open5GS endpoint targets", len(targets))
}

// registerContainerMetrics registers container-level metrics for all components
func (mmo *ModernMetricsOrchestrator) registerContainerMetrics(topology *discovery.NetworkTopology) {
	for name, component := range topology.Components {
		if !component.IsRunning {
			continue
		}

		target := &MetricTarget{
			JobName:     fmt.Sprintf("%s-container", name),
			Target:      fmt.Sprintf("host.docker.internal:%d", mmo.containerMetricsPort),
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
		mmo.registry.Targets[target.JobName] = target
	}
}

// registerHealthChecks registers health check metrics for all components
func (mmo *ModernMetricsOrchestrator) registerHealthChecks(topology *discovery.NetworkTopology) {
	for name, component := range topology.Components {
		if !component.IsRunning {
			continue
		}

		target := &MetricTarget{
			JobName:     fmt.Sprintf("%s-health", name),
			Target:      fmt.Sprintf("host.docker.internal:%d", mmo.healthCheckPort),
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
		mmo.registry.Targets[target.JobName] = target
	}
}

// generatePrometheusConfig generates a comprehensive Prometheus configuration file
func (mmo *ModernMetricsOrchestrator) generatePrometheusConfig() error {
	configContent := mmo.buildPrometheusConfigContent()

	if err := os.WriteFile(mmo.configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write Prometheus config: %w", err)
	}

	log.Printf("📄 Generated Prometheus configuration with %d targets", len(mmo.registry.Targets))
	return nil
}

// buildPrometheusConfigContent creates the Prometheus configuration content
func (mmo *ModernMetricsOrchestrator) buildPrometheusConfigContent() string {
	var config strings.Builder

	// Global configuration
	config.WriteString("global:\n")
	config.WriteString("  scrape_interval: 5s\n")
	config.WriteString("  evaluation_interval: 5s\n")
	config.WriteString("  external_labels:\n")
	config.WriteString("    monitor: 'om-module-modern'\n")
	if mmo.lastTopology != nil {
		config.WriteString(fmt.Sprintf("    deployment_type: '%s'\n", mmo.lastTopology.Type))
	}
	config.WriteString("\n")

	// Rule files
	config.WriteString("rule_files:\n")
	config.WriteString("  - 'rules/*.yaml'\n")
	config.WriteString("\n")

	// Scrape configurations
	config.WriteString("scrape_configs:\n")

	// Group targets by source type for better organization
	realTargets := make([]*MetricTarget, 0)
	containerTargets := make([]*MetricTarget, 0)
	healthTargets := make([]*MetricTarget, 0)

	for _, target := range mmo.registry.Targets {
		switch target.Source {
		case SOURCE_REAL_OPEN5GS:
			realTargets = append(realTargets, target)
		case SOURCE_CONTAINER_STATS:
			containerTargets = append(containerTargets, target)
		case SOURCE_HEALTH_CHECK:
			healthTargets = append(healthTargets, target)
		}
	}

	// Real Open5GS endpoints section
	if len(realTargets) > 0 {
		config.WriteString("  # Real Open5GS Network Function Endpoints\n")
		for _, target := range realTargets {
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
func (mmo *ModernMetricsOrchestrator) logComprehensiveMetricsSummary() {
	log.Printf("📊 =================================")
	log.Printf("📊 MODERN METRICS SUMMARY")
	log.Printf("📊 =================================")

	if mmo.lastTopology != nil {
		log.Printf("📊 Deployment Type: %s", mmo.lastTopology.Type)
		log.Printf("📊 Total Components: %d", len(mmo.lastTopology.Components))
	}

	// Count targets by source
	realCount := 0
	containerCount := 0
	healthCount := 0

	for _, target := range mmo.registry.Targets {
		switch target.Source {
		case SOURCE_REAL_OPEN5GS:
			realCount++
		case SOURCE_CONTAINER_STATS:
			containerCount++
		case SOURCE_HEALTH_CHECK:
			healthCount++
		}
	}

	log.Printf("📊 Real Open5GS Endpoints: %d collectors", realCount)
	log.Printf("📊 Container Metrics: %d targets", containerCount)
	log.Printf("📊 Health Checks: %d targets", healthCount)
	log.Printf("📊 Total Targets: %d", len(mmo.registry.Targets))

	// Show real collector details
	endpoints := mmo.realOrchestrator.GetMetricsEndpoints()
	if len(endpoints) > 0 {
		log.Printf("📊 Running Real Open5GS Collectors:")
		for componentName, endpoint := range endpoints {
			log.Printf("📊   - %s: %s", componentName, endpoint)
		}
	}

	log.Printf("✅ Real Open5GS metrics integration active")
	log.Printf("📊 =================================")
}

// GetMetricsRegistry returns the current metrics registry
func (mmo *ModernMetricsOrchestrator) GetMetricsRegistry() *MetricsRegistry {
	return mmo.registry
}

// GetCollectorInfo returns information about all collectors
func (mmo *ModernMetricsOrchestrator) GetCollectorInfo() map[string]any {
	info := map[string]any{
		"orchestrator":  "Modern Metrics Orchestrator v3.0 with Real Open5GS",
		"last_update":   time.Now().Unix(),
		"total_targets": len(mmo.registry.Targets),
		"collectors": map[string]any{
			"real_open5gs": mmo.realOrchestrator.GetStatus(),
			"container": map[string]any{
				"port":     mmo.containerMetricsPort,
				"endpoint": fmt.Sprintf("http://localhost:%d/container/metrics", mmo.containerMetricsPort),
				"status":   "running",
			},
			"health": map[string]any{
				"port":     mmo.healthCheckPort,
				"endpoint": fmt.Sprintf("http://localhost:%d/health/metrics", mmo.healthCheckPort),
				"status":   "running",
			},
		},
	}

	if mmo.lastTopology != nil {
		info["topology"] = map[string]any{
			"type":       string(mmo.lastTopology.Type),
			"components": len(mmo.lastTopology.Components),
		}
	}

	return info
}

// GetPrometheusTargets returns formatted Prometheus targets for external use
func (mmo *ModernMetricsOrchestrator) GetPrometheusTargets() []map[string]any {
	var targets []map[string]any

	for _, target := range mmo.registry.Targets {
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

// GetRealMetricsEndpoints returns real Open5GS metrics endpoints
func (mmo *ModernMetricsOrchestrator) GetRealMetricsEndpoints() map[string]string {
	return mmo.realOrchestrator.GetMetricsEndpoints()
}
