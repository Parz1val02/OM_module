package ui

import (
	"fmt"
	"strings"

	"github.com/Parz1val02/OM_module/discovery"
)

// PrintDiscoveryModeDescription prints clear description of what discovery mode does
func PrintDiscoveryModeDescription() {
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

// DisplayDiscoveryResults displays enhanced results for discovery mode
func DisplayDiscoveryResults(topology *discovery.NetworkTopology) {
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
			if IsLoggingComponent(componentName) {
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

// PrintRealMetricsNextSteps displays next steps for real Open5GS metrics setup
func PrintRealMetricsNextSteps() {
	fmt.Printf("\n🚀 Next Steps for Real Open5GS Metrics\n")
	fmt.Printf("=====================================\n")

	if IsRunningInDocker() {
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

// IsLoggingComponent identifies logging components
func IsLoggingComponent(componentName string) bool {
	loggingComponents := []string{"amf", "smf", "upf", "pcf", "mme", "hss", "pcrf", "sgw", "nrf", "udm", "udr", "ausf", "nssf", "bsf", "srs", "enb", "gnb", "ue"}

	for _, logComp := range loggingComponents {
		if strings.Contains(componentName, logComp) {
			return true
		}
	}
	return false
}
