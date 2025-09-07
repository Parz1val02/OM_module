package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	HealthStatusUp       HealthStatus = "up"
	HealthStatusDown     HealthStatus = "down"
	HealthStatusUnknown  HealthStatus = "unknown"
	HealthStatusDegraded HealthStatus = "degraded"
)

// ComponentHealth represents health information for a single component
type ComponentHealth struct {
	ComponentID      string       `json:"component_id"`
	Name             string       `json:"name"`
	Status           HealthStatus `json:"status"`
	ResponseTime     float64      `json:"response_time_ms"`
	LastCheck        int64        `json:"last_check"`
	ErrorMessage     string       `json:"error_message,omitempty"`
	Endpoint         string       `json:"endpoint"`
	CheckType        string       `json:"check_type"`
	ConsecutiveFails int          `json:"consecutive_fails"`
	TotalChecks      int          `json:"total_checks"`
	SuccessRate      float64      `json:"success_rate"`
}

// HealthCheckConfig defines how to check a component's health
type HealthCheckConfig struct {
	ComponentID  string
	Name         string
	CheckType    string // "http", "tcp", "ping"
	Endpoint     string
	Timeout      time.Duration
	ExpectedCode int    // For HTTP checks
	Path         string // For HTTP checks
}

// HealthCheckCollector manages health checking for all components
type HealthCheckCollector struct {
	topology      *discovery.NetworkTopology
	healthCache   map[string]*ComponentHealth
	checkConfigs  map[string]*HealthCheckConfig
	httpClient    *http.Client
	port          int
	checkInterval time.Duration
}

// NewHealthCheckCollector creates a new health check collector
func NewHealthCheckCollector(port int) *HealthCheckCollector {
	return &HealthCheckCollector{
		healthCache:  make(map[string]*ComponentHealth),
		checkConfigs: make(map[string]*HealthCheckConfig),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		port:          port,
		checkInterval: 15 * time.Second,
	}
}

// Start starts the health check collection server
func (hcc *HealthCheckCollector) Start(ctx context.Context, topology *discovery.NetworkTopology) error {
	hcc.topology = topology

	// Generate health check configurations based on topology
	hcc.generateHealthCheckConfigs()

	// Start periodic health checking in background
	go hcc.performHealthChecksPeriodically(ctx)

	// Start HTTP server for Prometheus scraping
	mux := http.NewServeMux()
	mux.HandleFunc("/health/metrics", hcc.handleMetricsRequest)
	mux.HandleFunc("/health/status", hcc.handleHealthStatusRequest)
	mux.HandleFunc("/health", hcc.handleCollectorHealthCheck)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", hcc.port),
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		log.Printf("🏥 Health check server listening on :%d", hcc.port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ Health check server error: %v", err)
		}
	}()

	// Wait for context cancellation to shutdown
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

// generateHealthCheckConfigs creates health check configs based on discovered topology
func (hcc *HealthCheckCollector) generateHealthCheckConfigs() {
	if hcc.topology == nil {
		return
	}

	// Clear existing configs
	hcc.checkConfigs = make(map[string]*HealthCheckConfig)

	for name, component := range hcc.topology.Components {
		if !component.IsRunning {
			continue
		}

		config := hcc.createHealthCheckConfig(name, component)
		if config != nil {
			hcc.checkConfigs[name] = config
			log.Printf("📋 Configured health check for %s: %s", name, config.Endpoint)
		}
	}
}

// createHealthCheckConfig creates appropriate health check config for a component
func (hcc *HealthCheckCollector) createHealthCheckConfig(name string, component discovery.Component) *HealthCheckConfig {
	config := &HealthCheckConfig{
		ComponentID: name,
		Name:        name,
		Timeout:     5 * time.Second,
	}

	// Determine check type and endpoint based on component type
	switch {
	// Web interfaces - HTTP check
	case strings.Contains(name, "webui") || strings.Contains(name, "grafana"):
		config.CheckType = "http"
		config.Path = "/"
		config.ExpectedCode = 200
		// Try to find HTTP port
		for _, port := range component.Ports {
			if strings.Contains(port, "80") || strings.Contains(port, "30") || strings.Contains(port, "90") {
				config.Endpoint = fmt.Sprintf("http://%s", strings.Split(port, "/")[0])
				if strings.Contains(port, ":") {
					config.Endpoint = fmt.Sprintf("http://%s", port)
				} else {
					config.Endpoint = fmt.Sprintf("http://%s:%s", component.IP, strings.Split(port, "/")[0])
				}
				break
			}
		}

	// Database components - TCP check
	case strings.Contains(name, "mongo") || strings.Contains(name, "redis") || strings.Contains(name, "mysql"):
		config.CheckType = "tcp"
		// Find database port
		for _, port := range component.Ports {
			portNum := strings.Split(port, "/")[0]
			config.Endpoint = fmt.Sprintf("%s:%s", component.IP, portNum)
			break
		}

	// 5G/4G Core components - HTTP health check on common ports
	case strings.Contains(name, "amf") || strings.Contains(name, "smf") ||
		strings.Contains(name, "nrf") || strings.Contains(name, "pcf") ||
		strings.Contains(name, "mme") || strings.Contains(name, "hss"):
		config.CheckType = "http"
		config.Path = "/health"
		config.ExpectedCode = 200
		// Try common management ports first, then main service ports
		managementPorts := []string{"8080", "8090", "9090", "7777"}
		found := false

		for _, mgmtPort := range managementPorts {
			config.Endpoint = fmt.Sprintf("http://%s:%s%s", component.IP, mgmtPort, config.Path)
			found = true
			break
		}

		if !found && len(component.Ports) > 0 {
			// Fallback to first available port
			portNum := strings.Split(component.Ports[0], "/")[0]
			config.Endpoint = fmt.Sprintf("http://%s:%s%s", component.IP, portNum, config.Path)
		}

	// UPF and other data plane components - TCP check
	case strings.Contains(name, "upf") || strings.Contains(name, "sgw"):
		config.CheckType = "tcp"
		if len(component.Ports) > 0 {
			portNum := strings.Split(component.Ports[0], "/")[0]
			config.Endpoint = fmt.Sprintf("%s:%s", component.IP, portNum)
		}

	// Default case - simple TCP connectivity check
	default:
		config.CheckType = "tcp"
		if len(component.Ports) > 0 {
			portNum := strings.Split(component.Ports[0], "/")[0]
			config.Endpoint = fmt.Sprintf("%s:%s", component.IP, portNum)
		} else {
			// No ports found, skip health check
			return nil
		}
	}

	// Validate that we have an endpoint
	if config.Endpoint == "" {
		return nil
	}

	return config
}

// performHealthChecksPeriodically runs health checks at regular intervals
func (hcc *HealthCheckCollector) performHealthChecksPeriodically(ctx context.Context) {
	ticker := time.NewTicker(hcc.checkInterval)
	defer ticker.Stop()

	// Perform initial health check immediately
	hcc.performAllHealthChecks()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hcc.performAllHealthChecks()
		}
	}
}

// performAllHealthChecks executes health checks for all configured components
func (hcc *HealthCheckCollector) performAllHealthChecks() {
	for componentID, config := range hcc.checkConfigs {
		health := hcc.performHealthCheck(config)
		hcc.updateComponentHealth(componentID, health)
	}
}

// performHealthCheck executes a single health check
func (hcc *HealthCheckCollector) performHealthCheck(config *HealthCheckConfig) *ComponentHealth {
	startTime := time.Now()

	health := &ComponentHealth{
		ComponentID: config.ComponentID,
		Name:        config.Name,
		LastCheck:   startTime.Unix(),
		Endpoint:    config.Endpoint,
		CheckType:   config.CheckType,
	}

	var err error
	switch config.CheckType {
	case "http":
		err = hcc.performHTTPCheck(config)
	case "tcp":
		err = hcc.performTCPCheck(config)
	default:
		err = fmt.Errorf("unknown check type: %s", config.CheckType)
	}

	// Calculate response time
	health.ResponseTime = float64(time.Since(startTime).Nanoseconds()) / 1_000_000 // Convert to milliseconds

	// Update status based on check result
	if err != nil {
		health.Status = HealthStatusDown
		health.ErrorMessage = err.Error()
	} else {
		health.Status = HealthStatusUp
		health.ErrorMessage = ""
	}

	return health
}

// performHTTPCheck performs an HTTP health check
func (hcc *HealthCheckCollector) performHTTPCheck(config *HealthCheckConfig) error {
	url := config.Endpoint
	if config.Path != "" && !strings.HasSuffix(config.Endpoint, config.Path) {
		url = config.Endpoint + config.Path
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := hcc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		err = resp.Body.Close()
	}()

	// Check if status code is acceptable
	expectedCode := config.ExpectedCode
	if expectedCode == 0 {
		expectedCode = 200 // Default expected code
	}

	if resp.StatusCode != expectedCode {
		// Allow some tolerance for health checks
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil // 2xx is generally OK
		}
		return fmt.Errorf("unexpected status code: %d (expected %d)", resp.StatusCode, expectedCode)
	}

	return nil
}

// performTCPCheck performs a TCP connectivity check
func (hcc *HealthCheckCollector) performTCPCheck(config *HealthCheckConfig) error {
	conn, err := net.DialTimeout("tcp", config.Endpoint, config.Timeout)
	if err != nil {
		return fmt.Errorf("TCP connection failed: %w", err)
	}
	defer func() {
		err = conn.Close()
	}()
	return nil
}

// updateComponentHealth updates the health cache with new health data
func (hcc *HealthCheckCollector) updateComponentHealth(componentID string, newHealth *ComponentHealth) {
	existing, exists := hcc.healthCache[componentID]

	if exists {
		// Update counters based on previous state
		newHealth.TotalChecks = existing.TotalChecks + 1

		if newHealth.Status == HealthStatusDown {
			newHealth.ConsecutiveFails = existing.ConsecutiveFails + 1
		} else {
			newHealth.ConsecutiveFails = 0
		}

		// Calculate success rate
		if newHealth.TotalChecks > 0 {
			successfulChecks := newHealth.TotalChecks - newHealth.ConsecutiveFails
			if existing.Status != HealthStatusDown {
				successfulChecks = newHealth.TotalChecks - newHealth.ConsecutiveFails + existing.TotalChecks - existing.ConsecutiveFails
				newHealth.TotalChecks = existing.TotalChecks + 1
			}
			newHealth.SuccessRate = float64(successfulChecks) / float64(newHealth.TotalChecks) * 100
		}
	} else {
		// First check for this component
		newHealth.TotalChecks = 1
		if newHealth.Status == HealthStatusDown {
			newHealth.ConsecutiveFails = 1
			newHealth.SuccessRate = 0
		} else {
			newHealth.ConsecutiveFails = 0
			newHealth.SuccessRate = 100
		}
	}

	hcc.healthCache[componentID] = newHealth
}

// handleMetricsRequest handles Prometheus metrics endpoint requests
func (hcc *HealthCheckCollector) handleMetricsRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	metrics := hcc.generatePrometheusMetrics()

	if _, err := w.Write([]byte(metrics)); err != nil {
		log.Printf("❌ Failed to write health metrics response: %v", err)
	}
}

// handleHealthStatusRequest returns current health status as JSON
func (hcc *HealthCheckCollector) handleHealthStatusRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	healthData := maps.Clone(hcc.healthCache)

	response := map[string]any{
		"timestamp":  time.Now().Unix(),
		"components": healthData,
		"summary":    hcc.generateHealthSummary(),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("❌ Failed to encode health status: %v", err)
	}
}

// handleCollectorHealthCheck returns health of the collector itself
func (hcc *HealthCheckCollector) handleCollectorHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]any{
		"status":                 "healthy",
		"timestamp":              time.Now().Unix(),
		"components_monitored":   len(hcc.healthCache),
		"check_interval_seconds": hcc.checkInterval.Seconds(),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("❌ Failed to encode health status: %v", err)
	}
}

// generatePrometheusMetrics generates Prometheus-formatted health metrics
func (hcc *HealthCheckCollector) generatePrometheusMetrics() string {
	var metrics strings.Builder

	// Add metadata
	metrics.WriteString("# HELP component_health_status Component health status (1=up, 0=down)\n")
	metrics.WriteString("# TYPE component_health_status gauge\n")

	metrics.WriteString("# HELP component_health_response_time Component health check response time in milliseconds\n")
	metrics.WriteString("# TYPE component_health_response_time gauge\n")

	metrics.WriteString("# HELP component_health_success_rate Component health check success rate percentage\n")
	metrics.WriteString("# TYPE component_health_success_rate gauge\n")

	metrics.WriteString("# HELP component_health_consecutive_failures Number of consecutive health check failures\n")
	metrics.WriteString("# TYPE component_health_consecutive_failures gauge\n")

	metrics.WriteString("# HELP component_health_total_checks Total number of health checks performed\n")
	metrics.WriteString("# TYPE component_health_total_checks counter\n")

	// Generate metrics for each component
	for _, health := range hcc.healthCache {
		labels := hcc.generateLabels(health)

		// Health status (1 for up, 0 for down)
		statusValue := 0
		if health.Status == HealthStatusUp {
			statusValue = 1
		}

		metrics.WriteString(fmt.Sprintf("component_health_status{%s} %d\n",
			labels, statusValue))

		metrics.WriteString(fmt.Sprintf("component_health_response_time{%s} %s\n",
			labels, formatFloat(health.ResponseTime)))

		metrics.WriteString(fmt.Sprintf("component_health_success_rate{%s} %s\n",
			labels, formatFloat(health.SuccessRate)))

		metrics.WriteString(fmt.Sprintf("component_health_consecutive_failures{%s} %d\n",
			labels, health.ConsecutiveFails))

		metrics.WriteString(fmt.Sprintf("component_health_total_checks{%s} %d\n",
			labels, health.TotalChecks))
	}

	return metrics.String()
}

// generateLabels generates Prometheus labels for a health check
func (hcc *HealthCheckCollector) generateLabels(health *ComponentHealth) string {
	labels := []string{
		fmt.Sprintf(`component_name="%s"`, health.Name),
		fmt.Sprintf(`component_id="%s"`, health.ComponentID),
		fmt.Sprintf(`check_type="%s"`, health.CheckType),
		fmt.Sprintf(`endpoint="%s"`, health.Endpoint),
	}

	// Add topology-specific labels if available
	if hcc.topology != nil {
		if component, exists := hcc.topology.Components[health.ComponentID]; exists {
			labels = append(labels,
				fmt.Sprintf(`component_type="%s"`, component.Type),
				fmt.Sprintf(`deployment_type="%s"`, hcc.topology.Type),
			)
		}
	}

	return strings.Join(labels, ",")
}

// generateHealthSummary creates a summary of overall health status
func (hcc *HealthCheckCollector) generateHealthSummary() map[string]any {
	total := len(hcc.healthCache)
	up := 0
	down := 0
	avgResponseTime := 0.0

	for _, health := range hcc.healthCache {
		if health.Status == HealthStatusUp {
			up++
		} else {
			down++
		}
		avgResponseTime += health.ResponseTime
	}

	if total > 0 {
		avgResponseTime /= float64(total)
	}

	return map[string]any{
		"total_components":     total,
		"components_up":        up,
		"components_down":      down,
		"health_percentage":    float64(up) / float64(total) * 100,
		"avg_response_time_ms": avgResponseTime,
	}
}

// UpdateTopology updates the topology and regenerates health check configs
func (hcc *HealthCheckCollector) UpdateTopology(topology *discovery.NetworkTopology) {
	hcc.topology = topology
	hcc.generateHealthCheckConfigs()
	log.Printf("🔄 Updated health check configurations for %d components", len(hcc.checkConfigs))
}

// GetHealthStatus returns the current health status for all components
func (hcc *HealthCheckCollector) GetHealthStatus() map[string]*ComponentHealth {
	return maps.Clone(hcc.healthCache)
}
