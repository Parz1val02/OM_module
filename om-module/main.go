package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
)

func main() {
	// Initialize auto-discovery service
	envFile := "../.env" // Default path - same directory
	if len(os.Args) > 1 {
		envFile = os.Args[1]
	}

	// Check if env file exists
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		log.Printf("⚠️  Environment file %s not found, using defaults", envFile)
	}

	discoveryService, err := discovery.NewAutoDiscoveryService(envFile)
	if err != nil {
		log.Fatalf("❌ Failed to initialize discovery service: %v", err)
	}
	defer func() {
		if closeErr := discoveryService.Close(); closeErr != nil {
			log.Printf("Warning: Failed to close discovery service: %v", closeErr)
		}
	}()

	// Create context with timeout for operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Display banner
	printBanner()

	// Perform discovery with error handling
	fmt.Printf("🔍 Discovering network topology...\n")
	topology, err := discoveryService.DiscoverTopology(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to discover topology: %v", err)
	}

	// Display discovery results
	displayTopologyResults(topology)

	// Display health status
	fmt.Printf("🏥 Checking component health...\n")
	healthStatus, err := discoveryService.GetHealthStatus(ctx)
	if err != nil {
		log.Printf("⚠️  Failed to get health status: %v", err)
	} else {
		displayHealthStatus(healthStatus)
	}

	// List active components
	fmt.Printf("📋 Active components:\n")
	activeComponents, err := discoveryService.ListActiveComponents(ctx)
	if err != nil {
		log.Printf("⚠️  Failed to list active components: %v", err)
	} else {
		for i, component := range activeComponents {
			fmt.Printf("   %d. %s\n", i+1, component)
		}
	}

	// Educational insights
	printEducationalInsights(topology)

	// Save topology to files
	saveTopologyFiles(topology)

	// Display next steps
	printNextSteps()
}

// printBanner displays the application banner
func printBanner() {
	fmt.Printf(`
╔════════════════════════════════════════════════════════════════╗
║                    🔧 O&M Module Discovery Tool                ║
║                   4G/5G Network Topology Scanner               ║
╚════════════════════════════════════════════════════════════════╝

`)
}

// displayTopologyResults shows the discovered topology
func displayTopologyResults(topology *discovery.NetworkTopology) {
	fmt.Printf("📡 Network Topology Discovery Results\n")
	fmt.Printf("=====================================\n\n")

	fmt.Printf("📊 Deployment Type: %s\n", topology.Type)
	fmt.Printf("🔧 Components Found: %d\n", len(topology.Components))
	fmt.Printf("⚙️  Environment Variables: %d\n\n", len(topology.Environment))

	// Display components grouped by type
	componentsByType := groupComponentsByType(topology.Components)

	for componentType, components := range componentsByType {
		fmt.Printf("📦 %s Components:\n", componentType)
		for _, component := range components {
			status := getStatusEmoji(component.IsRunning)
			fmt.Printf("  ├─ %s [%s] - IP: %s %s\n",
				component.Name, component.Image, component.IP, status)
			if len(component.Ports) > 0 {
				fmt.Printf("     └─ Ports: %v\n", component.Ports)
			}
		}
		fmt.Println()
	}
}

// displayHealthStatus shows the health status of components
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

// groupComponentsByType groups components by their type
func groupComponentsByType(components map[string]discovery.Component) map[string][]discovery.Component {
	groups := make(map[string][]discovery.Component)

	for _, component := range components {
		groups[component.Type] = append(groups[component.Type], component)
	}

	return groups
}

// printEducationalInsights provides educational information about the topology
func printEducationalInsights(topology *discovery.NetworkTopology) {
	fmt.Printf("🎓 Educational Insights:\n")
	fmt.Printf("========================\n")

	switch topology.Type {
	case discovery.TYPE_4G:
		fmt.Printf("📚 This is a 4G EPC (Evolved Packet Core) deployment\n")
		fmt.Printf("   • Key components: MME (Mobility Management), HSS (Subscriber Database)\n")
		fmt.Printf("   • Architecture: Control and User plane separated (CUPS)\n")
		fmt.Printf("   • Interfaces: S1, S6a, S11, S5/S8, SGi\n")

	case discovery.TYPE_5G:
		fmt.Printf("📚 This is a 5G Core (5GC) Service Based Architecture deployment\n")
		fmt.Printf("   • Key components: AMF (Access & Mobility), SMF (Session Management)\n")
		fmt.Printf("   • Architecture: Microservices with Service Based Interfaces (SBI)\n")
		fmt.Printf("   • Interfaces: N1, N2, N3, N4, N6, Nnrf, Namf, Nsmf\n")

	case discovery.TYPE_MIXED:
		fmt.Printf("📚 This is a MIXED 4G/5G deployment\n")
		fmt.Printf("   • Shows evolution from EPC to 5GC architecture\n")
		fmt.Printf("   • Useful for migration and interworking scenarios\n")
		fmt.Printf("   • Demonstrates NSA (Non-Standalone) 5G deployment\n")
	}

	// Count running vs stopped components
	running := 0
	total := len(topology.Components)
	for _, component := range topology.Components {
		if component.IsRunning {
			running++
		}
	}

	healthPercentage := float64(running) / float64(total) * 100
	fmt.Printf("\n📊 System Health: %d/%d components running (%.1f%%)\n",
		running, total, healthPercentage)

	if running < total {
		fmt.Printf("⚠️  Some components are not running - check your deployment\n")
		fmt.Printf("   Try: docker-compose logs <component_name> to debug\n")
	} else if running == total {
		fmt.Printf("✅ All components are healthy - ready for testing!\n")
	}

	fmt.Println()
}

// saveTopologyFiles saves the topology to various formats
func saveTopologyFiles(topology *discovery.NetworkTopology) {
	// Save detailed JSON
	if err := saveTopologyToJSON(topology, "topology.json"); err != nil {
		log.Printf("⚠️  Failed to save topology.json: %v", err)
	} else {
		fmt.Printf("💾 Detailed topology saved to topology.json\n")
	}

	// Save simplified summary
	if err := saveTopologySummary(topology, "topology_summary.txt"); err != nil {
		log.Printf("⚠️  Failed to save topology summary: %v", err)
	} else {
		fmt.Printf("📄 Topology summary saved to topology_summary.txt\n")
	}

	// Save Prometheus targets (for monitoring setup)
	if err := savePrometheusTargets(topology, "prometheus_targets.yml"); err != nil {
		log.Printf("⚠️  Failed to save Prometheus targets: %v", err)
	} else {
		fmt.Printf("🎯 Prometheus targets saved to prometheus_targets.yml\n")
	}
}

// saveTopologyToJSON saves the topology to a JSON file
func saveTopologyToJSON(topology *discovery.NetworkTopology, filename string) error {
	data, err := json.MarshalIndent(topology, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal topology: %w", err)
	}

	return os.WriteFile(filename, data, 0644)
}

// saveTopologySummary saves a human-readable summary
func saveTopologySummary(topology *discovery.NetworkTopology, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {

		err = file.Close()

	}()

	_, err = fmt.Fprintf(file, "Network Topology Summary\n")
	_, err = fmt.Fprintf(file, "========================\n\n")
	_, err = fmt.Fprintf(file, "Deployment Type: %s\n", topology.Type)
	_, err = fmt.Fprintf(file, "Components: %d\n\n", len(topology.Components))

	for name, component := range topology.Components {
		status := "STOPPED"
		if component.IsRunning {
			status = "RUNNING"
		}
		_, err = fmt.Fprintf(file, "- %s (%s): %s - IP: %s [%s]\n",
			name, component.Type, status, component.IP, component.Image)
	}

	return nil
}

// savePrometheusTargets saves Prometheus scrape targets configuration
func savePrometheusTargets(topology *discovery.NetworkTopology, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}

	defer func() {

		err = file.Close()

	}()

	_, err = fmt.Fprintf(file, "# Auto-generated Prometheus targets\n")
	_, err = fmt.Fprintf(file, "# Generated by O&M Module Discovery Tool\n\n")

	// Group by component types that expose metrics
	metricsComponents := []string{"amf", "smf", "pcf", "upf", "mme", "pcrf"}

	for _, component := range topology.Components {
		for _, metricsComp := range metricsComponents {
			if strings.Contains(strings.ToLower(component.Name), metricsComp) && component.IsRunning {
				_, err = fmt.Fprintf(file, "- job_name: '%s'\n", component.Name)
				_, err = fmt.Fprintf(file, "  static_configs:\n")
				_, err = fmt.Fprintf(file, "    - targets: ['%s:9091']\n", component.IP)
				_, err = fmt.Fprintf(file, "      labels:\n")
				_, err = fmt.Fprintf(file, "        component: '%s'\n", component.Name)
				_, err = fmt.Fprintf(file, "        type: '%s'\n\n", component.Type)
			}
		}
	}

	return nil
}

// printNextSteps shows what users can do next
func printNextSteps() {
	fmt.Printf("🚀 Next Steps:\n")
	fmt.Printf("==============\n")
	fmt.Printf("1. 📊 View Grafana Dashboard: http://<DOCKER_HOST_IP>:3000\n")
	fmt.Printf("   Username: open5gs, Password: open5gs\n\n")
	fmt.Printf("2. 🌐 Access WebUI: http://<DOCKER_HOST_IP>:9999\n")
	fmt.Printf("   Username: admin, Password: 1423\n\n")
	fmt.Printf("3. 📈 Check Prometheus: http://<DOCKER_HOST_IP>:9090\n\n")
	fmt.Printf("4. 🔧 Use generated files:\n")
	fmt.Printf("   • topology.json - Full topology data\n")
	fmt.Printf("   • topology_summary.txt - Human-readable summary\n")
	fmt.Printf("   • prometheus_targets.yml - Monitoring configuration\n\n")
	fmt.Printf("5. 📝 Monitor logs: docker-compose logs -f <component_name>\n")
	fmt.Printf("6. 🔄 Re-run discovery: ./om-module [path/to/.env]\n\n")
}

// getStatusEmoji returns appropriate emoji for running status
func getStatusEmoji(isRunning bool) string {
	if isRunning {
		return "🟢 RUNNING"
	}
	return "🔴 STOPPED"
}

// getHealthEmoji returns appropriate emoji for health status
func getHealthEmoji(status string) string {
	switch status {
	case "healthy":
		return "🟢"
	case "failed":
		return "🔴"
	case "recovering":
		return "🟡"
	default:
		return "⚫"
	}
}
