package ui

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/Parz1val02/OM_module/logging"
	"github.com/Parz1val02/OM_module/metrics"
)

// PrintOrchestratorModeDescription prints clear description of what orchestrator mode does
func PrintOrchestratorModeDescription() {
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
	if IsRunningInDocker() {
		fmt.Printf("🐳 Docker Mode:\n")
		fmt.Printf("   • Automatic Prometheus configuration\n")
		fmt.Printf("   • Docker network integration\n")
		fmt.Printf("   • Shared volume configuration\n\n")
	}
	fmt.Printf("⚠️  Note: Requires running Open5GS containers with metrics enabled\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
}

// DisplayRealMetricsStatus displays enhanced status for orchestrator mode
func DisplayRealMetricsStatus(orchestrator *metrics.RealCollectorOrchestrator, loggingService *logging.LoggingService, debugMode bool) {
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
					// Handle collector_port properly
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

					// Handle metrics_count properly
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

					// Handle status properly
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
					if IsRunningInDocker() {
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
	if IsRunningInDocker() {
		fmt.Printf("   ├─ Docker endpoints: http://om-module:8080 & http://om-module:8081\n")
	}
	fmt.Printf("   ├─ Container Stats  🟢 Active:\n")
	fmt.Printf("   │  ├─ All containers: %s:8080/container/metrics\n", baseURL)
	fmt.Printf("   │  └─ Collector health: %s:8080/health\n", baseURL)
	fmt.Printf("   ├─ Health Checks    🟢 Active:\n")
	fmt.Printf("   │  ├─ All components: %s:8081/health/metrics\n", baseURL)
	fmt.Printf("   │  └─ Collector health: %s:8081/health\n", baseURL)
	fmt.Printf("   └─ System Resources 🟢 Active → Collected every 10s\n\n")

	// Display logging service status
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

	// Logging endpoints
	if loggingService != nil {
		fmt.Printf("   curl %s:8082/health            # Log parser health\n", baseURL)
		fmt.Printf("   curl %s:8083/logging/status    # Logging service status\n", baseURL)
		fmt.Printf("   curl %s:8083/logging/configs   # Generated Promtail configs\n", baseURL)
	}

	if IsRunningInDocker() {
		fmt.Printf("\n🐳 Docker Integration:\n")
		fmt.Printf("   • Prometheus config: /etc/prometheus/configs/prometheus.yml\n")
		fmt.Printf("   • Promtail configs: Auto-generated and mounted\n")
		fmt.Printf("   • Auto-reload: Configuration updated automatically\n")
	}
	fmt.Printf("\n")

	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
}
