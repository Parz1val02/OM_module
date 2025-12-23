package export

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
	"github.com/Parz1val02/OM_module/logging"
	"github.com/Parz1val02/OM_module/metrics"
	"github.com/Parz1val02/OM_module/utils"
)

// WriteFile writes content to a file
func WriteFile(filename, content string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	return err
}

// TopologyToJSON converts topology to JSON string
func TopologyToJSON(topology *discovery.NetworkTopology) string {
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

// ExportTopologyAndConfig exports topology and configuration files
func ExportTopologyAndConfig(topology *discovery.NetworkTopology) {
	// Export topology
	if err := WriteFile("topology.json", TopologyToJSON(topology)); err != nil {
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
	if err := WriteFile("prometheus_real_open5gs.yml", prometheusConfig); err != nil {
		log.Printf("⚠️  Failed to write Prometheus config: %v", err)
	} else {
		fmt.Printf("📄 Generated Prometheus config: prometheus_real_open5gs.yml\n")
	}

	// Generate enhanced summary report focused on real metrics
	GenerateEnhancedSummaryReport(topology)
}

// GenerateEducationalContent generates educational content files
func GenerateEducationalContent(topology *discovery.NetworkTopology) {
	// Generate educational dashboard
	dashboard := logging.GenerateEducationalDashboard(topology)
	if err := WriteFile("educational_dashboard.md", dashboard); err != nil {
		log.Printf("⚠️ Failed to write educational dashboard: %v", err)
	} else {
		log.Printf("📚 Educational dashboard written to: educational_dashboard.md")
	}

	// Write logging insights
	insights := logging.GetEducationalInsights(topology)
	insightsJSON, _ := json.MarshalIndent(insights, "", "  ")
	if err := WriteFile("logging_insights.json", string(insightsJSON)); err != nil {
		log.Printf("⚠️ Failed to write logging insights: %v", err)
	} else {
		log.Printf("💡 Logging insights written to: logging_insights.json")
	}
}

// GenerateEnhancedSummaryReport generates enhanced summary report with real metrics focus
func GenerateEnhancedSummaryReport(topology *discovery.NetworkTopology) {
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
				if utils.ContainsNF(name, nfType) {
					collectorPort := utils.GetCollectorPort(nfType)
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

	if err := WriteFile("real_metrics_summary.md", summary); err != nil {
		log.Printf("⚠️  Failed to write summary: %v", err)
	} else {
		fmt.Printf("📄 Generated enhanced summary: real_metrics_summary.md\n")
	}
}
