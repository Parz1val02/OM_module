package metrics

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NFType represents the type of Network Function
type NFType string

const (
	NF_AMF NFType = "amf"

	NF_SMF NFType = "smf"
	NF_PCF NFType = "pcf"
	NF_UPF NFType = "upf"
	NF_MME NFType = "mme"

	NF_PCRF NFType = "pcrf"
)

// OpenGSMetric represents a parsed metric from Open5GS
type OpenGSMetric struct {
	Name   string
	Type   string // gauge, counter, histogram
	Help   string
	Value  float64
	Labels map[string]string
}

// RealOpen5GSCollector fetches real metrics from Open5GS endpoints
type RealOpen5GSCollector struct {
	nfType      NFType
	port        int
	componentIP string
	open5gsPort int // The actual Open5GS metrics port (usually 9091)
	registry    *prometheus.Registry
	httpServer  *http.Server
	httpClient  *http.Client
	topology    *discovery.NetworkTopology

	// Real metrics storage
	realMetrics    map[string]*prometheus.GaugeVec
	realCounters   map[string]*prometheus.CounterVec
	lastUpdate     time.Time
	lastFetchError error
	mu             sync.RWMutex
}

// NewRealOpen5GSCollector creates a collector that fetches from actual Open5GS endpoints
func NewRealOpen5GSCollector(nfType NFType, port int, componentIP string, open5gsPort int) *RealOpen5GSCollector {
	registry := prometheus.NewRegistry()

	collector := &RealOpen5GSCollector{
		nfType:      nfType,
		port:        port,
		componentIP: componentIP,
		open5gsPort: open5gsPort,
		registry:    registry,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		realMetrics:  make(map[string]*prometheus.GaugeVec),
		realCounters: make(map[string]*prometheus.CounterVec),
	}

	return collector
}

// Start starts the real metrics collector
func (roc *RealOpen5GSCollector) Start(ctx context.Context, topology *discovery.NetworkTopology) error {
	roc.topology = topology

	// Set up HTTP server for re-exposing metrics
	mux := http.NewServeMux()

	// Prometheus metrics endpoint - exposes the collected real metrics
	mux.Handle("/metrics", promhttp.HandlerFor(roc.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	// Health check endpoint
	mux.HandleFunc("/health", roc.handleHealthCheck)

	// Educational dashboard endpoint
	mux.HandleFunc("/dashboard", roc.handleEducationalDashboard)

	// Debug endpoint to see raw fetched metrics
	mux.HandleFunc("/debug/raw", roc.handleRawMetrics)

	roc.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", roc.port),
		Handler: mux,
	}

	// Start metrics collection goroutine
	go roc.startMetricsCollection(ctx)

	// Start HTTP server in goroutine
	go func() {
		log.Printf("🚀 Real %s metrics collector listening on :%d (fetching from %s:%d)",
			strings.ToUpper(string(roc.nfType)), roc.port, roc.componentIP, roc.open5gsPort)
		if err := roc.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ Real %s collector server error: %v",
				strings.ToUpper(string(roc.nfType)), err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	log.Printf("🛑 Shutting down real %s collector...", strings.ToUpper(string(roc.nfType)))
	return roc.httpServer.Shutdown(context.Background())
}

// startMetricsCollection begins fetching metrics from the real Open5GS endpoint
func (roc *RealOpen5GSCollector) startMetricsCollection(ctx context.Context) {
	// Initial fetch
	roc.fetchAndParseMetrics()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			roc.fetchAndParseMetrics()
		}
	}
}

// fetchAndParseMetrics fetches metrics from the real Open5GS endpoint
func (roc *RealOpen5GSCollector) fetchAndParseMetrics() {
	roc.mu.Lock()
	defer roc.mu.Unlock()

	if roc.componentIP == "" {
		roc.lastFetchError = fmt.Errorf("component IP not configured")
		return
	}

	url := fmt.Sprintf("http://%s:%d/metrics", roc.componentIP, roc.open5gsPort)

	log.Printf("🔍 Fetching real metrics from %s for %s", url, roc.nfType)

	resp, err := roc.httpClient.Get(url)
	if err != nil {
		roc.lastFetchError = fmt.Errorf("failed to fetch from Open5GS: %w", err)
		log.Printf("⚠️  Failed to fetch metrics from %s: %v", url, err)
		return
	}
	defer func() {
		err = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		roc.lastFetchError = fmt.Errorf("Open5GS returned status %d", resp.StatusCode)
		log.Printf("⚠️  Open5GS metrics endpoint returned status %d", resp.StatusCode)
		return
	}

	// Parse the Prometheus format response
	metrics, err := roc.parsePrometheusFormat(resp.Body)
	if err != nil {
		roc.lastFetchError = fmt.Errorf("failed to parse metrics: %w", err)
		log.Printf("⚠️  Failed to parse metrics from %s: %v", url, err)
		return
	}

	// Update our internal Prometheus metrics
	roc.updateInternalMetrics(metrics)

	roc.lastUpdate = time.Now()
	roc.lastFetchError = nil

	log.Printf("✅ Successfully fetched %d metrics from %s", len(metrics), roc.nfType)
}

// parsePrometheusFormat parses the Prometheus text format from Open5GS
func (roc *RealOpen5GSCollector) parsePrometheusFormat(reader io.Reader) ([]OpenGSMetric, error) {
	var metrics []OpenGSMetric
	var currentMetric OpenGSMetric

	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse HELP lines
		if strings.HasPrefix(line, "# HELP ") {
			parts := strings.SplitN(line[7:], " ", 2)
			if len(parts) >= 2 {
				currentMetric = OpenGSMetric{
					Name:   parts[0],
					Help:   parts[1],
					Labels: make(map[string]string),
				}
			}
			continue
		}

		// Parse TYPE lines
		if strings.HasPrefix(line, "# TYPE ") {
			parts := strings.Fields(line[7:])
			if len(parts) >= 2 {
				currentMetric.Type = parts[1]
			}
			continue
		}

		// Skip other comment lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Parse metric lines
		if strings.Contains(line, " ") {
			metric, err := roc.parseMetricLine(line, currentMetric)
			if err != nil {
				log.Printf("⚠️  Failed to parse metric line '%s': %v", line, err)
				continue
			}
			metrics = append(metrics, metric)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading metrics: %w", err)
	}

	return metrics, nil
}

// parseMetricLine parses a single metric line like "metric_name{label1="value1"} 123"
func (roc *RealOpen5GSCollector) parseMetricLine(line string, templateMetric OpenGSMetric) (OpenGSMetric, error) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return OpenGSMetric{}, fmt.Errorf("invalid metric line format")
	}

	metricPart := parts[0]
	valuePart := parts[1]

	// Parse the value
	value, err := strconv.ParseFloat(valuePart, 64)
	if err != nil {
		return OpenGSMetric{}, fmt.Errorf("invalid metric value: %w", err)
	}

	// Initialize metric with template data
	metric := OpenGSMetric{
		Help:   templateMetric.Help,
		Type:   templateMetric.Type,
		Value:  value,
		Labels: make(map[string]string),
	}

	// Parse metric name and labels
	if strings.Contains(metricPart, "{") {
		// Has labels: metric_name{label1="value1",label2="value2"}
		nameEnd := strings.Index(metricPart, "{")
		metric.Name = metricPart[:nameEnd]

		labelsPart := metricPart[nameEnd+1 : len(metricPart)-1] // Remove { and }
		labels, err := roc.parseLabels(labelsPart)
		if err != nil {
			return OpenGSMetric{}, fmt.Errorf("failed to parse labels: %w", err)
		}
		metric.Labels = labels
	} else {
		// No labels: just metric_name
		metric.Name = metricPart
	}

	return metric, nil
}

// parseLabels parses label string like 'label1="value1",label2="value2"'
func (roc *RealOpen5GSCollector) parseLabels(labelStr string) (map[string]string, error) {
	labels := make(map[string]string)

	if labelStr == "" {
		return labels, nil
	}

	// Split by comma, but be careful with quoted values
	var currentLabel strings.Builder
	var inQuotes bool
	var labelPairs []string

	for _, char := range labelStr {
		if char == '"' {
			inQuotes = !inQuotes
			currentLabel.WriteRune(char)
		} else if char == ',' && !inQuotes {
			labelPairs = append(labelPairs, currentLabel.String())
			currentLabel.Reset()
		} else {
			currentLabel.WriteRune(char)
		}
	}

	// Add the last label
	if currentLabel.Len() > 0 {
		labelPairs = append(labelPairs, currentLabel.String())
	}

	// Parse each label pair
	for _, pair := range labelPairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		labels[key] = value
	}

	return labels, nil
}

// updateInternalMetrics updates our internal Prometheus metrics with fetched data
func (roc *RealOpen5GSCollector) updateInternalMetrics(fetchedMetrics []OpenGSMetric) {
	for _, metric := range fetchedMetrics {
		// Create label slices for Prometheus metrics
		var labelNames []string
		var labelValues []string

		// Always add component_id and nf_type labels
		labelNames = append(labelNames, "component_id", "nf_type")
		labelValues = append(labelValues, string(roc.nfType), string(roc.nfType))

		// Add labels from the metric
		for labelName, labelValue := range metric.Labels {
			labelNames = append(labelNames, labelName)
			labelValues = append(labelValues, labelValue)
		}

		// Create or update Prometheus metric based on type
		switch metric.Type {
		case "gauge":
			if _, exists := roc.realMetrics[metric.Name]; !exists {
				roc.realMetrics[metric.Name] = prometheus.NewGaugeVec(
					prometheus.GaugeOpts{
						Name: metric.Name,
						Help: metric.Help,
					},
					labelNames,
				)
				roc.registry.MustRegister(roc.realMetrics[metric.Name])
			}
			roc.realMetrics[metric.Name].WithLabelValues(labelValues...).Set(metric.Value)

		case "counter":
			if _, exists := roc.realCounters[metric.Name]; !exists {
				roc.realCounters[metric.Name] = prometheus.NewCounterVec(
					prometheus.CounterOpts{
						Name: metric.Name,
						Help: metric.Help,
					},
					labelNames,
				)
				roc.registry.MustRegister(roc.realCounters[metric.Name])
			}
			// For counters, we need to be careful about setting values
			// Since Prometheus counters only increase, we set the gauge to the current value
			roc.realCounters[metric.Name].WithLabelValues(labelValues...).Add(0) // Initialize if needed
		}
	}
}

// handleHealthCheck provides health status for the collector
func (roc *RealOpen5GSCollector) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	roc.mu.RLock()
	defer roc.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	status := "healthy"
	if roc.lastFetchError != nil {
		status = "degraded"
	}

	if time.Since(roc.lastUpdate) > 30*time.Second {
		status = "unhealthy"
	}

	health := map[string]any{
		"status":           status,
		"nf_type":          string(roc.nfType),
		"component_ip":     roc.componentIP,
		"open5gs_port":     roc.open5gsPort,
		"last_update":      roc.lastUpdate.Unix(),
		"last_fetch_error": nil,
		"metrics_count":    len(roc.realMetrics) + len(roc.realCounters),
		"uptime":           time.Since(roc.lastUpdate).String(),
	}

	if roc.lastFetchError != nil {
		health["last_fetch_error"] = roc.lastFetchError.Error()
	}

	if err := json.NewEncoder(w).Encode(health); err != nil {
		log.Printf("❌ Health collector server error: %v", err)
	}
}

// handleEducationalDashboard provides educational information about the NF
func (roc *RealOpen5GSCollector) handleEducationalDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var info map[string]any

	switch roc.nfType {
	case NF_AMF:
		info = map[string]any{
			"name":           "Access and Mobility Management Function",
			"description":    "Manages UE access, mobility, and security in 5G",
			"key_interfaces": []string{"N1", "N2", "N8", "N11", "N14"},
			"main_functions": []string{
				"UE registration and authentication",
				"Mobility management and tracking",
				"Session management coordination",
				"Security anchor function",
			},
			"real_metrics_examples": []string{
				"fivegs_amffunction_rm_reginitreq - Initial registration requests",
				"fivegs_amffunction_rm_reginitsucc - Successful registrations",
				"amf_session - Active AMF sessions",
				"ran_ue - Connected RAN UEs",
				"gnb - Number of gNodeBs",
			},
		}
	case NF_SMF:
		info = map[string]any{
			"name":           "Session Management Function",
			"description":    "Manages PDU sessions and coordinates with UPF",
			"key_interfaces": []string{"N4", "N7", "N10", "N11"},
			"main_functions": []string{
				"PDU session establishment and management",
				"UPF selection and PFCP management",
				"QoS policy enforcement",
				"Charging trigger points",
			},
			"real_metrics_examples": []string{
				"pfcp_sessions_active - Active PFCP sessions",
				"ues_active - Active user equipments",
				"gtp2_sessions_active - Active GTP sessions",
				"bearers_active - Active bearers",
				"fivegs_smffunction_sm_n4sessionreport - N4 session reports",
			},
		}
	case NF_UPF:
		info = map[string]any{
			"name":           "User Plane Function",
			"description":    "Handles user data forwarding and processing",
			"key_interfaces": []string{"N3", "N4", "N6", "N9"},
			"main_functions": []string{
				"Packet routing and forwarding",
				"QoS handling and traffic steering",
				"Usage reporting and charging",
				"Data path anchor for mobility",
			},
			"real_metrics_examples": []string{
				"upf_sessions - Active UPF sessions",
				"upf_packets_total - Packet counters",
				"upf_bytes_total - Byte counters",
			},
		}
	case NF_PCF:
		info = map[string]any{
			"name":           "Policy Control Function",
			"description":    "Provides policy rules and charging control",
			"key_interfaces": []string{"N5", "N7", "N15", "N28"},
			"main_functions": []string{
				"Policy decision making",
				"QoS and charging rule provision",
				"Access and mobility policy",
				"UE policy association management",
			},
			"real_metrics_examples": []string{
				"pcf_sessions - Active PCF sessions",
				"pcf_policies_total - Policy rule counters",
			},
		}
	case NF_MME:
		info = map[string]any{
			"name":           "Mobility Management Entity",
			"description":    "Core control node for 4G LTE networks",
			"key_interfaces": []string{"S1-MME", "S6a", "S11", "S3"},
			"main_functions": []string{
				"UE attach and detach procedures",
				"Bearer management",
				"Handover control",
				"Authentication and security",
			},
			"real_metrics_examples": []string{
				"mme_ue_total - Total UE count",
				"mme_bearer_total - Bearer counts",
				"mme_sessions_active - Active sessions",
			},
		}
	case NF_PCRF:
		info = map[string]any{
			"name":           "Policy Charging and Rules Function",
			"description":    "Policy and charging control for 4G networks",
			"key_interfaces": []string{"Gx", "Rx", "Sp", "Sy"},
			"main_functions": []string{
				"Policy and charging rule creation",
				"Quality of Service control",
				"Application function interaction",
				"Subscription profile repository",
			},
			"real_metrics_examples": []string{
				"pcrf_diameter_sessions - Diameter sessions",
				"pcrf_policy_decisions_total - Policy decisions",
			},
		}
	}

	// Add real-time status
	roc.mu.RLock()
	info["real_time_status"] = map[string]any{
		"last_update":       roc.lastUpdate.Unix(),
		"metrics_count":     len(roc.realMetrics) + len(roc.realCounters),
		"fetch_url":         fmt.Sprintf("http://%s:%d/metrics", roc.componentIP, roc.open5gsPort),
		"collection_active": roc.lastFetchError == nil,
	}
	roc.mu.RUnlock()

	if err := json.NewEncoder(w).Encode(info); err != nil {
		log.Printf("❌ Information server error: %v", err)
	}
}

// handleRawMetrics provides debug endpoint to see raw fetched metrics
func (roc *RealOpen5GSCollector) handleRawMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	url := fmt.Sprintf("http://%s:%d/metrics", roc.componentIP, roc.open5gsPort)

	resp, err := roc.httpClient.Get(url)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		if _, err := fmt.Fprintf(w, "Error fetching from Open5GS: %v\n", err); err != nil {
			log.Printf("❌ Debug server error: %v", err)
		}
		return
	}
	defer func() {
		err = resp.Body.Close()
	}()

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("❌ Debug server error: %v", err)
	}
}

// GetCollectorConfig returns configuration for the real collector
func (roc *RealOpen5GSCollector) GetCollectorConfig() map[string]any {
	roc.mu.RLock()
	defer roc.mu.RUnlock()

	return map[string]any{
		"nf_type":        string(roc.nfType),
		"collector_port": roc.port,
		"component_ip":   roc.componentIP,
		"open5gs_port":   roc.open5gsPort,
		"fetch_url":      fmt.Sprintf("http://%s:%d/metrics", roc.componentIP, roc.open5gsPort),
		"last_update":    roc.lastUpdate.Unix(),
		"status":         roc.lastFetchError == nil,
		"metrics_count":  len(roc.realMetrics) + len(roc.realCounters),
	}
}

// RealCollectorManager manages real Open5GS metrics collectors
type RealCollectorManager struct {
	collectors map[string]*RealOpen5GSCollector
	topology   *discovery.NetworkTopology
	mu         sync.RWMutex
}

// NewRealCollectorManager creates a new manager for real collectors
func NewRealCollectorManager() *RealCollectorManager {
	return &RealCollectorManager{
		collectors: make(map[string]*RealOpen5GSCollector),
	}
}

// InitializeCollectors creates collectors for all supported Open5GS components
func (rcm *RealCollectorManager) InitializeCollectors(topology *discovery.NetworkTopology) error {
	rcm.mu.Lock()
	defer rcm.mu.Unlock()

	rcm.topology = topology

	// Configuration for each supported NF with their typical ports
	nfConfigs := map[string]struct {
		nfType      NFType
		port        int // Our collector port
		open5gsPort int // Open5GS metrics port
	}{
		"amf":  {NF_AMF, 9091, 9091},
		"smf":  {NF_SMF, 9092, 9091},
		"pcf":  {NF_PCF, 9093, 9091},
		"upf":  {NF_UPF, 9094, 9091},
		"mme":  {NF_MME, 9095, 9091},
		"pcrf": {NF_PCRF, 9096, 9091},
	}

	// Create collectors for components that exist in topology
	for componentName, component := range topology.Components {
		if !component.IsRunning {
			continue
		}

		// Check if this component is a supported NF
		for nfName, config := range nfConfigs {
			if containsNF(componentName, nfName) {
				collector := NewRealOpen5GSCollector(
					config.nfType,
					config.port,
					component.IP,
					config.open5gsPort,
				)

				rcm.collectors[componentName] = collector
				log.Printf("✅ Created real collector for %s (%s) at %s:%d -> :%d",
					componentName, config.nfType, component.IP, config.open5gsPort, config.port)
				break
			}
		}
	}

	if len(rcm.collectors) == 0 {
		return fmt.Errorf("no supported Open5GS components found in topology")
	}

	log.Printf("🎯 Initialized %d real collectors", len(rcm.collectors))
	return nil
}

// StartAll starts all initialized collectors
func (rcm *RealCollectorManager) StartAll(ctx context.Context) error {
	rcm.mu.RLock()
	defer rcm.mu.RUnlock()

	if len(rcm.collectors) == 0 {
		return fmt.Errorf("no collectors initialized")
	}

	// Start each collector in its own goroutine
	for componentName, collector := range rcm.collectors {
		go func(name string, coll *RealOpen5GSCollector) {
			if err := coll.Start(ctx, rcm.topology); err != nil {
				log.Printf("❌ Real collector for %s failed: %v", name, err)
			}
		}(componentName, collector)
	}

	log.Printf("🚀 Started %d real collectors", len(rcm.collectors))
	return nil
}

// GetCollectorStatus returns status of all collectors
func (rcm *RealCollectorManager) GetCollectorStatus() map[string]any {
	rcm.mu.RLock()
	defer rcm.mu.RUnlock()

	status := map[string]any{
		"total_collectors": len(rcm.collectors),
		"collectors":       make(map[string]any),
	}

	for componentName, collector := range rcm.collectors {
		status["collectors"].(map[string]any)[componentName] = collector.GetCollectorConfig()
	}

	return status
}

// GetPrometheusTargets returns Prometheus scrape configuration for all collectors
func (rcm *RealCollectorManager) GetPrometheusTargets() []map[string]any {
	rcm.mu.RLock()
	defer rcm.mu.RUnlock()

	var targets []map[string]any

	for componentName, collector := range rcm.collectors {
		config := collector.GetCollectorConfig()

		target := map[string]any{
			"job_name":    fmt.Sprintf("%s-real", componentName),
			"targets":     []string{fmt.Sprintf("localhost:%d", config["collector_port"])},
			"scrape_path": "/metrics",
			"interval":    "5s",
			"labels": map[string]string{
				"component":      componentName,
				"component_type": string(collector.nfType),
				"nf_type":        string(collector.nfType),
				"source":         "real_open5gs",
				"component_ip":   config["component_ip"].(string),
			},
		}

		// Add deployment type from topology
		if rcm.topology != nil {
			target["labels"].(map[string]string)["deployment"] = string(rcm.topology.Type)
		}

		targets = append(targets, target)
	}

	return targets
}

// GeneratePrometheusConfig generates a complete Prometheus configuration
func (rcm *RealCollectorManager) GeneratePrometheusConfig() string {
	targets := rcm.GetPrometheusTargets()

	config := `global:
  scrape_interval: 5s
  evaluation_interval: 5s
  external_labels:
    monitor: 'om-module-real-open5gs'

scrape_configs:
`

	for _, target := range targets {
		labels := target["labels"].(map[string]string)

		config += fmt.Sprintf(`  - job_name: '%s'
    scrape_interval: %s
    metrics_path: '%s'
    static_configs:
      - targets: %v
        labels:
`, target["job_name"], target["interval"], target["scrape_path"], target["targets"])

		for key, value := range labels {
			config += fmt.Sprintf("          %s: '%s'\n", key, value)
		}
		config += "\n"
	}

	return config
}

// RealCollectorOrchestrator orchestrates the entire real metrics collection system
type RealCollectorOrchestrator struct {
	discoveryService *discovery.AutoDiscoveryService
	collectorManager *RealCollectorManager
	ctx              context.Context
	cancel           context.CancelFunc
	status           string
	mu               sync.RWMutex
}

// NewRealCollectorOrchestrator creates a new orchestrator
func NewRealCollectorOrchestrator(discoveryService *discovery.AutoDiscoveryService) *RealCollectorOrchestrator {
	ctx, cancel := context.WithCancel(context.Background())

	return &RealCollectorOrchestrator{
		discoveryService: discoveryService,
		collectorManager: NewRealCollectorManager(),
		ctx:              ctx,
		cancel:           cancel,
		status:           "initialized",
	}
}

// Start starts the orchestration process
func (rco *RealCollectorOrchestrator) Start() error {
	rco.mu.Lock()
	defer rco.mu.Unlock()

	log.Printf("🎬 Starting Real Open5GS Metrics Orchestrator...")
	rco.status = "starting"

	// Discover network topology
	topology, err := rco.discoveryService.DiscoverTopology(rco.ctx)
	if err != nil {
		rco.status = "failed"
		return fmt.Errorf("failed to discover topology: %w", err)
	}

	log.Printf("🔍 Discovered %d components in topology", len(topology.Components))

	// Initialize collectors based on discovered topology
	if err := rco.collectorManager.InitializeCollectors(topology); err != nil {
		rco.status = "failed"
		return fmt.Errorf("failed to initialize collectors: %w", err)
	}

	// Start all collectors
	if err := rco.collectorManager.StartAll(rco.ctx); err != nil {
		rco.status = "failed"
		return fmt.Errorf("failed to start collectors: %w", err)
	}

	// Generate Prometheus configuration
	prometheusConfig := rco.collectorManager.GeneratePrometheusConfig()
	if err := rco.writePrometheusConfig(prometheusConfig); err != nil {
		log.Printf("⚠️  Failed to write Prometheus config: %v", err)
	}

	rco.status = "running"
	log.Printf("✅ Real Open5GS metrics orchestrator started successfully")
	return nil
}

// Stop stops the orchestrator
func (rco *RealCollectorOrchestrator) Stop() {
	rco.mu.Lock()
	defer rco.mu.Unlock()

	log.Printf("🛑 Stopping Real Open5GS Metrics Orchestrator...")
	rco.status = "stopping"
	rco.cancel()
	rco.status = "stopped"
}

// GetStatus returns the current status of the orchestrator
func (rco *RealCollectorOrchestrator) GetStatus() map[string]any {
	rco.mu.RLock()
	defer rco.mu.RUnlock()

	return map[string]any{
		"status":     rco.status,
		"collectors": rco.collectorManager.GetCollectorStatus(),
		"targets":    rco.collectorManager.GetPrometheusTargets(),
	}
}

// GetMetricsEndpoints returns all available metrics endpoints
func (rco *RealCollectorOrchestrator) GetMetricsEndpoints() map[string]string {
	status := rco.collectorManager.GetCollectorStatus()
	endpoints := make(map[string]string)

	if collectors, ok := status["collectors"].(map[string]any); ok {
		for componentName, collectorInfo := range collectors {
			if info, ok := collectorInfo.(map[string]any); ok {
				if port, exists := info["collector_port"]; exists {
					endpoints[componentName] = fmt.Sprintf("http://localhost:%v/metrics", port)
				}
			}
		}
	}

	return endpoints
}

// WaitForHealthy waits for all collectors to be healthy with timeout
func (rco *RealCollectorOrchestrator) WaitForHealthy(timeout time.Duration) error {
	rco.mu.RLock()
	defer rco.mu.RUnlock()

	if rco.status != "running" {
		return fmt.Errorf("orchestrator not running (status: %s)", rco.status)
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-rco.ctx.Done():
			return fmt.Errorf("orchestrator context cancelled")
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for healthy collectors")
			}

			// Check if all collectors are healthy
			status := rco.collectorManager.GetCollectorStatus()
			if collectors, ok := status["collectors"].(map[string]any); ok {
				allHealthy := true
				for _, collectorInfo := range collectors {
					if info, ok := collectorInfo.(map[string]any); ok {
						if status, exists := info["status"]; exists && status != true {
							allHealthy = false
							break
						}
					}
				}
				if allHealthy && len(collectors) > 0 {
					log.Printf("✅ All collectors are healthy")
					return nil
				}
			}
		}
	}
}

// Restart restarts the orchestrator (useful for configuration changes)
func (rco *RealCollectorOrchestrator) Restart() error {
	log.Printf("🔄 Restarting Real Open5GS Metrics Orchestrator...")

	// Stop current instance
	rco.Stop()

	// Wait a moment for cleanup
	time.Sleep(2 * time.Second)

	// Create new context
	rco.ctx, rco.cancel = context.WithCancel(context.Background())

	// Start again
	return rco.Start()
}

// writePrometheusConfig writes the Prometheus configuration to file
func (rco *RealCollectorOrchestrator) writePrometheusConfig(config string) error {
	filename := "prometheus_real_open5gs.yaml"

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create prometheus config file: %w", err)
	}
	defer func() {
		err = file.Close()
	}()

	if _, err := file.WriteString(config); err != nil {
		return fmt.Errorf("failed to write prometheus config: %w", err)
	}

	log.Printf("📄 Generated Prometheus configuration: %s", filename)
	return nil
}

// containsNF checks if a component name contains the NF name (case-insensitive)
func containsNF(componentName, nfName string) bool {
	lower := strings.ToLower(componentName)
	return strings.Contains(lower, nfName)
}
