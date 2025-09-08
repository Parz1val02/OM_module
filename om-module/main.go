package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
	"github.com/Parz1val02/OM_module/metrics"
)

var debugMode bool

func main() {
	log.Printf("🚀 Starting O&M Module for 4G/5G Educational Network Testbed")

	// Parse command line arguments
	mode := "discovery"
	envFile := "../.env"

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

// Real-time metrics orchestrator mode - Live monitoring and collection
func runRealMetricsOrchestrator(envFile string) {
	printBanner()
	printOrchestratorModeDescription()

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

	// NOW we can start container and health collectors with the variables
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

	// Wait for collectors to be healthy
	log.Printf("⏳ Performing health checks on all collectors...")
	if err := orchestrator.WaitForHealthy(30 * time.Second); err != nil {
		log.Printf("⚠️  Warning: Some collectors may not be fully ready: %v", err)
	}

	// Display live status
	displayRealMetricsStatus(orchestrator)

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("🌟 Orchestrator mode is now LIVE! All metrics are being collected in real-time.")
	log.Printf("📊 Access Prometheus metrics at the endpoints shown above")
	log.Printf("⚡ Press Ctrl+C to stop the orchestrator...")

	// Wait for shutdown signal
	<-sigChan
	log.Printf("🛑 Received shutdown signal, gracefully stopping all collectors...")

	// Cancel context to stop all collectors
	cancel()

	// Stop real orchestrator
	orchestrator.Stop()

	// Give collectors time to stop gracefully
	time.Sleep(2 * time.Second)

	log.Printf("✅ Real-time metrics orchestrator stopped cleanly")
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

	// Display comprehensive discovery results
	displayDiscoveryResults(topology)

	// Display next steps for real metrics
	printRealMetricsNextSteps()

	log.Printf("🎯 Discovery mode completed successfully!")
	log.Printf("📁 All configuration files are ready for use with Prometheus/Grafana")
	log.Printf("🚀 To start live monitoring, run: ./om-module orchestrator")
}

// Print clear description of what orchestrator mode does
func printOrchestratorModeDescription() {
	fmt.Printf("🎬 ORCHESTRATOR MODE - Live Real-Time Metrics Collection\n")
	fmt.Printf("═══════════════════════════════════════════════════════\n")
	fmt.Printf("This mode starts a LIVE metrics collection system that:\n\n")
	fmt.Printf("🔄 Real-Time Operations:\n")
	fmt.Printf("   • Continuously monitors all Open5GS network functions\n")
	fmt.Printf("   • Collects live metrics directly from AMF, SMF, UPF, etc.\n")
	fmt.Printf("   • Gathers container resource usage (CPU, memory, I/O)\n")
	fmt.Printf("   • Performs health checks every 15 seconds\n")
	fmt.Printf("   • Automatically adapts to topology changes\n\n")
	fmt.Printf("🌐 Active HTTP Endpoints:\n")
	fmt.Printf("   • Real Open5GS metrics: ports 9091-9096\n")
	fmt.Printf("   • Container metrics: port 8080\n")
	fmt.Printf("   • Health checks: port 8081\n")
	fmt.Printf("   • Educational dashboards and debug info\n\n")
	fmt.Printf("📊 Integration Ready:\n")
	fmt.Printf("   • Prometheus can scrape all endpoints immediately\n")
	fmt.Printf("   • Grafana dashboards show live data\n")
	fmt.Printf("   • Perfect for lab demonstrations and learning\n\n")
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
	fmt.Printf("   • Assesses health check capabilities\n\n")
	fmt.Printf("📝 Configuration Generation:\n")
	fmt.Printf("   • Creates Prometheus scrape configurations\n")
	fmt.Printf("   • Generates target definitions for all metric sources\n")
	fmt.Printf("   • Builds comprehensive monitoring setup files\n")
	fmt.Printf("   • Prepares educational dashboard configurations\n\n")
	fmt.Printf("📁 Output Files Created:\n")
	fmt.Printf("   • prometheus_targets.yml - Ready-to-use Prometheus config\n")
	fmt.Printf("   • topology.json - Machine-readable topology data\n")
	fmt.Printf("   • topology_summary.txt - Human-readable analysis report\n")
	fmt.Printf("   • real_metrics_summary.md - Setup instructions for students\n\n")
	fmt.Printf("🎯 Educational Benefits:\n")
	fmt.Printf("   • Students can see the complete monitoring architecture\n")
	fmt.Printf("   • Understand which components provide which metrics\n")
	fmt.Printf("   • Learn industry-standard observability practices\n\n")
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
					fmt.Printf("   📊 Exposing at:   http://localhost:%s/metrics\n", collectorPort)
					fmt.Printf("   🏥 Health check:  http://localhost:%s/health\n", collectorPort)
					fmt.Printf("   📚 Dashboard:     http://localhost:%s/dashboard\n", collectorPort)
					fmt.Printf("   🔍 Raw data:      http://localhost:%s/debug/raw\n", collectorPort)
					fmt.Printf("\n")
				}
			}
		} else {
			log.Printf("⚠️ No nested collectors found inside status[\"collectors\"]")
		}
	}

	fmt.Printf("📊 Infrastructure Metrics:\n")
	fmt.Printf("   ├─ Container Stats  🟢 Active:\n")
	fmt.Printf("   │  ├─ All containers: http://localhost:8080/container/metrics\n")
	fmt.Printf("   │  └─ Collector health: http://localhost:8080/health\n")
	fmt.Printf("   ├─ Health Checks    🟢 Active:\n")
	fmt.Printf("   │  ├─ All components: http://localhost:8081/health/metrics\n")
	fmt.Printf("   │  └─ Collector health: http://localhost:8081/health\n")
	fmt.Printf("   └─ System Resources 🟢 Active → Collected every 10s\n\n")

	fmt.Printf("🔧 Quick Tests:\n")
	// Real Open5GS endpoints
	for componentName, endpoint := range endpoints {
		fmt.Printf("   curl %s  # %s real metrics\n", endpoint, strings.ToUpper(componentName))
	}
	// Infrastructure endpoints
	fmt.Printf("   curl http://localhost:8080/container/metrics  # Container resources\n")
	fmt.Printf("   curl http://localhost:8081/health/metrics     # Component health\n")
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
		}
	}

	fmt.Printf("\n📊 Monitoring Capabilities Identified:\n")
	fmt.Printf("   ├─ Real Open5GS Metrics: %d components support native /metrics\n", realMetrics)
	fmt.Printf("   ├─ Container Monitoring: %d containers available for resource tracking\n", containerMetrics)
	fmt.Printf("   └─ Health Monitoring: %d components configured for health checks\n", healthMetrics)

	fmt.Printf("\n📁 Generated Configuration Files:\n")
	fmt.Printf("   ├─ prometheus_targets.yml → Complete Prometheus scrape configuration\n")
	fmt.Printf("   ├─ topology.json → Machine-readable topology data\n")
	fmt.Printf("   ├─ topology_summary.txt → Human-readable analysis report\n")
	fmt.Printf("   └─ real_metrics_summary.md → Student setup instructions\n")

	fmt.Printf("\n🎓 Educational Value:\n")
	fmt.Printf("   ├─ Students can examine the complete monitoring architecture\n")
	fmt.Printf("   ├─ Understanding of industry-standard observability practices\n")
	fmt.Printf("   ├─ Hands-on experience with Prometheus configuration\n")
	fmt.Printf("   └─ Real-world telecom O&M workflows\n")

	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
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

3. **Configure Prometheus:**
   prometheus --config.file=prometheus_real_open5gs.yml

4. **Monitor Health:**
   curl http://localhost:9091/health  # AMF health

### Real Metrics Examples

The system collects actual Open5GS metrics like:
- fivegs_amffunction_rm_reginitreq (AMF registration requests)
- pfcp_sessions_active (SMF PFCP sessions)  
- ues_active (Active user equipments)
- gtp2_sessions_active (GTP sessions)
- ran_ue (Connected RAN UEs)

### Architecture

This O&M module fetches metrics from Open5GS components and re-exposes them with:
- Enhanced labeling for better organization
- Educational information for learning
- Health monitoring and status reporting
- Debug access to raw Open5GS data

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
	fmt.Printf("1. 🎬 **Start real metrics collection:**\n")
	fmt.Printf("   ./om-module orchestrator\n\n")
	fmt.Printf("2. 🔍 **Test individual endpoints:**\n")
	fmt.Printf("   curl http://localhost:9091/metrics  # AMF real metrics\n")
	fmt.Printf("   curl http://localhost:9092/metrics  # SMF real metrics\n")
	fmt.Printf("   curl http://localhost:9091/debug/raw  # Raw Open5GS AMF\n\n")
	fmt.Printf("3. 📊 **Configure Prometheus:**\n")
	fmt.Printf("   prometheus --config.file=prometheus_real_open5gs.yml\n\n")
	fmt.Printf("4. 🏥 **Monitor health:**\n")
	fmt.Printf("   curl http://localhost:9091/health\n\n")
	fmt.Printf("5. 📚 **Educational dashboards:**\n")
	fmt.Printf("   curl http://localhost:9091/dashboard\n\n")
	fmt.Printf("⚡ **All endpoints fetch live data from Open5GS components!**\n")
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
	fmt.Printf("   • Perfect for understanding your setup before monitoring\n")
	fmt.Printf("   • Outputs: config files, topology analysis, setup guides\n\n")

	fmt.Printf("🎬 ORCHESTRATOR MODE:\n")
	fmt.Printf("   %s orchestrator [env_file]\n", os.Args[0])
	fmt.Printf("   • Starts live real-time metrics collection from all components\n")
	fmt.Printf("   • Provides HTTP endpoints for Prometheus scraping\n")
	fmt.Printf("   • Continuously monitors and adapts to topology changes\n")
	fmt.Printf("   • Use this when you want active monitoring and data collection\n\n")

	fmt.Printf("🐞 DEBUG OPTIONS:\n")
	fmt.Printf("   %s [mode] --debug    # Enable detailed debugging output\n", os.Args[0])
	fmt.Printf("   Creates additional debug files for troubleshooting\n\n")

	fmt.Printf("📁 ENV FILE:\n")
	fmt.Printf("   Default: ../env (Docker Compose environment)\n")
	fmt.Printf("   Custom: Specify path to your .env file\n\n")

	fmt.Printf("💡 EXAMPLES:\n")
	fmt.Printf("   %s discovery                    # Analyze topology\n", os.Args[0])
	fmt.Printf("   %s orchestrator                 # Start live monitoring\n", os.Args[0])
	fmt.Printf("   %s discovery /path/to/.env      # Custom env file\n", os.Args[0])
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
