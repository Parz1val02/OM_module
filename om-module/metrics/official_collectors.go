package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	NF_AMF  NFType = "amf"
	NF_SMF  NFType = "smf"
	NF_PCF  NFType = "pcf"
	NF_UPF  NFType = "upf"
	NF_MME  NFType = "mme"
	NF_PCRF NFType = "pcrf"
)

// NFMetrics represents core metrics for network functions
type NFMetrics struct {
	// Common 5G/4G Metrics
	ActiveSessions        prometheus.GaugeVec
	SessionSetupRate      prometheus.CounterVec
	SessionTeardownRate   prometheus.CounterVec
	MessageProcessingTime prometheus.HistogramVec
	ErrorRate             prometheus.CounterVec
	CPUUsage              prometheus.GaugeVec
	MemoryUsage           prometheus.GaugeVec

	// NF-specific metrics
	SpecificMetrics map[string]prometheus.Collector
}

// OfficialEndpointCollector manages official metrics endpoints for network functions
type OfficialEndpointCollector struct {
	nfType     NFType
	port       int
	registry   *prometheus.Registry
	metrics    *NFMetrics
	httpServer *http.Server
	topology   *discovery.NetworkTopology

	// Session tracking
	sessionData sync.Map
	lastUpdate  time.Time
}

// NewOfficialEndpointCollector creates a new official endpoint collector for a specific NF type
func NewOfficialEndpointCollector(nfType NFType, port int) *OfficialEndpointCollector {
	registry := prometheus.NewRegistry()

	collector := &OfficialEndpointCollector{
		nfType:   nfType,
		port:     port,
		registry: registry,
		metrics:  createNFMetrics(nfType),
	}

	// Register metrics with Prometheus
	collector.registerMetrics()

	return collector
}

// createNFMetrics creates Prometheus metrics for the specific NF type
func createNFMetrics(nfType NFType) *NFMetrics {
	metrics := &NFMetrics{
		ActiveSessions: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: fmt.Sprintf("%s_active_sessions_total", nfType),
				Help: fmt.Sprintf("Total number of active sessions in %s", strings.ToUpper(string(nfType))),
			},
			[]string{"slice_id", "dnn", "component_id"},
		),

		SessionSetupRate: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: fmt.Sprintf("%s_session_setup_total", nfType),
				Help: fmt.Sprintf("Total number of session setups processed by %s", strings.ToUpper(string(nfType))),
			},
			[]string{"result", "slice_id", "dnn", "component_id"},
		),

		SessionTeardownRate: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: fmt.Sprintf("%s_session_teardown_total", nfType),
				Help: fmt.Sprintf("Total number of session teardowns processed by %s", strings.ToUpper(string(nfType))),
			},
			[]string{"reason", "slice_id", "component_id"},
		),

		MessageProcessingTime: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    fmt.Sprintf("%s_message_processing_duration_seconds", nfType),
				Help:    fmt.Sprintf("Time spent processing messages in %s", strings.ToUpper(string(nfType))),
				Buckets: prometheus.DefBuckets,
			},
			[]string{"message_type", "component_id"},
		),

		ErrorRate: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: fmt.Sprintf("%s_errors_total", nfType),
				Help: fmt.Sprintf("Total number of errors in %s", strings.ToUpper(string(nfType))),
			},
			[]string{"error_type", "severity", "component_id"},
		),

		CPUUsage: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: fmt.Sprintf("%s_cpu_usage_percent", nfType),
				Help: fmt.Sprintf("CPU usage percentage for %s", strings.ToUpper(string(nfType))),
			},
			[]string{"component_id"},
		),

		MemoryUsage: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: fmt.Sprintf("%s_memory_usage_bytes", nfType),
				Help: fmt.Sprintf("Memory usage in bytes for %s", strings.ToUpper(string(nfType))),
			},
			[]string{"component_id"},
		),

		SpecificMetrics: make(map[string]prometheus.Collector),
	}

	// Add NF-specific metrics
	switch nfType {
	case NF_AMF:
		metrics.SpecificMetrics["ue_attached"] = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "amf_ue_attached_total",
				Help: "Total number of UEs attached to AMF",
			},
			[]string{"tracking_area", "component_id"},
		)

		metrics.SpecificMetrics["handover_rate"] = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "amf_handover_total",
				Help: "Total number of handovers processed by AMF",
			},
			[]string{"result", "handover_type", "component_id"},
		)

	case NF_SMF:
		metrics.SpecificMetrics["pdu_sessions"] = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "smf_pdu_sessions_total",
				Help: "Total number of PDU sessions managed by SMF",
			},
			[]string{"session_type", "dnn", "component_id"},
		)

		metrics.SpecificMetrics["pfcp_associations"] = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "smf_pfcp_associations_total",
				Help: "Total number of PFCP associations in SMF",
			},
			[]string{"upf_id", "status", "component_id"},
		)

	case NF_UPF:
		metrics.SpecificMetrics["packet_throughput"] = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "upf_packet_throughput_total",
				Help: "Total packets processed by UPF",
			},
			[]string{"direction", "dnn", "component_id"},
		)

		metrics.SpecificMetrics["data_volume"] = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "upf_data_volume_bytes_total",
				Help: "Total data volume processed by UPF in bytes",
			},
			[]string{"direction", "dnn", "component_id"},
		)

	case NF_PCF:
		metrics.SpecificMetrics["policy_rules"] = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pcf_policy_rules_active_total",
				Help: "Total number of active policy rules in PCF",
			},
			[]string{"rule_type", "component_id"},
		)

		metrics.SpecificMetrics["charging_events"] = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pcf_charging_events_total",
				Help: "Total number of charging events processed by PCF",
			},
			[]string{"event_type", "component_id"},
		)

	case NF_MME:
		metrics.SpecificMetrics["ue_attached"] = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "mme_ue_attached_total",
				Help: "Total number of UEs attached to MME",
			},
			[]string{"tracking_area", "component_id"},
		)

		metrics.SpecificMetrics["bearer_setup"] = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "mme_bearer_setup_total",
				Help: "Total number of bearer setups processed by MME",
			},
			[]string{"result", "bearer_type", "component_id"},
		)

	case NF_PCRF:
		metrics.SpecificMetrics["diameter_sessions"] = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pcrf_diameter_sessions_total",
				Help: "Total number of Diameter sessions in PCRF",
			},
			[]string{"session_type", "component_id"},
		)

		metrics.SpecificMetrics["policy_decisions"] = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pcrf_policy_decisions_total",
				Help: "Total number of policy decisions made by PCRF",
			},
			[]string{"decision_type", "component_id"},
		)
	}

	return metrics
}

// registerMetrics registers all metrics with the Prometheus registry
func (oec *OfficialEndpointCollector) registerMetrics() {
	oec.registry.MustRegister(&oec.metrics.ActiveSessions)
	oec.registry.MustRegister(&oec.metrics.SessionSetupRate)
	oec.registry.MustRegister(&oec.metrics.SessionTeardownRate)
	oec.registry.MustRegister(&oec.metrics.MessageProcessingTime)
	oec.registry.MustRegister(&oec.metrics.ErrorRate)
	oec.registry.MustRegister(&oec.metrics.CPUUsage)
	oec.registry.MustRegister(&oec.metrics.MemoryUsage)

	// Register NF-specific metrics
	for _, metric := range oec.metrics.SpecificMetrics {
		oec.registry.MustRegister(metric)
	}
}

// Start starts the official metrics endpoint server
func (oec *OfficialEndpointCollector) Start(ctx context.Context, topology *discovery.NetworkTopology) error {
	oec.topology = topology

	// Set up HTTP server
	mux := http.NewServeMux()

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.HandlerFor(oec.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	// Health check endpoint
	mux.HandleFunc("/health", oec.handleHealthCheck)

	// Educational dashboard endpoint
	mux.HandleFunc("/dashboard", oec.handleEducationalDashboard)

	// Configuration endpoint
	mux.HandleFunc("/config", oec.handleConfiguration)

	oec.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", oec.port),
		Handler: mux,
	}

	// Start metrics collection goroutine
	go oec.startMetricsCollection(ctx)

	// Start HTTP server in goroutine
	go func() {
		log.Printf("🚀 %s official endpoint collector listening on :%d",
			strings.ToUpper(string(oec.nfType)), oec.port)
		if err := oec.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ %s endpoint collector server error: %v",
				strings.ToUpper(string(oec.nfType)), err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	log.Printf("🛑 Shutting down %s official endpoint collector...",
		strings.ToUpper(string(oec.nfType)))
	return oec.httpServer.Shutdown(context.Background())
}

// startMetricsCollection begins collecting and updating metrics
func (oec *OfficialEndpointCollector) startMetricsCollection(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			oec.collectMetrics()
		}
	}
}

// collectMetrics simulates collecting real metrics from the network function
func (oec *OfficialEndpointCollector) collectMetrics() {
	if oec.topology == nil {
		return
	}

	// Find component in topology
	var component *discovery.Component
	for name, comp := range oec.topology.Components {
		if strings.Contains(strings.ToLower(name), string(oec.nfType)) && comp.IsRunning {
			component = &comp
			break
		}
	}

	if component == nil {
		return
	}

	componentID := component.Name

	// Simulate realistic metrics based on NF type
	switch oec.nfType {
	case NF_AMF:
		oec.simulateAMFMetrics(componentID)
	case NF_SMF:
		oec.simulateSMFMetrics(componentID)
	case NF_UPF:
		oec.simulateUPFMetrics(componentID)
	case NF_PCF:
		oec.simulatePCFMetrics(componentID)
	case NF_MME:
		oec.simulateMMEMetrics(componentID)
	case NF_PCRF:
		oec.simulatePCRFMetrics(componentID)
	}

	oec.lastUpdate = time.Now()
}

// simulateAMFMetrics generates realistic AMF metrics
func (oec *OfficialEndpointCollector) simulateAMFMetrics(componentID string) {
	// Simulate UE attachments (fluctuates between 10-50)
	ueCount := 25 + float64(time.Now().Unix()%20) - 10
	oec.metrics.SpecificMetrics["ue_attached"].(*prometheus.GaugeVec).
		WithLabelValues("TAC_001", componentID).Set(ueCount)

	// Active sessions
	oec.metrics.ActiveSessions.WithLabelValues("default", "internet", componentID).
		Set(ueCount * 1.2) // Slightly more sessions than UEs

	// Session setup rate (incremental)
	oec.metrics.SessionSetupRate.WithLabelValues("success", "default", "internet", componentID).
		Add(float64(time.Now().Unix()%5 + 1))

	// Handover events
	if time.Now().Unix()%10 == 0 { // Occasional handovers
		oec.metrics.SpecificMetrics["handover_rate"].(*prometheus.CounterVec).
			WithLabelValues("success", "intra_amf", componentID).Add(1)
	}

	// System metrics
	oec.metrics.CPUUsage.WithLabelValues(componentID).Set(30 + float64(time.Now().Unix()%40))
	oec.metrics.MemoryUsage.WithLabelValues(componentID).Set(1024*1024*1024*2 + float64(time.Now().Unix()%500000000))
}

// simulateSMFMetrics generates realistic SMF metrics
func (oec *OfficialEndpointCollector) simulateSMFMetrics(componentID string) {
	// PDU sessions
	pduSessions := 15 + float64(time.Now().Unix()%30)
	oec.metrics.SpecificMetrics["pdu_sessions"].(*prometheus.GaugeVec).
		WithLabelValues("IPv4", "internet", componentID).Set(pduSessions)

	// PFCP associations
	oec.metrics.SpecificMetrics["pfcp_associations"].(*prometheus.GaugeVec).
		WithLabelValues("upf_001", "associated", componentID).Set(1)

	// Session metrics
	oec.metrics.ActiveSessions.WithLabelValues("default", "internet", componentID).Set(pduSessions)

	// Message processing time
	oec.metrics.MessageProcessingTime.WithLabelValues("pdu_session_create", componentID).
		Observe(0.050 + float64(time.Now().Unix()%20)/1000.0)

	// System metrics
	oec.metrics.CPUUsage.WithLabelValues(componentID).Set(25 + float64(time.Now().Unix()%30))
	oec.metrics.MemoryUsage.WithLabelValues(componentID).Set(1024*1024*1024*1.5 + float64(time.Now().Unix()%300000000))
}

// simulateUPFMetrics generates realistic UPF metrics
func (oec *OfficialEndpointCollector) simulateUPFMetrics(componentID string) {
	// Packet throughput
	packetsPerSecond := 1000 + time.Now().Unix()%5000
	oec.metrics.SpecificMetrics["packet_throughput"].(*prometheus.CounterVec).
		WithLabelValues("uplink", "internet", componentID).Add(float64(packetsPerSecond))
	oec.metrics.SpecificMetrics["packet_throughput"].(*prometheus.CounterVec).
		WithLabelValues("downlink", "internet", componentID).Add(float64(packetsPerSecond * 2))

	// Data volume
	dataVolume := packetsPerSecond * 1500 // Average packet size
	oec.metrics.SpecificMetrics["data_volume"].(*prometheus.CounterVec).
		WithLabelValues("uplink", "internet", componentID).Add(float64(dataVolume))
	oec.metrics.SpecificMetrics["data_volume"].(*prometheus.CounterVec).
		WithLabelValues("downlink", "internet", componentID).Add(float64(dataVolume * 3))

	// Active sessions from PFCP
	oec.metrics.ActiveSessions.WithLabelValues("default", "internet", componentID).
		Set(15 + float64(time.Now().Unix()%30))

	// High CPU usage for data plane
	oec.metrics.CPUUsage.WithLabelValues(componentID).Set(60 + float64(time.Now().Unix()%30))
	oec.metrics.MemoryUsage.WithLabelValues(componentID).Set(1024*1024*1024*3 + float64(time.Now().Unix()%500000000))
}

// simulatePCFMetrics generates realistic PCF metrics
func (oec *OfficialEndpointCollector) simulatePCFMetrics(componentID string) {
	// Policy rules
	oec.metrics.SpecificMetrics["policy_rules"].(*prometheus.GaugeVec).
		WithLabelValues("qos", componentID).Set(50 + float64(time.Now().Unix()%20))
	oec.metrics.SpecificMetrics["policy_rules"].(*prometheus.GaugeVec).
		WithLabelValues("charging", componentID).Set(30 + float64(time.Now().Unix()%15))

	// Charging events
	if time.Now().Unix()%3 == 0 {
		oec.metrics.SpecificMetrics["charging_events"].(*prometheus.CounterVec).
			WithLabelValues("start", componentID).Add(1)
	}

	// Light system usage for policy functions
	oec.metrics.CPUUsage.WithLabelValues(componentID).Set(15 + float64(time.Now().Unix()%20))
	oec.metrics.MemoryUsage.WithLabelValues(componentID).Set(1024*1024*1024 + float64(time.Now().Unix()%200000000))
}

// simulateMMEMetrics generates realistic MME metrics
func (oec *OfficialEndpointCollector) simulateMMEMetrics(componentID string) {
	// UE attachments
	ueCount := 20 + float64(time.Now().Unix()%40)
	oec.metrics.SpecificMetrics["ue_attached"].(*prometheus.GaugeVec).
		WithLabelValues("TAC_001", componentID).Set(ueCount)

	// Bearer setup
	if time.Now().Unix()%5 == 0 {
		oec.metrics.SpecificMetrics["bearer_setup"].(*prometheus.CounterVec).
			WithLabelValues("success", "default", componentID).Add(1)
	}

	oec.metrics.CPUUsage.WithLabelValues(componentID).Set(35 + float64(time.Now().Unix()%25))
	oec.metrics.MemoryUsage.WithLabelValues(componentID).Set(1024*1024*1024*2.5 + float64(time.Now().Unix()%400000000))
}

// simulatePCRFMetrics generates realistic PCRF metrics
func (oec *OfficialEndpointCollector) simulatePCRFMetrics(componentID string) {
	// Diameter sessions
	oec.metrics.SpecificMetrics["diameter_sessions"].(*prometheus.GaugeVec).
		WithLabelValues("gx", componentID).Set(25 + float64(time.Now().Unix()%20))

	// Policy decisions
	if time.Now().Unix()%4 == 0 {
		oec.metrics.SpecificMetrics["policy_decisions"].(*prometheus.CounterVec).
			WithLabelValues("install", componentID).Add(1)
	}

	oec.metrics.CPUUsage.WithLabelValues(componentID).Set(20 + float64(time.Now().Unix()%25))
	oec.metrics.MemoryUsage.WithLabelValues(componentID).Set(1024*1024*1024*1.8 + float64(time.Now().Unix()%300000000))
}

// handleHealthCheck provides health status for the collector
func (oec *OfficialEndpointCollector) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	health := map[string]any{
		"status":       "healthy",
		"nf_type":      string(oec.nfType),
		"last_update":  oec.lastUpdate.Unix(),
		"uptime":       time.Since(oec.lastUpdate).String(),
		"metrics_port": oec.port,
	}

	json.NewEncoder(w).Encode(health)
}

// handleEducationalDashboard provides educational information about the NF
func (oec *OfficialEndpointCollector) handleEducationalDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var info map[string]any

	switch oec.nfType {
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
			"kpis": []string{
				"amf_ue_attached_total",
				"amf_handover_total",
				"amf_active_sessions_total",
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
			"kpis": []string{
				"smf_pdu_sessions_total",
				"smf_pfcp_associations_total",
				"smf_session_setup_total",
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
			"kpis": []string{
				"upf_packet_throughput_total",
				"upf_data_volume_bytes_total",
				"upf_active_sessions_total",
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
			"kpis": []string{
				"pcf_policy_rules_active_total",
				"pcf_charging_events_total",
				"pcf_message_processing_duration_seconds",
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
			"kpis": []string{
				"mme_ue_attached_total",
				"mme_bearer_setup_total",
				"mme_active_sessions_total",
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
			"kpis": []string{
				"pcrf_diameter_sessions_total",
				"pcrf_policy_decisions_total",
				"pcrf_message_processing_duration_seconds",
			},
		}
	}

	json.NewEncoder(w).Encode(info)
}

// handleConfiguration provides current collector configuration
func (oec *OfficialEndpointCollector) handleConfiguration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	config := map[string]any{
		"nf_type":             string(oec.nfType),
		"port":                oec.port,
		"metrics_path":        "/metrics",
		"health_path":         "/health",
		"dashboard_path":      "/dashboard",
		"last_update":         oec.lastUpdate.Unix(),
		"collection_interval": "5s",
	}

	json.NewEncoder(w).Encode(config)
}

// GetCollectorFactory returns a factory function to create collectors for different NF types
func GetCollectorFactory() map[NFType]func(int) *OfficialEndpointCollector {
	return map[NFType]func(int) *OfficialEndpointCollector{
		NF_AMF:  func(port int) *OfficialEndpointCollector { return NewOfficialEndpointCollector(NF_AMF, port) },
		NF_SMF:  func(port int) *OfficialEndpointCollector { return NewOfficialEndpointCollector(NF_SMF, port) },
		NF_PCF:  func(port int) *OfficialEndpointCollector { return NewOfficialEndpointCollector(NF_PCF, port) },
		NF_UPF:  func(port int) *OfficialEndpointCollector { return NewOfficialEndpointCollector(NF_UPF, port) },
		NF_MME:  func(port int) *OfficialEndpointCollector { return NewOfficialEndpointCollector(NF_MME, port) },
		NF_PCRF: func(port int) *OfficialEndpointCollector { return NewOfficialEndpointCollector(NF_PCRF, port) },
	}
}
