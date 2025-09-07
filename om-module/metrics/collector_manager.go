package metrics

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
)

// CollectorManager manages all official endpoint collectors
type CollectorManager struct {
	collectors       map[string]*OfficialEndpointCollector
	collectorFactory map[NFType]func(int) *OfficialEndpointCollector
	portMap          map[NFType]int
	topology         *discovery.NetworkTopology
	mu               sync.RWMutex
	running          map[string]context.CancelFunc
}

// NewCollectorManager creates a new collector manager
func NewCollectorManager() *CollectorManager {
	// Default port assignments for each NF type
	portMap := map[NFType]int{
		NF_AMF:  9091,
		NF_SMF:  9092,
		NF_PCF:  9093,
		NF_UPF:  9094,
		NF_MME:  9095,
		NF_PCRF: 9096,
	}

	return &CollectorManager{
		collectors:       make(map[string]*OfficialEndpointCollector),
		collectorFactory: GetCollectorFactory(),
		portMap:          portMap,
		running:          make(map[string]context.CancelFunc),
	}
}

// UpdateTopology updates the topology and manages collector lifecycle
func (cm *CollectorManager) UpdateTopology(ctx context.Context, topology *discovery.NetworkTopology) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.topology = topology

	// Track which collectors should be running
	requiredCollectors := make(map[string]NFType)

	// Identify required collectors based on topology
	for componentName, component := range topology.Components {
		if !component.IsRunning {
			continue
		}

		nfType := cm.detectNFType(componentName, component)
		if nfType != "" {
			requiredCollectors[componentName] = nfType
		}
	}

	// Stop collectors that are no longer needed
	for collectorID, cancelFunc := range cm.running {
		if _, required := requiredCollectors[collectorID]; !required {
			log.Printf("🛑 Stopping collector for %s", collectorID)
			cancelFunc()
			delete(cm.running, collectorID)
			delete(cm.collectors, collectorID)
		}
	}

	// Start new collectors
	for componentName, nfType := range requiredCollectors {
		if _, exists := cm.running[componentName]; !exists {
			if err := cm.startCollector(ctx, componentName, nfType); err != nil {
				log.Printf("❌ Failed to start collector for %s: %v", componentName, err)
				continue
			}
			log.Printf("🚀 Started collector for %s (%s)", componentName, nfType)
		}
	}

	return nil
}

// detectNFType determines the NF type from component name and properties
func (cm *CollectorManager) detectNFType(componentName string, component discovery.Component) NFType {
	name := strings.ToLower(componentName)

	// Direct name matching
	switch {
	case strings.Contains(name, "amf"):
		return NF_AMF
	case strings.Contains(name, "smf"):
		return NF_SMF
	case strings.Contains(name, "pcf"):
		return NF_PCF
	case strings.Contains(name, "upf"):
		return NF_UPF
	case strings.Contains(name, "mme"):
		return NF_MME
	case strings.Contains(name, "pcrf"):
		return NF_PCRF
	}

	// Fallback to component type detection
	componentType := strings.ToLower(component.Type)
	switch {
	case strings.Contains(componentType, "access") || strings.Contains(componentType, "mobility"):
		return NF_AMF
	case strings.Contains(componentType, "session"):
		return NF_SMF
	case strings.Contains(componentType, "policy"):
		if strings.Contains(componentType, "5g") {
			return NF_PCF
		}
		return NF_PCRF
	case strings.Contains(componentType, "user") || strings.Contains(componentType, "plane"):
		return NF_UPF
	case strings.Contains(componentType, "mme") || strings.Contains(componentType, "4g"):
		return NF_MME
	}

	return ""
}

// startCollector starts a new collector for the specified component
func (cm *CollectorManager) startCollector(parentCtx context.Context, componentName string, nfType NFType) error {
	_, exists := cm.portMap[nfType]
	if !exists {
		return fmt.Errorf("no port mapping for NF type %s", nfType)
	}

	// Adjust port if multiple instances of same NF type
	adjustedPort := cm.getAvailablePort(nfType, componentName)

	// Create collector
	factory := cm.collectorFactory[nfType]
	collector := factory(adjustedPort)

	// Create context for this collector
	ctx, cancel := context.WithCancel(parentCtx)

	// Start collector in goroutine
	go func() {
		if err := collector.Start(ctx, cm.topology); err != nil && err != context.Canceled {
			log.Printf("❌ Collector error for %s: %v", componentName, err)
		}
	}()

	// Store collector and cancel function
	cm.collectors[componentName] = collector
	cm.running[componentName] = cancel

	return nil
}

// getAvailablePort returns an available port for the NF type, handling multiple instances
func (cm *CollectorManager) getAvailablePort(nfType NFType, componentName string) int {
	basePort := cm.portMap[nfType]

	// Check if base port is already used by another instance
	usedPorts := make(map[int]bool)
	for _, collector := range cm.collectors {
		usedPorts[collector.port] = true
	}

	// Find next available port
	port := basePort
	for usedPorts[port] {
		port++
	}

	return port
}

// GetCollectorInfo returns information about all running collectors
func (cm *CollectorManager) GetCollectorInfo() map[string]CollectorInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	info := make(map[string]CollectorInfo)

	for componentName, collector := range cm.collectors {
		info[componentName] = CollectorInfo{
			ComponentName: componentName,
			NFType:        string(collector.nfType),
			Port:          collector.port,
			MetricsURL:    fmt.Sprintf("http://localhost:%d/metrics", collector.port),
			HealthURL:     fmt.Sprintf("http://localhost:%d/health", collector.port),
			DashboardURL:  fmt.Sprintf("http://localhost:%d/dashboard", collector.port),
			IsRunning:     true,
		}
	}

	return info
}

// CollectorInfo represents information about a running collector
type CollectorInfo struct {
	ComponentName string `json:"component_name"`
	NFType        string `json:"nf_type"`
	Port          int    `json:"port"`
	MetricsURL    string `json:"metrics_url"`
	HealthURL     string `json:"health_url"`
	DashboardURL  string `json:"dashboard_url"`
	IsRunning     bool   `json:"is_running"`
}

// Stop stops all running collectors
func (cm *CollectorManager) Stop() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	log.Printf("🛑 Stopping all official endpoint collectors...")

	for componentName, cancelFunc := range cm.running {
		log.Printf("🛑 Stopping collector for %s", componentName)
		cancelFunc()
	}

	// Wait a moment for graceful shutdown
	time.Sleep(2 * time.Second)

	// Clear maps
	cm.collectors = make(map[string]*OfficialEndpointCollector)
	cm.running = make(map[string]context.CancelFunc)

	log.Printf("✅ All collectors stopped")
}

// GetMetricsTargets returns Prometheus target configurations for all collectors
func (cm *CollectorManager) GetMetricsTargets() []MetricTarget {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var targets []MetricTarget

	for componentName, collector := range cm.collectors {
		target := MetricTarget{
			JobName:     fmt.Sprintf("%s-official", componentName),
			Target:      fmt.Sprintf("localhost:%d", collector.port),
			Source:      SOURCE_OFFICIAL_ENDPOINT,
			ScrapeePath: "/metrics",
			Interval:    "5s",
			ComponentID: componentName,
			Labels: map[string]string{
				"component":      componentName,
				"component_type": string(collector.nfType),
				"source":         string(SOURCE_OFFICIAL_ENDPOINT),
				"nf_type":        string(collector.nfType),
			},
		}

		// Add topology-specific labels if available
		if cm.topology != nil {
			if component, exists := cm.topology.Components[componentName]; exists {
				target.Labels["deployment"] = string(cm.topology.Type)
				target.Labels["component_ip"] = component.IP
			}
		}

		targets = append(targets, target)
	}

	return targets
}

// ValidateConfiguration validates the current collector configuration
func (cm *CollectorManager) ValidateConfiguration() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var issues []string

	// Check for port conflicts
	portUsage := make(map[int][]string)
	for componentName, collector := range cm.collectors {
		portUsage[collector.port] = append(portUsage[collector.port], componentName)
	}

	for port, components := range portUsage {
		if len(components) > 1 {
			issues = append(issues, fmt.Sprintf("Port conflict on %d: %v", port, components))
		}
	}

	// Check for missing expected collectors
	if cm.topology != nil {
		expectedNFs := []string{"amf", "smf", "upf"}
		for _, expectedNF := range expectedNFs {
			found := false
			for componentName := range cm.collectors {
				if strings.Contains(strings.ToLower(componentName), expectedNF) {
					found = true
					break
				}
			}
			if !found {
				issues = append(issues, fmt.Sprintf("Expected NF %s not found in collectors", expectedNF))
			}
		}
	}

	return issues
}

// GetEducationalInfo returns educational information about the managed collectors
func (cm *CollectorManager) GetEducationalInfo() map[string]any {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	info := map[string]any{
		"overview": "Official Endpoint Collectors provide standardized metrics from 4G/5G network functions",
		"architecture": map[string]any{
			"pattern":   "Each NF exposes Prometheus metrics on dedicated ports",
			"standards": []string{"3GPP TS 28.552", "Prometheus OpenMetrics", "Cloud Native Telemetry"},
			"benefits": []string{
				"Real-time performance monitoring",
				"Industry-standard KPIs",
				"Educational visualization",
				"Operational insights",
			},
		},
		"collectors":       make(map[string]any),
		"total_collectors": len(cm.collectors),
		"deployment_type":  "",
	}

	if cm.topology != nil {
		info["deployment_type"] = string(cm.topology.Type)
	}

	// Add collector-specific educational info
	for componentName, collector := range cm.collectors {
		var description string
		var keyMetrics []string

		switch collector.nfType {
		case NF_AMF:
			description = "5G Access and Mobility Management - handles UE registration, authentication, and mobility"
			keyMetrics = []string{"ue_attached_total", "handover_total", "session_setup_total"}
		case NF_SMF:
			description = "Session Management Function - manages PDU sessions and UPF coordination"
			keyMetrics = []string{"pdu_sessions_total", "pfcp_associations_total", "message_processing_duration"}
		case NF_UPF:
			description = "User Plane Function - handles data forwarding and processing"
			keyMetrics = []string{"packet_throughput_total", "data_volume_bytes_total", "active_sessions_total"}
		case NF_PCF:
			description = "Policy Control Function - provides policy decisions and charging rules"
			keyMetrics = []string{"policy_rules_active_total", "charging_events_total", "errors_total"}
		case NF_MME:
			description = "4G Mobility Management Entity - core control for LTE networks"
			keyMetrics = []string{"ue_attached_total", "bearer_setup_total", "active_sessions_total"}
		case NF_PCRF:
			description = "4G Policy and Charging Rules Function - policy control for LTE"
			keyMetrics = []string{"diameter_sessions_total", "policy_decisions_total", "message_processing_duration"}
		}

		info["collectors"].(map[string]any)[componentName] = map[string]any{
			"nf_type":     string(collector.nfType),
			"description": description,
			"port":        collector.port,
			"key_metrics": keyMetrics,
			"endpoints": map[string]string{
				"metrics":   fmt.Sprintf("http://localhost:%d/metrics", collector.port),
				"health":    fmt.Sprintf("http://localhost:%d/health", collector.port),
				"dashboard": fmt.Sprintf("http://localhost:%d/dashboard", collector.port),
			},
		}
	}

	return info
}
