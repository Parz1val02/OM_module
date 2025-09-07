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
	// Parse command line arguments
	var envFile, mode string
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if len(os.Args) > 2 {
		envFile = os.Args[2]
	} else {
		envFile = "../.env" // Default path
	}

	// Check if env file exists
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		log.Printf("⚠️  Environment file %s not found, using defaults", envFile)
	}

	// Initialize auto-discovery service
	discoveryService, err := discovery.NewAutoDiscoveryService(envFile)
	if err != nil {
		log.Fatalf("❌ Failed to initialize discovery service: %v", err)
	}
	defer func() {
		if closeErr := discoveryService.Close(); closeErr != nil {
			log.Printf("Warning: Failed to close discovery service: %v", closeErr)
		}
	}()

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Printf("📝 Received shutdown signal, stopping...")
		cancel()
	}()

	// Check mode
	switch mode {
	case "orchestrator", "metrics":
		runMetricsOrchestrator(ctx, discoveryService)
	case "discovery", "":
		runDiscoveryMode(ctx, discoveryService)
	default:
		log.Printf("❌ Unknown mode: %s", mode)
		printUsage()
		os.Exit(1)
	}
}

// runMetricsOrchestrator runs the continuous metrics orchestration mode
func runMetricsOrchestrator(ctx context.Context, discoveryService *discovery.AutoDiscoveryService) {
	printBanner()
	fmt.Printf("🔄 Starting O&M Module in Enhanced Metrics Orchestrator mode...\n\n")

	// Initialize UPDATED metrics orchestrator (THIS IS THE KEY CHANGE)
	prometheusConfigPath := "./metrics/prometheus.yml"
	orchestrator, err := metrics.NewUpdatedMetricsOrchestrator(discoveryService, prometheusConfigPath)
	if err != nil {
		log.Fatalf("❌ Failed to initialize updated metrics orchestrator: %v", err)
	}

	// Initial discovery to show current state
	fmt.Printf("🔍 Performing initial topology discovery...\n")
	topology, err := discoveryService.DiscoverTopology(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to discover topology: %v", err)
	}

	displayTopologyResults(topology)

	// Show initial health status
	fmt.Printf("🏥 Checking component health...\n")
	healthStatus, err := discoveryService.GetHealthStatus(ctx)
	if err != nil {
		log.Printf("⚠️  Failed to get health status: %v", err)
	} else {
		displayHealthStatus(healthStatus)
	}

	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	fmt.Printf("🚀 Starting enhanced metrics orchestration with official collectors...\n")
	fmt.Printf("📊 Monitoring topology changes and updating Prometheus configuration\n")
	fmt.Printf("📈 Container metrics server will start on port 8080\n")
	fmt.Printf("🏥 Health check server will start on port 8081\n")

	// NEW: Show official collector information
	fmt.Printf("🔧 Official Network Function Collectors:\n")
	fmt.Printf("   📡 AMF Collector:  http://localhost:9091/metrics\n")
	fmt.Printf("   📡 SMF Collector:  http://localhost:9092/metrics\n")
	fmt.Printf("   📡 PCF Collector:  http://localhost:9093/metrics\n")
	fmt.Printf("   📡 UPF Collector:  http://localhost:9094/metrics\n")
	fmt.Printf("   📡 MME Collector:  http://localhost:9095/metrics\n")
	fmt.Printf("   📡 PCRF Collector: http://localhost:9096/metrics\n")

	fmt.Printf("🔄 Press Ctrl+C to stop\n")
	fmt.Printf("%s\n\n", strings.Repeat("=", 80))

	// Start orchestrator (this will run continuously until context is cancelled)
	if err := orchestrator.Start(ctx); err != nil && err != context.Canceled {
		log.Fatalf("❌ Enhanced Orchestrator failed: %v", err)
	}

	fmt.Printf("\n✅ Enhanced Metrics Orchestrator stopped gracefully\n")
}

// runDiscoveryMode runs the one-time discovery mode (original behavior) - UNCHANGED
func runDiscoveryMode(ctx context.Context, discoveryService *discovery.AutoDiscoveryService) {
	// Create context with timeout for operations
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Display banner
	printBanner()

	// Perform discovery with error handling
	fmt.Printf("🔍 Discovering network topology...\n")
	topology, err := discoveryService.DiscoverTopology(timeoutCtx)
	if err != nil {
		log.Fatalf("❌ Failed to discover topology: %v", err)
	}

	// Display discovery results
	displayTopologyResults(topology)

	// Display health status
	fmt.Printf("🏥 Checking component health...\n")
	healthStatus, err := discoveryService.GetHealthStatus(timeoutCtx)
	if err != nil {
		log.Printf("⚠️  Failed to get health status: %v", err)
	} else {
		displayHealthStatus(healthStatus)
	}

	// List active components
	fmt.Printf("📋 Active components:\n")
	activeComponents, err := discoveryService.ListActiveComponents(timeoutCtx)
	if err != nil {
		log.Printf("⚠️  Failed to list active components: %v", err)
	} else {
		for i, component := range activeComponents {
			fmt.Printf("   %d. %s\n", i+1, component)
		}
	}

	// Display educational insights
	printEducationalInsights(topology)

	// Export topology data
	exportTopologyData(topology)

	// NEW: Display enhanced collector information
	printEnhancedCollectorInfo()

	// Show next steps
	printNextSteps()
}

// NEW: Enhanced collector information display
func printEnhancedCollectorInfo() {
	fmt.Printf("\n🔧 Enhanced O&M Module Features:\n")
	fmt.Printf("================================\n")
	fmt.Printf("📊 Official Network Function Collectors:\n")
	fmt.Printf("   • AMF Metrics:  http://localhost:9091/metrics\n")
	fmt.Printf("   • SMF Metrics:  http://localhost:9092/metrics\n")
	fmt.Printf("   • PCF Metrics:  http://localhost:9093/metrics\n")
	fmt.Printf("   • UPF Metrics:  http://localhost:9094/metrics\n")
	fmt.Printf("   • MME Metrics:  http://localhost:9095/metrics\n")
	fmt.Printf("   • PCRF Metrics: http://localhost:9096/metrics\n")
	fmt.Printf("\n📚 Educational Dashboards:\n")
	fmt.Printf("   • AMF Dashboard:  http://localhost:9091/dashboard\n")
	fmt.Printf("   • SMF Dashboard:  http://localhost:9092/dashboard\n")
	fmt.Printf("   • PCF Dashboard:  http://localhost:9093/dashboard\n")
	fmt.Printf("   • UPF Dashboard:  http://localhost:9094/dashboard\n")
	fmt.Printf("   • MME Dashboard:  http://localhost:9095/dashboard\n")
	fmt.Printf("   • PCRF Dashboard: http://localhost:9096/dashboard\n")
	fmt.Printf("\n🏥 Health Check Endpoints:\n")
	fmt.Printf("   • Component Health: http://localhost:8081/health/status\n")
	fmt.Printf("   • Health Metrics:   http://localhost:8081/health/metrics\n")
	fmt.Printf("\n📈 Container Metrics:\n")
	fmt.Printf("   • Container Stats:  http://localhost:8080/container/metrics\n")
	fmt.Printf("   • Container Health: http://localhost:8080/health\n")
}

// printBanner displays the application banner - UNCHANGED
func printBanner() {
	fmt.Printf("\n")
	fmt.Printf("╔═══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                    📡 5G/4G Core Network O&M Module                          ║\n")
	fmt.Printf("║                         Enhanced with Official Collectors                     ║\n")
	fmt.Printf("╚═══════════════════════════════════════════════════════════════════════════════╝\n")
	fmt.Printf("\n")
}

// UNCHANGED functions (keeping existing implementations)
func displayTopologyResults(topology *discovery.NetworkTopology) {
	fmt.Printf("\n🌐 Network Topology Discovered:\n")
	fmt.Printf("===============================\n")
	fmt.Printf("Deployment Type: %s\n", topology.Type)
	fmt.Printf("Total Components: %d\n", len(topology.Components))
	fmt.Printf("Timestamp: %s\n\n", time.Unix(topology.Timestamp, 0).Format("2006-01-02 15:04:05"))

	// Group components by type for better display
	groupedComponents := groupComponentsByType(topology.Components)

	for componentType, components := range groupedComponents {
		fmt.Printf("📦 %s Components:\n", componentType)
		for _, component := range components {
			status := "🔴 Down"
			if component.IsRunning {
				status = "🟢 Running"
			}
			fmt.Printf("  • %s (%s) - %s [%s]\n", component.Name, component.IP, status, strings.Join(component.Ports, ", "))
		}
		fmt.Println()
	}
}

func displayHealthStatus(healthStatus map[string]string) {
	fmt.Printf("🏥 Component Health Status:\n")
	fmt.Printf("==========================\n")

	healthCounts := make(map[string]int)

	for component, status := range healthStatus {
		emoji := getHealthEmoji(status)
		fmt.Printf("  %s %s: %s\n", emoji, component, status)
		healthCounts[status]++
	}

	fmt.Printf("\n📊 Health Summary:\n")
	for status, count := range healthCounts {
		emoji := getHealthEmoji(status)
		fmt.Printf("  %s %s: %d components\n", emoji, status, count)
	}
	fmt.Println()
}

func groupComponentsByType(components map[string]discovery.Component) map[string][]discovery.Component {
	groups := make(map[string][]discovery.Component)

	for _, component := range components {
		groups[component.Type] = append(groups[component.Type], component)
	}

	return groups
}

func printEducationalInsights(topology *discovery.NetworkTopology) {
	fmt.Printf("🎓 Educational Insights:\n")
	fmt.Printf("========================\n")

	switch topology.Type {
	case discovery.TYPE_4G:
		fmt.Printf("📚 This is a 4G EPC (Evolved Packet Core) deployment\n")
		fmt.Printf("   • Key components: MME (Mobility Management), HSS (Subscriber Database)\n")
		fmt.Printf("   • Architecture: Control and User plane separated (CUPS)\n")
		fmt.Printf("   • Interfaces: S1, S6a, S11, S5/S8, SGi\n")
		fmt.Printf("   • NEW: Official metrics available for MME, PCRF, SMF, UPF\n")

	case discovery.TYPE_5G:
		fmt.Printf("📚 This is a 5G Core (5GC) Service Based Architecture deployment\n")
		fmt.Printf("   • Key components: AMF (Access & Mobility), SMF (Session Management)\n")
		fmt.Printf("   • Architecture: Microservices with Service Based Interfaces (SBI)\n")
		fmt.Printf("   • Interfaces: N1, N2, N3, N4, N6, Nnrf, Namf, Nsmf\n")
		fmt.Printf("   • NEW: Official metrics available for AMF, SMF, PCF, UPF\n")

	case discovery.TYPE_MIXED:
		fmt.Printf("📚 This is a Mixed 4G/5G deployment (Non-Standalone)\n")
		fmt.Printf("   • Hybrid architecture with both EPC and 5GC components\n")
		fmt.Printf("   • Enables migration scenarios and interoperability testing\n")
		fmt.Printf("   • NEW: Full metrics coverage for both 4G and 5G components\n")
	}

	fmt.Printf("\n🔧 Enhanced O&M Capabilities:\n")
	fmt.Printf("   • Real-time KPI monitoring\n")
	fmt.Printf("   • Industry-standard metrics exposition\n")
	fmt.Printf("   • Educational dashboards with explanations\n")
	fmt.Printf("   • Health checking and alerting\n")
	fmt.Printf("   • Performance analysis tools\n")
	fmt.Println()
}

func exportTopologyData(topology *discovery.NetworkTopology) {
	// Export as JSON
	jsonData, err := json.MarshalIndent(topology, "", "  ")
	if err != nil {
		log.Printf("⚠️  Failed to marshal topology to JSON: %v", err)
	} else {
		if err := os.WriteFile("topology.json", jsonData, 0644); err != nil {
			log.Printf("⚠️  Failed to write topology.json: %v", err)
		} else {
			fmt.Printf("📄 Exported topology data to topology.json\n")
		}
	}
}

func getHealthEmoji(status string) string {
	switch strings.ToLower(status) {
	case "healthy", "up", "running":
		return "🟢"
	case "unhealthy", "down", "stopped":
		return "🔴"
	case "degraded", "warning":
		return "🟡"
	default:
		return "⚪"
	}
}

func printUsage() {
	fmt.Printf("Usage: %s [mode] [env_file]\n", os.Args[0])
	fmt.Printf("Modes:\n")
	fmt.Printf("  discovery    - One-time discovery (default)\n")
	fmt.Printf("  orchestrator - Continuous metrics orchestration\n")
	fmt.Printf("  metrics      - Alias for orchestrator\n")
	fmt.Printf("\nEnv file: Path to .env file (default: ../.env)\n")
}

// UPDATED: Enhanced next steps with new collector information
func printNextSteps() {
	fmt.Printf("\n🚀 Next Steps:\n")
	fmt.Printf("==============\n")
	fmt.Printf("1. 🔄 Run continuous monitoring: %s orchestrator\n", os.Args[0])
	fmt.Printf("\n2. 📊 Access enhanced metrics:\n")
	fmt.Printf("   • Official NF metrics: http://localhost:909[1-6]/metrics\n")
	fmt.Printf("   • Container metrics:    http://localhost:8080/container/metrics\n")
	fmt.Printf("   • Health check metrics: http://localhost:8081/health/metrics\n")
	fmt.Printf("\n3. 📚 Educational dashboards:\n")
	fmt.Printf("   • NF dashboards: http://localhost:909[1-6]/dashboard\n")
	fmt.Printf("   • Component health: http://localhost:8081/health/status\n")
	fmt.Printf("\n4. 📈 Start Prometheus monitoring:\n")
	fmt.Printf("   • Use generated prometheus.yml configuration\n")
	fmt.Printf("   • Check Prometheus: http://<DOCKER_HOST_IP>:9090\n")
	fmt.Printf("\n5. 🔧 Test all endpoints:\n")
	fmt.Printf("   • Run: curl http://localhost:9091/health (AMF)\n")
	fmt.Printf("   • Run: curl http://localhost:9092/metrics (SMF)\n")
	fmt.Printf("   • Run: curl http://localhost:8080/container/metrics\n")
	fmt.Printf("\n6. 🔄 Monitor in real-time: %s orchestrator\n", os.Args[0])
	fmt.Printf("7. 📝 Monitor logs: docker-compose logs -f <component_name>\n")
}
