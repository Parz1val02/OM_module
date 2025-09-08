package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
	"github.com/Parz1val02/OM_module/metrics"
)

func main() {
	log.Printf("🚀 Starting O&M Module with Real Open5GS Metrics Collection")

	// Parse command line arguments
	mode := "discovery"
	envFile := "../.env"

	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if len(os.Args) > 2 {
		envFile = os.Args[2]
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

// NEW: Real metrics orchestrator mode
func runRealMetricsOrchestrator(envFile string) {
	printBanner()
	log.Printf("🎬 Starting Real Open5GS Metrics Orchestrator mode")

	// Create discovery service
	discoveryService, err := discovery.NewAutoDiscoveryService(envFile)
	if err != nil {
		log.Fatalf("❌ Failed to create discovery service: %v", err)
	}
	defer func() {
		err = discoveryService.Close()
	}()

	// Create real collector orchestrator
	orchestrator := metrics.NewRealCollectorOrchestrator(discoveryService)

	// Start the orchestrator
	if err := orchestrator.Start(); err != nil {
		log.Fatalf("❌ Failed to start real metrics orchestrator: %v", err)
	}

	// Wait for collectors to be healthy
	log.Printf("⏳ Waiting for collectors to be healthy...")
	if err := orchestrator.WaitForHealthy(30 * time.Second); err != nil {
		log.Printf("⚠️  Warning: %v", err)
	}

	// Display status
	displayRealMetricsStatus(orchestrator)

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("⚡ Press Ctrl+C to stop...")

	// Wait for shutdown signal
	<-sigChan
	log.Printf("🛑 Received shutdown signal, stopping...")

	// Stop orchestrator
	orchestrator.Stop()

	log.Printf("✅ Real Open5GS metrics orchestrator stopped")
}

// NEW: Display real metrics status
func displayRealMetricsStatus(orchestrator *metrics.RealCollectorOrchestrator) {
	endpoints := orchestrator.GetMetricsEndpoints()
	status := orchestrator.GetStatus()

	fmt.Printf("\n🎯 Real Open5GS Metrics Collection Status\n")
	fmt.Printf("==========================================\n")

	if collectors, ok := status["collectors"].(map[string]any); ok {
		fmt.Printf("📊 Active Collectors: %d\n\n", len(collectors))

		for componentName, info := range collectors {
			if collectorInfo, ok := info.(map[string]any); ok {
				// nf_type
				nfType, ok := collectorInfo["nf_type"].(string)
				if !ok {
					log.Printf("⚠️  Missing or invalid nf_type for collector %s", componentName)
					nfType = "unknown"
				}

				// component_ip
				componentIP, ok := collectorInfo["component_ip"].(string)
				if !ok {
					log.Printf("⚠️  Missing or invalid component_ip for collector %s", componentName)
					componentIP = "unknown"
				}

				// fetch_url
				fetchURL, ok := collectorInfo["fetch_url"].(string)
				if !ok {
					log.Printf("⚠️  Missing or invalid fetch_url for collector %s", componentName)
					fetchURL = ""
				}

				// collector_port (can be int, float64, or string)
				var collectorPort any = "N/A"
				if port, exists := collectorInfo["collector_port"]; exists {
					collectorPort = port
				} else {
					log.Printf("⚠️  Missing collector_port for collector %s", componentName)
				}

				// status
				isHealthy := false
				if healthy, ok := collectorInfo["status"].(bool); ok {
					isHealthy = healthy
				} else {
					log.Printf("⚠️  Missing or invalid status for collector %s", componentName)
				}

				// Health indicator
				healthIcon := "🟢"
				if !isHealthy {
					healthIcon = "🔴"
				}

				// Print collector info
				fmt.Printf("%s %s (%s) %s\n", healthIcon, componentName, strings.ToUpper(nfType), componentIP)
				if fetchURL != "" {
					fmt.Printf("   📡 Fetching from: %s\n", fetchURL)
				}
				fmt.Printf("   📊 Exposing at:   http://localhost:%v/metrics\n", collectorPort)
				fmt.Printf("   🏥 Health check:  http://localhost:%v/health\n", collectorPort)
				fmt.Printf("   📚 Dashboard:     http://localhost:%v/dashboard\n", collectorPort)
				fmt.Printf("   🔍 Raw data:      http://localhost:%v/debug/raw\n", collectorPort)
				fmt.Printf("\n")
			}
		}
	}

	fmt.Printf("🔧 Quick Tests:\n")
	for componentName, endpoint := range endpoints {
		fmt.Printf("   curl %s  # %s metrics\n", endpoint, strings.ToUpper(componentName))
	}
	fmt.Printf("\n")
}

// Enhanced discovery mode
func runDiscoveryMode(envFile string) {
	printBanner()
	log.Printf("🔍 Running in discovery mode")

	// Create discovery service
	discoveryService, err := discovery.NewAutoDiscoveryService(envFile)
	if err != nil {
		log.Fatalf("❌ Failed to create discovery service: %v", err)
	}
	defer func() {
		err = discoveryService.Close()
	}()

	// Discover topology
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	topology, err := discoveryService.DiscoverTopology(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to discover topology: %v", err)
	}

	// Display discovery results with real metrics focus
	printEnhancedDiscoveryResults(topology)

	// Export topology and configuration files
	exportTopologyAndConfig(topology)

	// Display next steps for real metrics
	printRealMetricsNextSteps()
}

// NEW: Enhanced discovery results with real metrics focus
func printEnhancedDiscoveryResults(topology *discovery.NetworkTopology) {
	fmt.Printf("\n🌐 Network Topology Discovery Results\n")
	fmt.Printf("====================================\n")
	fmt.Printf("📋 Deployment Type: %s\n", topology.Type)
	fmt.Printf("🕐 Discovery Time: %s\n", topology.FormattedTimestamp())
	fmt.Printf("📊 Total Components: %d\n\n", len(topology.Components))

	// Categorize components for real metrics
	realMetricsSupported := []string{}
	containerMetricsOnly := []string{}

	supportedNFs := map[string]bool{
		"amf": true, "smf": true, "pcf": true,
		"upf": true, "mme": true, "pcrf": true,
	}

	for name, component := range topology.Components {
		if component.IsRunning {
			hasRealMetrics := false
			for nfType := range supportedNFs {
				if containsNF(name, nfType) {
					realMetricsSupported = append(realMetricsSupported,
						fmt.Sprintf("%s (%s) -> Real Open5GS metrics available",
							name, component.IP))
					hasRealMetrics = true
					break
				}
			}
			if !hasRealMetrics {
				containerMetricsOnly = append(containerMetricsOnly,
					fmt.Sprintf("%s (%s) -> Container metrics only",
						name, component.Type))
			}
		}
	}

	// Display real metrics components
	fmt.Printf("🎯 Components with Real Open5GS Metrics:\n")
	if len(realMetricsSupported) > 0 {
		for i, comp := range realMetricsSupported {
			fmt.Printf("   %d. %s\n", i+1, comp)
		}
	} else {
		fmt.Printf("   ⚠️  No supported Open5GS NFs found!\n")
		fmt.Printf("   💡 Make sure Open5GS components are running and configured for metrics.\n")
	}

	// Display container-only components
	fmt.Printf("\n📦 Other Components (Container Metrics Only):\n")
	if len(containerMetricsOnly) > 0 {
		for i, comp := range containerMetricsOnly {
			fmt.Printf("   %d. %s\n", i+1, comp)
		}
	} else {
		fmt.Printf("   None\n")
	}

	// Show expected real metrics ports
	if len(realMetricsSupported) > 0 {
		fmt.Printf("\n📡 Expected Open5GS Metrics Endpoints:\n")
		for name, component := range topology.Components {
			if component.IsRunning {
				for nfType := range supportedNFs {
					if containsNF(name, nfType) {
						fmt.Printf("   • %s: http://%s:9091/metrics\n", name, component.IP)
						break
					}
				}
			}
		}
	}
}

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

	// Generate and save Prometheus configuration
	prometheusConfig := collectorManager.GeneratePrometheusConfig()
	if err := writeFile("prometheus_real_open5gs.yml", prometheusConfig); err != nil {
		log.Printf("⚠️  Failed to write Prometheus config: %v", err)
	} else {
		fmt.Printf("📄 Generated Prometheus config: prometheus_real_open5gs.yml\n")
	}

	// Generate enhanced summary report
	generateEnhancedSummaryReport(topology)
}

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
	fmt.Printf("Usage: %s [mode] [env_file]\n", os.Args[0])
	fmt.Printf("Modes:\n")
	fmt.Printf("  discovery        - Discover topology and generate configs (default)\n")
	fmt.Printf("  orchestrator     - Start real Open5GS metrics collection\n")
	fmt.Printf("\nEnv file: Path to .env file (default: ../.env)\n")
	fmt.Printf("\nExamples:\n")
	fmt.Printf("  %s discovery          # Discover and generate configs\n", os.Args[0])
	fmt.Printf("  %s orchestrator       # Start real metrics collection\n", os.Args[0])
}

// Helper functions
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
	// Convert topology to JSON string
	// Implementation depends on your discovery package
	jsonBytes, err := json.MarshalIndent(topology, "", "  ")
	if err != nil {
		// Returns a fallback JSON structure with error information
		return fmt.Sprintf(`{
      "error": "Failed to marshal topology: %s",
      "timestamp": "%s", 
      "component_count": %d
    }`, err.Error(), time.Now().Format("2006-01-02T15:04:05Z"), len(topology.Components))
	}
	return string(jsonBytes)
}

func printBanner() {
	fmt.Printf("\n")
	fmt.Printf("╔═══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                    📡 5G/4G Core Network O&M Module                           ║\n")
	fmt.Printf("║                         Enhanced with Official Collectors                     ║\n")
	fmt.Printf("╚═══════════════════════════════════════════════════════════════════════════════╝\n")
	fmt.Printf("\n")
}
