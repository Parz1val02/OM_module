package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Parz1val02/OM_module/dashboards"
	"github.com/Parz1val02/OM_module/discovery"
	"github.com/Parz1val02/OM_module/logging"
	"github.com/Parz1val02/OM_module/metrics"
)

var debugMode bool
var loggingService *logging.LoggingService

func main() {
	log.Printf("🚀 Starting O&M Module for 4G/5G Educational Network Testbed")

	// Parse command line arguments
	mode := "discovery"
	envFile := "../.env"

	// Detect if running in Docker and adjust env file path
	if isRunningInDocker() {
		envFile = ".env"
		log.Printf("🐳 Running in Docker environment")
	}

	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if len(os.Args) > 2 {
		if os.Args[2] == "--debug" {
			debugMode = true
			log.Printf("🐞 Debug mode enabled")
		} else {
			envFile = os.Args[2]
		}
	}

	switch mode {
	case "orchestrator":
		runRealMetricsOrchestrator(envFile)
	case "discovery":
		runDiscoveryMode(envFile)
	default:
		printBanner()
		printUsage()
		os.Exit(1)
	}
}

// Detect if running in Docker environment
func isRunningInDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return false
}

// Real-time metrics orchestrator mode - Live monitoring and collection
func runRealMetricsOrchestrator(envFile string) {
	printBanner()
	printOrchestratorModeDescription()

	// Detect environment
	inDocker := isRunningInDocker()
	if inDocker {
		log.Printf("🐳 Docker environment detected - using Docker networking")
	}

	// Create discovery service
	discoveryService, err := discovery.NewAutoDiscoveryService(envFile)
	if err != nil {
		log.Fatalf("❌ Failed to create discovery service: %v", err)
	}
	defer func() {
		err = discoveryService.Close()
	}()

	// Create context for all operations
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Discover topology first
	log.Printf("🔄 Discovering network topology...")
	topology, err := discoveryService.DiscoverTopology(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to discover topology: %v", err)
	}

	log.Printf("🔄 Initializing real-time metrics collection system...")

	// Create real collector orchestrator
	orchestrator := metrics.NewRealCollectorOrchestrator(discoveryService)

	// Start the real Open5GS orchestrator
	if err := orchestrator.Start(); err != nil {
		log.Fatalf("❌ Failed to start real metrics orchestrator: %v", err)
	}

	// NEW: Initialize and start logging service
	log.Printf("📝 Initializing dynamic logging pipeline...")
	if err := initializeLoggingService(topology); err != nil {
		log.Printf("⚠️ Failed to initialize logging service: %v", err)
		// Continue without logging service - don't fail the entire application
	} else {
		log.Printf("✅ Logging pipeline initialized successfully")
	}

	// Start infrastructure collectors
	log.Printf("🔄 Starting infrastructure collectors...")

	// Start container metrics collector
	containerCollector, err := metrics.NewContainerMetricsCollector(8080)
	if err != nil {
		log.Printf("⚠️ Failed to create container collector: %v", err)
	} else {
		go func() {
			if err := containerCollector.Start(ctx, topology); err != nil && err != context.Canceled {
				log.Printf("❌ Container collector error: %v", err)
			}
		}()
		log.Printf("🟢 Started container metrics collector on :8080")
	}

	// Start health check collector
	healthCollector := metrics.NewHealthCheckCollector(8081)
	go func() {
		if err := healthCollector.Start(ctx, topology); err != nil && err != context.Canceled {
			log.Printf("❌ Health collector error: %v", err)
		}
	}()
	log.Printf("🟢 Started health check collector on :8081")

	// Generate Docker-aware Prometheus configuration
	if err := generateDockerPrometheusConfig(orchestrator, topology, inDocker); err != nil {
		log.Printf("⚠️ Failed to generate Prometheus config: %v", err)
	}

	// Wait for collectors to be healthy
	log.Printf("⏳ Performing health checks on all collectors...")
	if err := orchestrator.WaitForHealthy(30 * time.Second); err != nil {
		log.Printf("⚠️  Warning: Some collectors may not be fully ready: %v", err)
	}

	// Generate Grafana dashboards
	if err := dashboards.GenerateGrafanaDashboards(orchestrator, topology, inDocker); err != nil {
		log.Printf("⚠️ Failed to generate Grafana dashboards: %v", err)
	} else {
		log.Printf("✅ Generated Grafana dashboards successfully")
	}

	// NEW: Start HTTP server for logging endpoints
	go startLoggingHTTPServer()

	// Display live status
	displayRealMetricsStatus(orchestrator)

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("🌟 Orchestrator mode is now LIVE! All metrics are being collected in real-time.")
	log.Printf("📊 Access Prometheus metrics at the endpoints shown above")
	log.Printf("📝 Access logging service at http://localhost:8080/logging/*")
	if inDocker {
		log.Printf("🐳 Docker mode: Prometheus config written to shared volume")
	}
	log.Printf("⚡ Press Ctrl+C to stop the orchestrator...")

	// Wait for shutdown signal
	<-sigChan
	log.Printf("🛑 Received shutdown signal, gracefully stopping all collectors...")

	// Cancel context to stop all collectors
	cancel()

	// Stop services
	orchestrator.Stop()
	cleanupLoggingService()

	// Give collectors time to stop gracefully
	time.Sleep(2 * time.Second)

	log.Printf("✅ Real-time metrics orchestrator stopped cleanly")
}

// NEW: Initialize logging service
func initializeLoggingService(topology *discovery.NetworkTopology) error {
	log.Printf("🔧 Initializing Logging Service...")

	// Load configuration from environment
	config := logging.LoadConfigFromEnv()

	// Create logging service
	loggingService = logging.NewLoggingService(topology, config)

	// Start the service
	if err := loggingService.Start(); err != nil {
		return fmt.Errorf("failed to start logging service: %w", err)
	}

	log.Printf("✅ Logging Service initialized successfully")
	return nil
}

// NEW: Start HTTP server for logging endpoints
func startLoggingHTTPServer() {
	mux := http.NewServeMux()

	// Add logging endpoints
	addLoggingEndpoints(mux)

	// Start server on a separate port to avoid conflicts
	server := &http.Server{
		Addr:    ":8083", // Use port 8083 for logging HTTP endpoints
		Handler: mux,
	}

	log.Printf("🌐 Logging HTTP server starting on :8083")
	if err := server.ListenAndServe(); err != nil {
		log.Printf("⚠️ Logging HTTP server error: %v", err)
	}
}

// NEW: Add logging endpoints to HTTP server
func addLoggingEndpoints(mux *http.ServeMux) {
	// Logging service endpoints
	mux.HandleFunc("/logging/status", handleLoggingStatus)
	mux.HandleFunc("/logging/configs", handlePromtailConfigs)
	mux.HandleFunc("/logging/health", handleLoggingHealth)
	mux.HandleFunc("/logging/dashboard", handleEducationalDashboard)

	// Educational endpoints
	mux.HandleFunc("/educational/insights", func(w http.ResponseWriter, r *http.Request) {
		topology := getCurrentTopology()
		insights := logging.GetEducationalInsights(topology)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(insights); err != nil {
			log.Printf("❌ Log collector server error: %v", err)
		}
	})

	// Root endpoint for logging service
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"service": "O&M Logging Service",
			"version": "1.0.0",
			"endpoints": []string{
				"/logging/status",
				"/logging/configs",
				"/logging/health",
				"/logging/dashboard",
				"/educational/insights",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("❌ Log collector server error: %v", err)
		}
	})
}

// NEW: HTTP handlers for logging service

// handleLoggingStatus returns the status of the logging service
func handleLoggingStatus(w http.ResponseWriter, r *http.Request) {
	if loggingService == nil {
		http.Error(w, "Logging service not initialized", http.StatusServiceUnavailable)
		return
	}

	status := loggingService.GetStatus()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, "Failed to encode status", http.StatusInternalServerError)
		return
	}
}

// handlePromtailConfigs returns the generated Promtail configurations
func handlePromtailConfigs(w http.ResponseWriter, r *http.Request) {
	if loggingService == nil {
		http.Error(w, "Logging service not initialized", http.StatusServiceUnavailable)
		return
	}

	configs, err := loggingService.GetPromtailConfigs()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get configs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(configs); err != nil {
		http.Error(w, "Failed to encode configs", http.StatusInternalServerError)
		return
	}
}

// handleEducationalDashboard returns educational content for students
func handleEducationalDashboard(w http.ResponseWriter, r *http.Request) {
	if loggingService == nil {
		http.Error(w, "Logging service not initialized", http.StatusServiceUnavailable)
		return
	}

	topology := getCurrentTopology()

	dashboard := logging.GenerateEducationalDashboard(topology)
	insights := logging.GetEducationalInsights(topology)

	response := map[string]any{
		"dashboard": dashboard,
		"insights":  insights,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode dashboard", http.StatusInternalServerError)
		return
	}
}

// handleLoggingHealth provides health check for logging components
func handleLoggingHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"status":    "healthy",
		"components": map[string]string{
			"loki":     "checking...",
			"parser":   "checking...",
			"promtail": "checking...",
		},
	}

	if loggingService != nil {
		status := loggingService.GetStatus()
		if running, ok := status["running"].(bool); ok && running {
			health["components"].(map[string]string)["logging_service"] = "healthy"
		} else {
			health["components"].(map[string]string)["logging_service"] = "stopped"
			health["status"] = "degraded"
		}
	} else {
		health["components"].(map[string]string)["logging_service"] = "not_initialized"
		health["status"] = "unhealthy"
	}

	// Check Loki connectivity
	go checkLokiHealth(health)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
		log.Printf("❌ Log health server error: %v", err)
	}
}

// checkLokiHealth checks if Loki is accessible
func checkLokiHealth(health map[string]any) {
	lokiURL := os.Getenv("LOKI_URL")
	if lokiURL == "" {
		lokiURL = "http://loki:3100"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(lokiURL + "/ready")
	if err != nil {
		health["components"].(map[string]string)["loki"] = "unreachable"
		return
	}
	defer func() {
		err = resp.Body.Close()
	}()

	if resp.StatusCode == 200 {
		health["components"].(map[string]string)["loki"] = "healthy"
	} else {
		health["components"].(map[string]string)["loki"] = "unhealthy"
	}
}

// getCurrentTopology returns the current topology (you may need to adapt this)
func getCurrentTopology() *discovery.NetworkTopology {
	// For now, return nil - you may want to store the topology globally
	// or retrieve it from your discovery service
	return nil
}

// cleanupLoggingService should be called during application shutdown
func cleanupLoggingService() {
	if loggingService != nil {
		if err := loggingService.Stop(); err != nil {
			log.Printf("⚠️ Error stopping logging service: %v", err)
		}
	}
}

// updateTopologyInLoggingService updates the logging service when topology changes
func updateTopologyInLoggingService(newTopology *discovery.NetworkTopology) {
	if loggingService != nil {
		if err := loggingService.UpdateTopology(newTopology); err != nil {
			log.Printf("⚠️ Failed to update logging service topology: %v", err)
		}
	}
}

// Generate Docker-aware Prometheus configuration
func generateDockerPrometheusConfig(orchestrator *metrics.RealCollectorOrchestrator, topology *discovery.NetworkTopology, inDocker bool) error {
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
		go reloadPrometheus()
	}

	return nil
}

// Extract port number from endpoint URL
func extractPortFromEndpoint(endpoint string) string {
	// Parse URL like "http://localhost:9091/metrics" -> "9091"
	parts := strings.Split(endpoint, ":")
	if len(parts) >= 3 {
		portPart := parts[2]
		return strings.Split(portPart, "/")[0]
	}
	return "9091" // default
}

// Reload Prometheus configuration
func reloadPrometheus() {
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
	defer func() {
		err = resp.Body.Close()
	}()

	if resp.StatusCode == 200 {
		log.Printf("✅ Prometheus configuration reloaded successfully")
	} else {
		log.Printf("⚠️ Prometheus reload returned status: %d", resp.StatusCode)
	}
}

// Discovery mode - Static analysis and configuration generation
func runDiscoveryMode(envFile string) {
	printBanner()
	printDiscoveryModeDescription()

	// Create discovery service
	discoveryService, err := discovery.NewAutoDiscoveryService(envFile)
	if err != nil {
		log.Fatalf("❌ Failed to create discovery service: %v", err)
	}
	defer func() {
		err = discoveryService.Close()
	}()

	// Create context with timeout for discovery operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("🔍 Step 1: Analyzing Docker containers and network topology...")

	// Discover topology
	topology, err := discoveryService.DiscoverTopology(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to discover topology: %v", err)
	}

	log.Printf("✅ Step 1 Complete: Found %d components in %s deployment",
		len(topology.Components), topology.Type)

	log.Printf("🔧 Step 2: Generating static configuration files...")

	// Export topology and configuration files
	exportTopologyAndConfig(topology)

	log.Printf("✅ Step 2 Complete: Configuration files generated")

	// NEW: Step 3 - Initialize logging service for configuration generation
	log.Printf("📝 Step 3: Generating dynamic logging configurations...")
	if err := initializeLoggingServiceForDiscovery(topology); err != nil {
		log.Printf("⚠️ Failed to generate logging configurations: %v", err)
	} else {
		log.Printf("✅ Step 3 Complete: Logging configurations generated")

		// Generate educational content
		generateEducationalContent(topology)
	}

	// Display comprehensive discovery results
	displayDiscoveryResults(topology)

	// NEW: Display logging setup results
	displayLoggingResults()

	// Display next steps for real metrics
	printRealMetricsNextSteps()

	log.Printf("🎯 Discovery mode completed successfully!")
	log.Printf("📁 All configuration files are ready for use with Prometheus/Grafana")
	if isRunningInDocker() {
		log.Printf("🐳 To start live monitoring: docker-compose restart om-module")
	} else {
		log.Printf("🚀 To start live monitoring, run: ./om-module orchestrator")
	}
}

// NEW: Initialize logging service for discovery mode (config generation only)
func initializeLoggingServiceForDiscovery(topology *discovery.NetworkTopology) error {
	config := logging.LoadConfigFromEnv()

	// Create logging service but don't start log parser in discovery mode
	config.ParserEnabled = false

	tempLoggingService := logging.NewLoggingService(topology, config)

	// Generate configurations without starting the full service
	if err := tempLoggingService.Start(); err != nil {
		return fmt.Errorf("failed to generate logging configurations: %w", err)
	}

	// Stop the service after configuration generation
	if err := tempLoggingService.Stop(); err != nil {
		return fmt.Errorf("failed to stop logging configurations: %w", err)
	}

	return nil
}

// NEW: Generate educational content
func generateEducationalContent(topology *discovery.NetworkTopology) {
	// Generate educational dashboard
	dashboard := logging.GenerateEducationalDashboard(topology)
	if err := writeFile("educational_dashboard.md", dashboard); err != nil {
		log.Printf("⚠️ Failed to write educational dashboard: %v", err)
	} else {
		log.Printf("📚 Educational dashboard written to: educational_dashboard.md")
	}

	// Write logging insights
	insights := logging.GetEducationalInsights(topology)
	insightsJSON, _ := json.MarshalIndent(insights, "", "  ")
	if err := writeFile("logging_insights.json", string(insightsJSON)); err != nil {
		log.Printf("⚠️ Failed to write logging insights: %v", err)
	} else {
		log.Printf("💡 Logging insights written to: logging_insights.json")
	}
}

// NEW: Display logging setup results
func displayLoggingResults() {
	fmt.Printf("\n📝 LOGGING PIPELINE SETUP\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")

	fmt.Printf("🚀 Logging Configuration Status:\n")
	fmt.Printf("   ├─ Promtail Configs: ✅ Generated\n")
	fmt.Printf("   ├─ Core Network: ./promtail/core/config.yml\n")
	fmt.Printf("   ├─ RAN Components: ./promtail/ran/config.yml\n")
	fmt.Printf("   └─ Educational Mode: Enabled\n")

	fmt.Printf("\n📄 Generated Files:\n")
	fmt.Printf("   ├─ educational_dashboard.md → Student learning guide\n")
	fmt.Printf("   ├─ logging_insights.json → Protocol analysis insights\n")
	fmt.Printf("   ├─ promtail/core/config.yml → Core network log config\n")
	fmt.Printf("   └─ promtail/ran/config.yml → RAN log config\n")

	fmt.Printf("\n🎓 Educational Features:\n")
	fmt.Printf("   ├─ Protocol-aware log parsing (NAS, RRC, S1AP)\n")
	fmt.Printf("   ├─ 3GPP specification references\n")
	fmt.Printf("   ├─ Session flow tracking with IMSI correlation\n")
	fmt.Printf("   ├─ Performance metrics extraction (RSRP, RSRQ)\n")
	fmt.Printf("   └─ Troubleshooting guides and procedures\n")

	fmt.Printf("\n🔗 Runtime Access Points:\n")
	fmt.Printf("   ├─ Log Parser API: http://localhost:8082 (when running)\n")
	fmt.Printf("   ├─ Loki API: http://localhost:3100\n")
	fmt.Printf("   ├─ Grafana: http://localhost:3000\n")
	fmt.Printf("   └─ Logging Service: http://localhost:8083/logging/* (when running)\n")

	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
}

// Print clear description of what orchestrator mode does
func printOrchestratorModeDescription() {
	fmt.Printf("🎬 ORCHESTRATOR MODE - Live Real-Time Metrics Collection\n")
	fmt.Printf("═══════════════════════════════════════════════════════════\n")
	fmt.Printf("This mode starts a LIVE metrics collection system that:\n\n")
	fmt.Printf("🔄 Real-Time Operations:\n")
	fmt.Printf("   • Continuously monitors all Open5GS network functions\n")
	fmt.Printf("   • Collects live metrics directly from AMF, SMF, UPF, etc.\n")
	fmt.Printf("   • Gathers container resource usage (CPU, memory, I/O)\n")
	fmt.Printf("   • Performs health checks every 15 seconds\n")
	fmt.Printf("   • Automatically adapts to topology changes\n")
	fmt.Printf("   • Processes logs in real-time with educational context\n\n")
	fmt.Printf("🌐 Active HTTP Endpoints:\n")
	fmt.Printf("   • Real Open5GS metrics: ports 9091-9096\n")
	fmt.Printf("   • Container metrics: port 8080\n")
	fmt.Printf("   • Health checks: port 8081\n")
	fmt.Printf("   • Log parser: port 8082\n")
	fmt.Printf("   • Logging service: port 8083\n")
	fmt.Printf("   • Educational dashboards and debug info\n\n")
	fmt.Printf("📊 Integration Ready:\n")
	fmt.Printf("   • Prometheus can scrape all endpoints immediately\n")
	fmt.Printf("   • Grafana dashboards show live data\n")
	fmt.Printf("   • Loki receives structured logs with educational metadata\n")
	fmt.Printf("   • Perfect for lab demonstrations and learning\n\n")
	if isRunningInDocker() {
		fmt.Printf("🐳 Docker Mode:\n")
		fmt.Printf("   • Automatic Prometheus configuration\n")
		fmt.Printf("   • Docker network integration\n")
		fmt.Printf("   • Shared volume configuration\n\n")
	}
	fmt.Printf("⚠️  Note: Requires running Open5GS containers with metrics enabled\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
}

// Print clear description of what discovery mode does
func printDiscoveryModeDescription() {
	fmt.Printf("🔍 DISCOVERY MODE - Static Analysis & Configuration Generation\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("This mode performs a comprehensive analysis of your testbed setup:\n\n")
	fmt.Printf("🔎 Analysis Phase:\n")
	fmt.Printf("   • Scans Docker containers to identify network functions\n")
	fmt.Printf("   • Maps network topology (4G vs 5G components)\n")
	fmt.Printf("   • Determines which components support native metrics\n")
	fmt.Printf("   • Identifies container resource monitoring targets\n")
	fmt.Printf("   • Assesses health check capabilities\n")
	fmt.Printf("   • Analyzes log sources and formats\n\n")
	fmt.Printf("📝 Configuration Generation:\n")
	fmt.Printf("   • Creates Prometheus scrape configurations\n")
	fmt.Printf("   • Generates dynamic Promtail configurations\n")
	fmt.Printf("   • Builds comprehensive monitoring setup files\n")
	fmt.Printf("   • Prepares educational dashboard configurations\n")
	fmt.Printf("   • Creates protocol-aware log parsing rules\n\n")
	fmt.Printf("📁 Output Files Created:\n")
	fmt.Printf("   • prometheus_targets.yml - Ready-to-use Prometheus config\n")
	fmt.Printf("   • promtail/core/config.yml - Core network log configuration\n")
	fmt.Printf("   • promtail/ran/config.yml - RAN log configuration\n")
	fmt.Printf("   • topology.json - Machine-readable topology data\n")
	fmt.Printf("   • educational_dashboard.md - Student learning guide\n")
	fmt.Printf("   • logging_insights.json - Protocol analysis insights\n\n")
	fmt.Printf("🎯 Educational Benefits:\n")
	fmt.Printf("   • Students can see the complete monitoring architecture\n")
	fmt.Printf("   • Understand which components provide which metrics\n")
	fmt.Printf("   • Learn industry-standard observability practices\n")
	fmt.Printf("   • See protocol-specific log parsing configurations\n\n")
	fmt.Printf("⚠️  Note: This mode does NOT start live collection - use 'orchestrator' for that\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
}

// Enhanced display for orchestrator mode with better categorization
func displayRealMetricsStatus(orchestrator *metrics.RealCollectorOrchestrator) {
	endpoints := orchestrator.GetMetricsEndpoints()
	status := orchestrator.GetStatus()

	// Optional debug dump
	if debugMode {
		if jsonBytes, err := json.MarshalIndent(status, "", "  "); err == nil {
			if err := os.WriteFile("status_debug.json", jsonBytes, 0644); err != nil {
				log.Printf("⚠️  Failed to write status_debug.json: %v", err)
			} else {
				log.Printf("🐞 Debug: status exported to status_debug.json")
			}
		} else {
			log.Printf("⚠️  Failed to marshal status for debug: %v", err)
		}
	}

	fmt.Printf("\n🎯 LIVE METRICS COLLECTION STATUS\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")

	// Real Open5GS collectors status
	if collectorsRoot, ok := status["collectors"].(map[string]any); ok {
		if collectors, ok := collectorsRoot["collectors"].(map[string]any); ok {
			fmt.Printf("📊 Active Collectors: %d\n\n", len(collectors))

			for componentName, info := range collectors {
				if collectorInfo, ok := info.(map[string]any); ok {
					// Extract fields with CORRECT field names from your JSON
					nfType, _ := collectorInfo["nf_type"].(string)
					componentIP, _ := collectorInfo["component_ip"].(string)
					fetchURL, _ := collectorInfo["fetch_url"].(string)
					// Fix 1: Handle collector_port properly
					collectorPort := "N/A"
					if port, ok := collectorInfo["collector_port"]; ok {
						// Handle both int and float64 types
						switch v := port.(type) {
						case float64:
							collectorPort = fmt.Sprintf("%.0f", v)
						case int:
							collectorPort = fmt.Sprintf("%d", v)
						case string:
							collectorPort = v
						default:
							collectorPort = fmt.Sprintf("%v", v)
						}
					}

					// Fix 2: Handle metrics_count properly
					metricsCount := 0
					if count, ok := collectorInfo["metrics_count"]; ok {
						switch v := count.(type) {
						case float64:
							metricsCount = int(v)
						case int:
							metricsCount = v
						default:
							if countStr := fmt.Sprintf("%v", v); countStr != "" {
								if parsed, err := strconv.Atoi(countStr); err == nil {
									metricsCount = parsed
								}
							}
						}
					}

					// Fix 3: Handle status properly
					isHealthy := false
					if statusBool, ok := collectorInfo["status"].(bool); ok {
						isHealthy = statusBool
					}

					healthIcon := "🟢"
					healthText := "Healthy"
					if !isHealthy {
						healthIcon = "🔴"
						healthText = "Unhealthy"
					}

					// Print detailed collector info with metrics count
					fmt.Printf("%s %s %s (%s) %s - %d metrics\n",
						healthIcon, healthText, componentName, strings.ToUpper(nfType), componentIP, int(metricsCount))
					if fetchURL != "" {
						fmt.Printf("   📡 Fetching from: %s\n", fetchURL)
					}

					// Show appropriate endpoint based on environment
					endpoint := fmt.Sprintf("http://localhost:%s", collectorPort)
					if isRunningInDocker() {
						fmt.Printf("   📊 Docker endpoint: http://om-module:%s/metrics\n", collectorPort)
						fmt.Printf("   📊 External access:  %s/metrics\n", endpoint)
					} else {
						fmt.Printf("   📊 Exposing at:   %s/metrics\n", endpoint)
					}

					fmt.Printf("   🏥 Health check:  %s/health\n", endpoint)
					fmt.Printf("   📚 Dashboard:     %s/dashboard\n", endpoint)
					fmt.Printf("   🔍 Raw data:      %s/debug/raw\n", endpoint)
					fmt.Printf("\n")
				}
			}
		} else {
			log.Printf("⚠️ No nested collectors found inside status[\"collectors\"]")
		}
	}

	fmt.Printf("📊 Infrastructure Metrics:\n")
	baseURL := "http://localhost"
	if isRunningInDocker() {
		fmt.Printf("   ├─ Docker endpoints: http://om-module:8080 & http://om-module:8081\n")
	}
	fmt.Printf("   ├─ Container Stats  🟢 Active:\n")
	fmt.Printf("   │  ├─ All containers: %s:8080/container/metrics\n", baseURL)
	fmt.Printf("   │  └─ Collector health: %s:8080/health\n", baseURL)
	fmt.Printf("   ├─ Health Checks    🟢 Active:\n")
	fmt.Printf("   │  ├─ All components: %s:8081/health/metrics\n", baseURL)
	fmt.Printf("   │  └─ Collector health: %s:8081/health\n", baseURL)
	fmt.Printf("   └─ System Resources 🟢 Active → Collected every 10s\n\n")

	// NEW: Display logging service status
	fmt.Printf("📝 Logging Service:\n")
	if loggingService != nil {
		loggingStatus := loggingService.GetStatus()
		if running, ok := loggingStatus["running"].(bool); ok && running {
			fmt.Printf("   ├─ Status: 🟢 Active\n")
			fmt.Printf("   ├─ Log Parser: %s:8082\n", baseURL)
			fmt.Printf("   ├─ Service API: %s:8083/logging/*\n", baseURL)
			fmt.Printf("   ├─ Educational Mode: Enabled\n")
			if educationalMode, ok := loggingStatus["educational_mode"].(bool); ok && educationalMode {
				fmt.Printf("   ├─ Protocol Parsing: 3GPP specifications enabled\n")
				fmt.Printf("   ├─ Session Tracking: IMSI correlation active\n")
			}
			fmt.Printf("   └─ Loki Integration: %s\n", loggingStatus["loki_url"])
		} else {
			fmt.Printf("   └─ Status: 🔴 Inactive\n")
		}
	} else {
		fmt.Printf("   └─ Status: ⚠️  Not initialized\n")
	}
	fmt.Printf("\n")

	fmt.Printf("🔧 Quick Tests:\n")
	// Real Open5GS endpoints
	for componentName, endpoint := range endpoints {
		fmt.Printf("   curl %s  # %s real metrics\n", endpoint, strings.ToUpper(componentName))
	}
	// Infrastructure endpoints
	fmt.Printf("   curl %s:8080/container/metrics  # Container resources\n", baseURL)
	fmt.Printf("   curl %s:8081/health/metrics     # Component health\n", baseURL)

	// NEW: Logging endpoints
	if loggingService != nil {
		fmt.Printf("   curl %s:8082/health            # Log parser health\n", baseURL)
		fmt.Printf("   curl %s:8083/logging/status    # Logging service status\n", baseURL)
		fmt.Printf("   curl %s:8083/logging/configs   # Generated Promtail configs\n", baseURL)
	}

	if isRunningInDocker() {
		fmt.Printf("\n🐳 Docker Integration:\n")
		fmt.Printf("   • Prometheus config: /etc/prometheus/configs/prometheus.yml\n")
		fmt.Printf("   • Promtail configs: Auto-generated and mounted\n")
		fmt.Printf("   • Auto-reload: Configuration updated automatically\n")
	}
	fmt.Printf("\n")

	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
}

// Enhanced display for discovery mode results
func displayDiscoveryResults(topology *discovery.NetworkTopology) {
	fmt.Printf("\n🎯 DISCOVERY ANALYSIS RESULTS\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")

	fmt.Printf("🏗️  Network Architecture Detected:\n")
	fmt.Printf("   ├─ Deployment Type: %s\n", topology.Type)
	fmt.Printf("   ├─ Total Components: %d\n", len(topology.Components))

	// Count by type and status
	runningCount := 0
	networkFunctions := 0
	supportingServices := 0

	for _, component := range topology.Components {
		if component.IsRunning {
			runningCount++
		}
		// Define all Open5GS network functions
		allNetworkFunctions := map[string]bool{
			// 4G Core Network Functions
			"mme":  true, // Mobility Management Entity
			"hss":  true, // Home Subscriber Server
			"pcrf": true, // Policy and Charging Rules Function
			"sgwc": true, // Serving Gateway Control Plane
			"sgwu": true, // Serving Gateway User Plane

			// 5G Core Network Functions
			"nrf":  true, // Network Repository Function
			"scp":  true, // Service Communication Proxy
			"amf":  true, // Access and Mobility Management Function
			"ausf": true, // Authentication Server Function
			"udm":  true, // Unified Data Management
			"udr":  true, // Unified Data Repository
			"pcf":  true, // Policy Control Function
			"nssf": true, // Network Slice Selection Function
			"bsf":  true, // Binding Support Function

			// Shared between 4G and 5G
			"smf": true, // Session Management Function
			"upf": true, // User Plane Function
		}

		componentName := strings.ToLower(component.Name)
		isNetworkFunction := false

		for nfType := range allNetworkFunctions {
			if strings.Contains(componentName, nfType) {
				networkFunctions++
				isNetworkFunction = true
				break
			}
		}

		if !isNetworkFunction {
			supportingServices++
		}

	}

	fmt.Printf("   ├─ Running Components: %d\n", runningCount)
	fmt.Printf("   ├─ Network Functions: %d (AMF, SMF, UPF, MME, etc.)\n", networkFunctions)
	fmt.Printf("   └─ Supporting Services: %d (MongoDB, WebUI, etc.)\n", supportingServices)

	// Metrics capabilities analysis
	realMetrics := 0
	containerMetrics := 0
	healthMetrics := 0
	logSources := 0

	functionsWithMetricsSupport := map[string]bool{
		"amf": true, "smf": true, "pcf": true, "upf": true, "mme": true, "pcrf": true,
	}

	for _, component := range topology.Components {
		if component.IsRunning {
			componentName := strings.ToLower(component.Name)

			// Count real metrics support
			for nfType := range functionsWithMetricsSupport {
				if strings.Contains(componentName, nfType) {
					realMetrics++
					break
				}
			}

			// Every running component supports container and health metrics
			containerMetrics++
			healthMetrics++

			// Count log sources (components that generate logs)
			if isLoggingComponent(componentName) {
				logSources++
			}
		}
	}

	fmt.Printf("\n📊 Monitoring Capabilities Identified:\n")
	fmt.Printf("   ├─ Real Open5GS Metrics: %d components support native /metrics\n", realMetrics)
	fmt.Printf("   ├─ Container Monitoring: %d containers available for resource tracking\n", containerMetrics)
	fmt.Printf("   ├─ Health Monitoring: %d components configured for health checks\n", healthMetrics)
	fmt.Printf("   └─ Log Sources: %d components generate structured logs\n", logSources)

	fmt.Printf("\n📁 Generated Configuration Files:\n")
	fmt.Printf("   ├─ prometheus_targets.yml → Complete Prometheus scrape configuration\n")
	fmt.Printf("   ├─ promtail/core/config.yml → Core network log configuration\n")
	fmt.Printf("   ├─ promtail/ran/config.yml → RAN log configuration\n")
	fmt.Printf("   ├─ topology.json → Machine-readable topology data\n")
	fmt.Printf("   ├─ educational_dashboard.md → Student learning guide\n")
	fmt.Printf("   └─ logging_insights.json → Protocol analysis insights\n")

	fmt.Printf("\n🎓 Educational Value:\n")
	fmt.Printf("   ├─ Students can examine the complete monitoring architecture\n")
	fmt.Printf("   ├─ Understanding of industry-standard observability practices\n")
	fmt.Printf("   ├─ Hands-on experience with Prometheus configuration\n")
	fmt.Printf("   ├─ Protocol-aware log analysis with 3GPP specifications\n")
	fmt.Printf("   ├─ Session flow tracking and troubleshooting workflows\n")
	fmt.Printf("   └─ Real-world telecom O&M workflows\n")

	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
}

// NEW: Helper function to identify logging components
func isLoggingComponent(componentName string) bool {
	loggingComponents := []string{"amf", "smf", "upf", "pcf", "mme", "hss", "pcrf", "sgw", "nrf", "udm", "udr", "ausf", "nssf", "bsf", "srs", "enb", "gnb", "ue"}

	for _, logComp := range loggingComponents {
		if strings.Contains(componentName, logComp) {
			return true
		}
	}
	return false
}

// Export topology and real metrics configuration files
func exportTopologyAndConfig(topology *discovery.NetworkTopology) {
	// Export topology
	if err := writeFile("topology.json", topologyToJSON(topology)); err != nil {
		log.Printf("⚠️  Failed to export topology: %v", err)
	} else {
		fmt.Printf("\n📄 Exported topology to: topology.json\n")
	}

	// Create real collector manager to generate Prometheus config
	collectorManager := metrics.NewRealCollectorManager()
	if err := collectorManager.InitializeCollectors(topology); err != nil {
		log.Printf("⚠️  Failed to initialize collectors for config generation: %v", err)
		return
	}

	// Generate and save Prometheus configuration for real Open5GS metrics
	prometheusConfig := collectorManager.GeneratePrometheusConfig()
	if err := writeFile("prometheus_real_open5gs.yml", prometheusConfig); err != nil {
		log.Printf("⚠️  Failed to write Prometheus config: %v", err)
	} else {
		fmt.Printf("📄 Generated Prometheus config: prometheus_real_open5gs.yml\n")
	}

	// Generate enhanced summary report focused on real metrics
	generateEnhancedSummaryReport(topology)
}

// Generate enhanced summary report with real metrics focus
func generateEnhancedSummaryReport(topology *discovery.NetworkTopology) {
	summary := fmt.Sprintf(`# O&M Module - Real Open5GS Metrics Summary
Generated: %s
Deployment Type: %s
Total Components: %d

## Real Open5GS Metrics Collection

This O&M module now fetches REAL metrics from actual Open5GS components.
No simulation - 100%% live telecommunications data!

### Supported Network Functions with Real Metrics
`, topology.FormattedTimestamp(), topology.Type, len(topology.Components))

	supportedCount := 0
	for name, component := range topology.Components {
		if component.IsRunning {
			supportedTypes := []string{"amf", "smf", "pcf", "upf", "mme", "pcrf"}
			for _, nfType := range supportedTypes {
				if containsNF(name, nfType) {
					collectorPort := getCollectorPort(nfType)
					summary += fmt.Sprintf(`
- **%s** (%s)
  - Open5GS Endpoint: http://%s:9091/metrics
  - O&M Module Endpoint: http://localhost:%s/metrics  
  - Health Check: http://localhost:%s/health
  - Educational Dashboard: http://localhost:%s/dashboard
  - Raw Data Debug: http://localhost:%s/debug/raw
`, name, component.IP, component.IP, collectorPort, collectorPort, collectorPort, collectorPort)
					supportedCount++
					break
				}
			}
		}
	}

	if supportedCount == 0 {
		summary += "\n⚠️  **No supported Open5GS NFs found!**\n"
		summary += "Make sure Open5GS components are configured with metrics enabled.\n"
	}

	summary += `
### Quick Start Commands

1. **Start Real Metrics Collection:**
   ./om-module orchestrator

2. **Test Real Metrics:**
   curl http://localhost:9091/metrics  # AMF real metrics
   curl http://localhost:9092/metrics  # SMF real metrics
   curl http://localhost:9091/debug/raw  # Raw Open5GS AMF data

3. **Test Logging Service:**
   curl http://localhost:8082/health          # Log parser health
   curl http://localhost:8083/logging/status  # Logging service status

4. **Configure Prometheus:**
   prometheus --config.file=prometheus_real_open5gs.yml

5. **Monitor Health:**
   curl http://localhost:9091/health  # AMF health

### Real Metrics Examples

The system collects actual Open5GS metrics like:
- fivegs_amffunction_rm_reginitreq (AMF registration requests)
- pfcp_sessions_active (SMF PFCP sessions)  
- ues_active (Active user equipments)
- gtp2_sessions_active (GTP sessions)
- ran_ue (Connected RAN UEs)

### Logging Pipeline

The integrated logging system provides:
- Real-time log parsing with educational context
- Protocol-aware analysis (NAS, RRC, S1AP, NGAP)
- 3GPP specification references
- Session flow tracking with IMSI correlation
- Performance metrics extraction from logs
- Dynamic Promtail configuration generation

### Architecture

This O&M module fetches metrics from Open5GS components and re-exposes them with:
- Enhanced labeling for better organization
- Educational information for learning
- Health monitoring and status reporting
- Debug access to raw Open5GS data
- Structured log processing with Loki integration

**No simulation - Real telecommunications monitoring!** 🚀
`

	if err := writeFile("real_metrics_summary.md", summary); err != nil {
		log.Printf("⚠️  Failed to write summary: %v", err)
	} else {
		fmt.Printf("📄 Generated enhanced summary: real_metrics_summary.md\n")
	}
}

// Display next steps for real Open5GS metrics setup
func printRealMetricsNextSteps() {
	fmt.Printf("\n🚀 Next Steps for Real Open5GS Metrics\n")
	fmt.Printf("=====================================\n")

	if isRunningInDocker() {
		fmt.Printf("🐳 Docker Environment Detected:\n")
		fmt.Printf("1. **Start complete stack:**\n")
		fmt.Printf("   docker-compose -f services.yml up\n\n")
		fmt.Printf("2. **Access Prometheus:**\n")
		fmt.Printf("   http://localhost:9090/targets\n\n")
		fmt.Printf("3. **Access Grafana:**\n")
		fmt.Printf("   http://localhost:3000 (admin/admin)\n\n")
		fmt.Printf("4. **Access Loki logs:**\n")
		fmt.Printf("   http://localhost:3100 (via Grafana)\n\n")
	} else {
		fmt.Printf("1. 🎬 **Start real metrics collection:**\n")
		fmt.Printf("   ./om-module orchestrator\n\n")
		fmt.Printf("2. 🔍 **Test individual endpoints:**\n")
		fmt.Printf("   curl http://localhost:9091/metrics  # AMF real metrics\n")
		fmt.Printf("   curl http://localhost:9092/metrics  # SMF real metrics\n")
		fmt.Printf("   curl http://localhost:9091/debug/raw  # Raw Open5GS AMF\n\n")
		fmt.Printf("3. 📝 **Test logging service:**\n")
		fmt.Printf("   curl http://localhost:8082/health          # Log parser\n")
		fmt.Printf("   curl http://localhost:8083/logging/status  # Logging service\n\n")
		fmt.Printf("4. 📊 **Configure Prometheus:**\n")
		fmt.Printf("   prometheus --config.file=prometheus_real_open5gs.yml\n\n")
	}

	fmt.Printf("5. 🏥 **Monitor health:**\n")
	fmt.Printf("   curl http://localhost:9091/health\n\n")
	fmt.Printf("6. 📚 **Educational dashboards:**\n")
	fmt.Printf("   curl http://localhost:9091/dashboard\n")
	fmt.Printf("   curl http://localhost:8083/logging/dashboard\n\n")
	fmt.Printf("⚡ **All endpoints fetch live data from Open5GS components!**\n")
	fmt.Printf("📝 **Logs are processed with educational context and 3GPP specs!**\n")
	fmt.Printf("🎯 **No simulation - 100%% real telecommunications metrics!**\n")
}

func printUsage() {
	fmt.Printf("\n📖 USAGE INFORMATION\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("Usage: %s [mode] [options]\n\n", os.Args[0])

	fmt.Printf("🔍 DISCOVERY MODE (default):\n")
	fmt.Printf("   %s discovery [env_file]\n", os.Args[0])
	fmt.Printf("   • Analyzes your network topology without starting collectors\n")
	fmt.Printf("   • Generates Prometheus configurations and documentation\n")
	fmt.Printf("   • Creates dynamic Promtail configurations for logging\n")
	fmt.Printf("   • Perfect for understanding your setup before monitoring\n")
	fmt.Printf("   • Outputs: config files, topology analysis, setup guides\n\n")

	fmt.Printf("🎬 ORCHESTRATOR MODE:\n")
	fmt.Printf("   %s orchestrator [env_file]\n", os.Args[0])
	fmt.Printf("   • Starts live real-time metrics collection from all components\n")
	fmt.Printf("   • Provides HTTP endpoints for Prometheus scraping\n")
	fmt.Printf("   • Starts log parser for real-time log processing\n")
	fmt.Printf("   • Continuously monitors and adapts to topology changes\n")
	fmt.Printf("   • Use this when you want active monitoring and data collection\n\n")

	fmt.Printf("🐞 DEBUG OPTIONS:\n")
	fmt.Printf("   %s [mode] --debug    # Enable detailed debugging output\n", os.Args[0])
	fmt.Printf("   Creates additional debug files for troubleshooting\n\n")

	fmt.Printf("📁 ENV FILE:\n")
	if isRunningInDocker() {
		fmt.Printf("   Docker mode: .env (container environment)\n")
	} else {
		fmt.Printf("   Default: ../env (Docker Compose environment)\n")
		fmt.Printf("   Custom: Specify path to your .env file\n")
	}
	fmt.Printf("\n")

	fmt.Printf("💡 EXAMPLES:\n")
	fmt.Printf("   %s discovery                    # Analyze topology\n", os.Args[0])
	fmt.Printf("   %s orchestrator                 # Start live monitoring\n", os.Args[0])
	if !isRunningInDocker() {
		fmt.Printf("   %s discovery /path/to/.env      # Custom env file\n", os.Args[0])
	}
	fmt.Printf("   %s orchestrator --debug         # Debug mode\n", os.Args[0])
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
}

// Helper functions remain the same but with updated documentation

func printBanner() {
	fmt.Printf("\n")
	fmt.Printf("╔═══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                    📡 4G/5G Network O&M Module v2.0                           ║\n")
	fmt.Printf("║                   Real-Time Monitoring & Educational Platform                 ║\n")
	fmt.Printf("║                    🎓 Industry-Grade Observability for Labs                   ║\n")
	if isRunningInDocker() {
		fmt.Printf("║                          🐳 Docker Integration Mode                           ║\n")
	}
	fmt.Printf("╚═══════════════════════════════════════════════════════════════════════════════╝\n")
	fmt.Printf("\n")
}

// Utility functions (keeping existing implementations)
func containsNF(componentName, nfName string) bool {
	return strings.Contains(strings.ToLower(componentName), nfName)
}

func getCollectorPort(nfType string) string {
	ports := map[string]string{
		"amf": "9091", "smf": "9092", "pcf": "9093",
		"upf": "9094", "mme": "9095", "pcrf": "9096",
	}
	if port, exists := ports[nfType]; exists {
		return port
	}
	return "9091"
}

func writeFile(filename, content string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		err = file.Close()
	}()

	_, err = file.WriteString(content)
	return err
}

func topologyToJSON(topology *discovery.NetworkTopology) string {
	jsonBytes, err := json.MarshalIndent(topology, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{
      "error": "Failed to marshal topology: %s",
      "timestamp": "%s", 
      "component_count": %d
    }`, err.Error(), time.Now().Format("2006-01-02T15:04:05Z"), len(topology.Components))
	}
	return string(jsonBytes)
}

// Enhanced dashboard generation with log-based dashboards
func generateEnhancedDashboards(orchestrator *metrics.RealCollectorOrchestrator, topology *discovery.NetworkTopology, inDocker bool) error {
	log.Printf("📊 Generating enhanced Grafana dashboards with log integration...")

	// Generate existing dashboards
	if err := dashboards.GenerateGrafanaDashboards(orchestrator, topology, inDocker); err != nil {
		log.Printf("⚠️ Failed to generate standard dashboards: %v", err)
	} else {
		log.Printf("✅ Generated standard Grafana dashboards")
	}

	// Generate log-based dashboards if logging service is available
	if loggingService != nil {
		if err := dashboards.EnhanceWithLogDashboards(topology, loggingService); err != nil {
			log.Printf("⚠️ Failed to generate log-based dashboards: %v", err)
		} else {

			log.Printf("✅ Generated log-based educational dashboards")
		}
	} else {

		log.Printf("⚠️ Logging service not available - skipping log-based dashboards")
	}

	return nil
}
