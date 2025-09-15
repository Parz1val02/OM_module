package dashboards

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
	"github.com/Parz1val02/OM_module/metrics"
)

// Enhanced Generate function with metric discovery
func GenerateGrafanaDashboards(orchestrator *metrics.RealCollectorOrchestrator, topology *discovery.NetworkTopology, inDocker bool) error {
	log.Printf("📊 Starting intelligent dashboard generation...")

	// Step 1: Discover available metrics from all endpoints
	log.Printf("🔍 Discovering available metrics from active collectors...")
	availableMetrics, err := discoverAvailableMetrics(orchestrator.GetMetricsEndpoints())
	if err != nil {
		log.Printf("⚠️ Failed to discover metrics, using fallback defaults: %v", err)
		availableMetrics = getDefaultMetricsStructure()
	}

	// Step 2: Log discovered metrics for debugging
	logDiscoveredMetrics(availableMetrics)

	// Step 3: Generate dashboards based on discovered metrics
	dashboards := []Dashboard{
		generateNetworkOverviewDashboard(orchestrator.GetMetricsEndpoints(), topology, availableMetrics),
		generateInfrastructureDashboard(topology, availableMetrics),
		generateHealthDashboard(topology, availableMetrics),
		generateEducationalDashboard(topology, availableMetrics),
	}

	// Step 4: Add component-specific dashboards for components with metrics
	for componentName, componentMetrics := range availableMetrics {
		if componentMetrics.IsAvailable && len(componentMetrics.Metrics) > 0 {
			dashboards = append(dashboards, generateComponentDashboard(componentName, topology, componentMetrics))
		}
	}

	// Step 5: Write dashboard files
	if err := writeDashboardFiles(dashboards, inDocker); err != nil {
		return fmt.Errorf("failed to write dashboard files: %w", err)
	}

	// Step 6: Generate dashboard provisioning config
	if err := generateDashboardProvisioning(inDocker); err != nil {
		return fmt.Errorf("failed to generate dashboard provisioning: %w", err)
	}

	log.Printf("✅ Generated %d Grafana dashboards with real metric discovery", len(dashboards))
	return nil
}

// Discover available metrics from active endpoints
func discoverAvailableMetrics(endpoints map[string]string) (map[string]ComponentMetrics, error) {
	availableMetrics := make(map[string]ComponentMetrics)
	client := &http.Client{Timeout: 10 * time.Second}

	for componentName, endpoint := range endpoints {
		log.Printf("🔍 Fetching metrics from %s (%s)...", componentName, endpoint)

		metrics, err := fetchMetricInfo(client, endpoint, componentName)
		if err != nil {
			log.Printf("⚠️ Failed to fetch metrics from %s: %v", componentName, err)
			availableMetrics[componentName] = ComponentMetrics{
				ComponentName: componentName,
				Metrics:       []MetricInfo{},
				IsAvailable:   false,
				LastUpdated:   time.Now(),
			}
			continue
		}

		availableMetrics[componentName] = ComponentMetrics{
			ComponentName: componentName,
			Metrics:       metrics,
			IsAvailable:   true,
			LastUpdated:   time.Now(),
		}

		log.Printf("✅ Discovered %d metrics from %s", len(metrics), componentName)
	}

	return availableMetrics, nil
}

// Fetch detailed metric information from an endpoint
func fetchMetricInfo(client *http.Client, endpoint, componentName string) ([]MetricInfo, error) {
	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		err = resp.Body.Close()
	}()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	var metrics []MetricInfo
	scanner := bufio.NewScanner(resp.Body)

	var currentMetric MetricInfo

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "# HELP ") {
			// Parse help text: # HELP metric_name description
			parts := strings.SplitN(line, " ", 4)
			if len(parts) >= 4 {
				currentMetric = MetricInfo{
					Name:      parts[2],
					Help:      parts[3],
					Component: componentName,
					Category:  categorizeMetric(parts[2]),
				}
			}
		} else if strings.HasPrefix(line, "# TYPE ") {
			// Parse type: # TYPE metric_name gauge|counter|histogram
			parts := strings.SplitN(line, " ", 4)
			if len(parts) >= 4 && currentMetric.Name == parts[2] {
				currentMetric.Type = parts[3]
			}
		} else if line != "" && !strings.HasPrefix(line, "#") {
			// This is an actual metric line, extract the name
			metricName := strings.Split(strings.Split(line, "{")[0], " ")[0]

			// If we have a current metric being built, add it
			if currentMetric.Name != "" && currentMetric.Name == metricName {
				metrics = append(metrics, currentMetric)
				currentMetric = MetricInfo{} // Reset
			} else if currentMetric.Name == "" {
				// Found a metric without HELP/TYPE, create basic info
				metrics = append(metrics, MetricInfo{
					Name:      metricName,
					Type:      "unknown",
					Help:      "No description available",
					Component: componentName,
					Category:  categorizeMetric(metricName),
				})
			}
		}
	}

	// Add the last metric if it exists
	if currentMetric.Name != "" {
		metrics = append(metrics, currentMetric)
	}

	// Remove duplicates and sort
	metrics = removeDuplicateMetrics(metrics)
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].Name < metrics[j].Name
	})

	return metrics, scanner.Err()
}

// Categorize metrics for better organization
func categorizeMetric(metricName string) string {
	name := strings.ToLower(metricName)

	// Network function specific patterns
	if strings.Contains(name, "session") || strings.Contains(name, "bearer") {
		return "sessions"
	}
	if strings.Contains(name, "ue") || strings.Contains(name, "user") {
		return "users"
	}
	if strings.Contains(name, "request") || strings.Contains(name, "response") {
		return "requests"
	}
	if strings.Contains(name, "gtp") || strings.Contains(name, "pfcp") {
		return "protocols"
	}
	if strings.Contains(name, "cpu") || strings.Contains(name, "memory") || strings.Contains(name, "process") {
		return "system"
	}
	if strings.Contains(name, "health") || strings.Contains(name, "up") {
		return "health"
	}

	return "general"
}

// Remove duplicate metrics
func removeDuplicateMetrics(metrics []MetricInfo) []MetricInfo {
	seen := make(map[string]bool)
	var result []MetricInfo

	for _, metric := range metrics {
		if !seen[metric.Name] {
			seen[metric.Name] = true
			result = append(result, metric)
		}
	}

	return result
}

// Log discovered metrics for debugging
func logDiscoveredMetrics(availableMetrics map[string]ComponentMetrics) {
	log.Printf("📊 Metric Discovery Summary:")

	for componentName, componentMetrics := range availableMetrics {
		if componentMetrics.IsAvailable {
			log.Printf("  ✅ %s: %d metrics discovered", componentName, len(componentMetrics.Metrics))

			// Group by category
			categories := make(map[string]int)
			for _, metric := range componentMetrics.Metrics {
				categories[metric.Category]++
			}

			for category, count := range categories {
				log.Printf("    - %s: %d metrics", category, count)
			}
		} else {
			log.Printf("  ❌ %s: No metrics available", componentName)
		}
	}
}

// Enhanced Network Overview Dashboard with real metrics
func generateNetworkOverviewDashboard(endpoints map[string]string, topology *discovery.NetworkTopology, availableMetrics map[string]ComponentMetrics) Dashboard {
	var panels []Panel
	panelID := 1
	yPos := 0

	// Title panel with real status
	activeComponents := 0
	totalMetrics := 0
	for _, metrics := range availableMetrics {
		if metrics.IsAvailable {
			activeComponents++
			totalMetrics += len(metrics.Metrics)
		}
	}

	panels = append(panels, Panel{
		ID:      panelID,
		Title:   fmt.Sprintf("%s Core Network Overview", topology.Type),
		Type:    "text",
		GridPos: GridPos{H: 3, W: 24, X: 0, Y: yPos},
		Options: &PanelOptions{
			Content: fmt.Sprintf(`# %s Core Network Real-Time Monitoring

## Live Status
- **Active Components**: %d/%d network functions with metrics
- **Total Metrics**: %d real-time metrics being collected
- **Last Updated**: %s
- **Monitoring Type**: Real Open5GS metrics + Infrastructure monitoring

## Key Performance Indicators
This dashboard shows live data from your %s core network. All metrics are collected directly from Open5GS components.`,
				topology.Type, activeComponents, len(endpoints), totalMetrics, time.Now().Format("15:04:05"), topology.Type),
			Mode: "markdown",
		},
	})
	panelID++
	yPos += 3

	// Network functions status using actual 'up' metrics
	panels = append(panels, Panel{
		ID:      panelID,
		Title:   "Network Functions Status",
		Type:    "stat",
		GridPos: GridPos{H: 4, W: 12, X: 0, Y: yPos},
		Targets: []Target{{
			Expr:         "up{source=\"real_open5gs\"}",
			RefID:        "A",
			LegendFormat: "{{component}}",
			Datasource:   &Datasource{Type: "prometheus", UID: "Prometheus"},
		}},
		Options: &PanelOptions{
			ReduceOptions: &ReduceOptions{
				Values: false,
				Calcs:  []string{"lastNotNull"},
				Fields: "",
			},
			Orientation: "auto",
			TextMode:    "auto",
			ColorMode:   "value",
			GraphMode:   "area",
			JustifyMode: "auto",
		},
		FieldConfig: &FieldConfig{
			Defaults: &FieldDefaults{
				Unit: "short",
				Thresholds: &ThresholdConfig{
					Mode: "absolute",
					Steps: []ThresholdStep{
						{Color: "red", Value: nil},
						{Color: "green", Value: floatPtr(1)},
					},
				},
			},
		},
		Datasource: &Datasource{Type: "prometheus", UID: "Prometheus"},
	})
	panelID++

	// Infrastructure status
	panels = append(panels, Panel{
		ID:      panelID,
		Title:   "Infrastructure Status",
		Type:    "stat",
		GridPos: GridPos{H: 4, W: 12, X: 12, Y: yPos},
		Targets: []Target{{
			Expr:         "up{source=~\"container_stats|health_check\"}",
			RefID:        "A",
			LegendFormat: "{{job}}",
			Datasource:   &Datasource{Type: "prometheus", UID: "Prometheus"},
		}},
		Options: &PanelOptions{
			ReduceOptions: &ReduceOptions{
				Values: false,
				Calcs:  []string{"lastNotNull"},
				Fields: "",
			},
			Orientation: "auto",
			TextMode:    "auto",
			ColorMode:   "value",
		},
		FieldConfig: &FieldConfig{
			Defaults: &FieldDefaults{
				Unit: "short",
				Thresholds: &ThresholdConfig{
					Mode: "absolute",
					Steps: []ThresholdStep{
						{Color: "red", Value: nil},
						{Color: "green", Value: floatPtr(1)},
					},
				},
			},
		},
		Datasource: &Datasource{Type: "prometheus", UID: "Prometheus"},
	})
	panelID++
	yPos += 4

	// Add component-specific panels using real discovered metrics
	x := 0
	for componentName, componentMetrics := range availableMetrics {
		if !componentMetrics.IsAvailable || len(componentMetrics.Metrics) == 0 {
			continue
		}

		if x >= 24 {
			x = 0
			yPos += 6
		}

		panels = append(panels, generateComponentMetricsPanel(componentName, componentMetrics, panelID, x, yPos))
		panelID++
		x += 8
	}

	return Dashboard{
		Title:        fmt.Sprintf("%s Network Overview", topology.Type),
		Description:  fmt.Sprintf("Real-time monitoring dashboard for %s core network with %d active metrics", topology.Type, totalMetrics),
		Tags:         []string{strings.ToLower(string(topology.Type)), "overview", "network-functions", "real-metrics"},
		Style:        "dark",
		Timezone:     "browser",
		Editable:     true,
		GraphTooltip: 0,
		Time: TimeRange{
			From: "now-15m",
			To:   "now",
		},
		Timepicker: TimePicker{
			RefreshIntervals: []string{"5s", "10s", "30s", "1m", "5m", "15m", "30m", "1h", "2h", "1d"},
		},
		Templating:           Templating{List: []any{}},
		Annotations:          Annotations{List: []any{}},
		Refresh:              "10s",
		SchemaVersion:        27,
		Version:              1,
		Panels:               panels,
		FiscalYearStartMonth: 0,
	}
}

// Generate component panel using real discovered metrics
func generateComponentMetricsPanel(componentName string, componentMetrics ComponentMetrics, panelID, x, y int) Panel {
	var targets []Target
	title := strings.ToUpper(componentName)

	// Use the most important metrics (limit to 4 for readability)
	priorityMetrics := prioritizeMetrics(componentMetrics.Metrics)

	for i, metric := range priorityMetrics {
		if i >= 4 { // Limit to 4 metrics per panel
			break
		}

		targets = append(targets, Target{
			Expr:         fmt.Sprintf("%s{component=\"%s\"}", metric.Name, componentName),
			RefID:        string(rune('A' + i)),
			LegendFormat: metric.Name,
			Datasource:   &Datasource{Type: "prometheus", UID: "Prometheus"},
		})
	}

	// If no specific metrics, fall back to basic up metric
	if len(targets) == 0 {
		targets = []Target{{
			Expr:         fmt.Sprintf("up{component=\"%s\"}", componentName),
			RefID:        "A",
			LegendFormat: "Status",
			Datasource:   &Datasource{Type: "prometheus", UID: "Prometheus"},
		}}
	}

	return Panel{
		ID:      panelID,
		Title:   fmt.Sprintf("%s Metrics", title),
		Type:    "timeseries",
		GridPos: GridPos{H: 6, W: 8, X: x, Y: y},
		Targets: targets,
		FieldConfig: &FieldConfig{
			Defaults: &FieldDefaults{
				Color: &ColorConfig{Mode: "palette-classic"},
				Custom: &CustomConfig{
					DrawStyle:         "line",
					LineInterpolation: "linear",
					LineWidth:         1,
					FillOpacity:       0,
					GradientMode:      "none",
					SpanNulls:         false,
					PointSize:         5,
				},
				Unit: "short",
			},
		},
		Datasource:  &Datasource{Type: "prometheus", UID: "Prometheus"},
		Description: fmt.Sprintf("Real-time metrics from %s component (%d total metrics available)", componentName, len(componentMetrics.Metrics)),
	}
}

// Prioritize metrics for display (most important first)
func prioritizeMetrics(metrics []MetricInfo) []MetricInfo {
	var prioritized []MetricInfo

	// Priority order: sessions > users > requests > protocols > system > general
	priorities := []string{"sessions", "users", "requests", "protocols", "system", "general"}

	for _, priority := range priorities {
		for _, metric := range metrics {
			if metric.Category == priority {
				prioritized = append(prioritized, metric)
			}
		}
	}

	return prioritized
}

// Get fallback metrics structure when discovery fails
func getDefaultMetricsStructure() map[string]ComponentMetrics {
	return map[string]ComponentMetrics{
		"amf": {
			ComponentName: "amf",
			Metrics: []MetricInfo{
				{Name: "up", Type: "gauge", Help: "Component status", Category: "health"},
			},
			IsAvailable: false,
		},
		"smf": {
			ComponentName: "smf",
			Metrics: []MetricInfo{
				{Name: "up", Type: "gauge", Help: "Component status", Category: "health"},
			},
			IsAvailable: false,
		},
	}
}

// Updated Infrastructure Dashboard
func generateInfrastructureDashboard(topology *discovery.NetworkTopology, availableMetrics map[string]ComponentMetrics) Dashboard {
	var panels []Panel
	panelID := 1

	// Container CPU usage
	panels = append(panels, Panel{
		ID:      panelID,
		Title:   "Container CPU Usage",
		Type:    "timeseries",
		GridPos: GridPos{H: 8, W: 12, X: 0, Y: 0},
		Targets: []Target{{
			Expr:         "container_cpu_usage_percent",
			RefID:        "A",
			LegendFormat: "{{container_name}}",
			Datasource:   &Datasource{Type: "prometheus", UID: "Prometheus"},
		}},
		FieldConfig: &FieldConfig{
			Defaults: &FieldDefaults{
				Unit: "percent",
				Custom: &CustomConfig{
					DrawStyle:         "line",
					LineInterpolation: "linear",
					LineWidth:         1,
					FillOpacity:       10,
				},
			},
		},
		Datasource: &Datasource{Type: "prometheus", UID: "Prometheus"},
	})
	panelID++

	// Container Memory usage
	panels = append(panels, Panel{
		ID:      panelID,
		Title:   "Container Memory Usage",
		Type:    "timeseries",
		GridPos: GridPos{H: 8, W: 12, X: 12, Y: 0},
		Targets: []Target{{
			Expr:         "container_memory_usage_bytes",
			RefID:        "A",
			LegendFormat: "{{container_name}}",
			Datasource:   &Datasource{Type: "prometheus", UID: "Prometheus"},
		}},
		FieldConfig: &FieldConfig{
			Defaults: &FieldDefaults{
				Unit: "bytes",
				Custom: &CustomConfig{
					DrawStyle:         "line",
					LineInterpolation: "linear",
					LineWidth:         1,
					FillOpacity:       10,
				},
			},
		},
		Datasource: &Datasource{Type: "prometheus", UID: "Prometheus"},
	})

	return Dashboard{
		Title:       "Infrastructure Monitoring",
		Description: "Container resources and health monitoring with real metrics",
		Tags:        []string{"infrastructure", "containers", "health"},
		Style:       "dark",
		Timezone:    "browser",
		Editable:    true,
		Time: TimeRange{
			From: "now-1h",
			To:   "now",
		},
		Refresh:       "30s",
		SchemaVersion: 27,
		Version:       1,
		Panels:        panels,
	}
}

// Updated Health Dashboard
func generateHealthDashboard(topology *discovery.NetworkTopology, availableMetrics map[string]ComponentMetrics) Dashboard {
	var panels []Panel
	panelID := 1

	// Overall health status
	panels = append(panels, Panel{
		ID:      panelID,
		Title:   "System Health Overview",
		Type:    "stat",
		GridPos: GridPos{H: 4, W: 24, X: 0, Y: 0},
		Targets: []Target{{
			Expr:         "avg(up)",
			RefID:        "A",
			LegendFormat: "Overall Health",
			Datasource:   &Datasource{Type: "prometheus", UID: "Prometheus"},
		}},
		Options: &PanelOptions{
			ColorMode:   "background",
			GraphMode:   "none",
			JustifyMode: "center",
		},
		FieldConfig: &FieldConfig{
			Defaults: &FieldDefaults{
				Unit: "percentunit",
				Thresholds: &ThresholdConfig{
					Steps: []ThresholdStep{
						{Color: "red", Value: nil},
						{Color: "yellow", Value: floatPtr(0.8)},
						{Color: "green", Value: floatPtr(0.95)},
					},
				},
			},
		},
		Datasource: &Datasource{Type: "prometheus", UID: "Prometheus"},
	})

	return Dashboard{
		Title:       "Health Monitoring",
		Description: "Component health and availability monitoring",
		Tags:        []string{"health", "monitoring", "availability"},
		Style:       "dark",
		Timezone:    "browser",
		Editable:    true,
		Time: TimeRange{
			From: "now-30m",
			To:   "now",
		},
		Refresh:       "15s",
		SchemaVersion: 27,
		Version:       1,
		Panels:        panels,
	}
}

// Enhanced Educational Dashboard
func generateEducationalDashboard(topology *discovery.NetworkTopology, availableMetrics map[string]ComponentMetrics) Dashboard {
	var panels []Panel
	panelID := 1

	// Educational content with real metrics info
	metricsCount := 0
	componentsList := make([]string, 0)
	for name, metrics := range availableMetrics {
		if metrics.IsAvailable {
			componentsList = append(componentsList, name)
			metricsCount += len(metrics.Metrics)
		}
	}

	educationalContent := fmt.Sprintf(`%s

## Live Metrics Being Collected
- **Total Real Metrics**: %d
- **Active Components**: %s
- **Collection Frequency**: Every 5-10 seconds

## How to Read This Data
1. All metrics are collected live from Open5GS components
2. Each component exposes its own metrics endpoint
3. Prometheus aggregates all data for visualization
4. Grafana provides the visualization layer

## Next Steps for Learning
- Explore individual component dashboards
- Compare 4G vs 5G metrics patterns
- Analyze traffic patterns during UE attachments`,
		generateEducationalContent(string(topology.Type)),
		metricsCount,
		strings.Join(componentsList, ", "))

	panels = append(panels, Panel{
		ID:      panelID,
		Title:   fmt.Sprintf("%s Architecture Guide", topology.Type),
		Type:    "text",
		GridPos: GridPos{H: 12, W: 12, X: 0, Y: 0},
		Options: &PanelOptions{
			Content: educationalContent,
			Mode:    "markdown",
		},
	})
	panelID++

	// Real metrics glossary based on discovered metrics
	realMetricsGlossary := generateRealMetricsGlossary(availableMetrics)
	panels = append(panels, Panel{
		ID:      panelID,
		Title:   "Discovered Metrics Glossary",
		Type:    "text",
		GridPos: GridPos{H: 12, W: 12, X: 12, Y: 0},
		Options: &PanelOptions{
			Content: realMetricsGlossary,
			Mode:    "markdown",
		},
	})

	return Dashboard{
		Title:       fmt.Sprintf("%s Educational Dashboard", topology.Type),
		Description: "Learn about telecommunications monitoring with real metrics",
		Tags:        []string{"education", strings.ToLower(string(topology.Type)), "learning", "real-metrics"},
		Style:       "dark",
		Timezone:    "browser",
		Editable:    true,
		Time: TimeRange{
			From: "now-15m",
			To:   "now",
		},
		Refresh:       "1m",
		SchemaVersion: 27,
		Version:       1,
		Panels:        panels,
	}
}

// Generate component-specific dashboard with real metrics
func generateComponentDashboard(componentName string, topology *discovery.NetworkTopology, componentMetrics ComponentMetrics) Dashboard {
	var panels []Panel
	panelID := 1

	title := fmt.Sprintf("%s Detailed Metrics", strings.ToUpper(componentName))

	// Component status panel
	panels = append(panels, Panel{
		ID:      panelID,
		Title:   "Component Status",
		Type:    "stat",
		GridPos: GridPos{H: 4, W: 8, X: 0, Y: 0},
		Targets: []Target{{
			Expr:         fmt.Sprintf("up{component=\"%s\"}", componentName),
			RefID:        "A",
			LegendFormat: "Status",
			Datasource:   &Datasource{Type: "prometheus", UID: "Prometheus"},
		}},
		FieldConfig: &FieldConfig{
			Defaults: &FieldDefaults{
				Thresholds: &ThresholdConfig{
					Steps: []ThresholdStep{
						{Color: "red", Value: nil},
						{Color: "green", Value: floatPtr(1)},
					},
				},
			},
		},
		Datasource: &Datasource{Type: "prometheus", UID: "Prometheus"},
	})
	panelID++

	// Metrics summary panel
	panels = append(panels, Panel{
		ID:      panelID,
		Title:   "Metrics Summary",
		Type:    "text",
		GridPos: GridPos{H: 4, W: 16, X: 8, Y: 0},
		Options: &PanelOptions{
			Content: fmt.Sprintf(`## %s Component Details

**Total Metrics Available**: %d
**Last Updated**: %s
**Component Type**: %s Network Function

### Available Metric Categories:
%s

All metrics are collected live from the Open5GS %s component.`,
				componentName,
				len(componentMetrics.Metrics),
				componentMetrics.LastUpdated.Format("15:04:05"),
				strings.ToUpper(string(topology.Type)),
				getMetricCategorySummary(componentMetrics.Metrics),
				componentName),
			Mode: "markdown",
		},
	})
	panelID++

	// Create panels for each metric category
	yPos := 4
	x := 0

	categories := groupMetricsByCategory(componentMetrics.Metrics)
	for category, categoryMetrics := range categories {
		if x >= 24 {
			x = 0
			yPos += 8
		}

		panels = append(panels, generateCategoryPanel(category, categoryMetrics, componentName, panelID, x, yPos))
		panelID++
		x += 12
	}

	return Dashboard{
		Title:       title,
		Description: fmt.Sprintf("Detailed monitoring for %s component with %d real metrics", componentName, len(componentMetrics.Metrics)),
		Tags:        []string{componentName, "detailed", strings.ToLower(string(topology.Type)), "real-metrics"},
		Style:       "dark",
		Timezone:    "browser",
		Editable:    true,
		Time: TimeRange{
			From: "now-1h",
			To:   "now",
		},
		Refresh:       "10s",
		SchemaVersion: 27,
		Version:       1,
		Panels:        panels,
	}
}

// Group metrics by category for better organization
func groupMetricsByCategory(metrics []MetricInfo) map[string][]MetricInfo {
	categories := make(map[string][]MetricInfo)

	for _, metric := range metrics {
		categories[metric.Category] = append(categories[metric.Category], metric)
	}

	return categories
}

// Generate panel for a specific metric category
func generateCategoryPanel(category string, metrics []MetricInfo, componentName string, panelID, x, y int) Panel {
	var targets []Target

	// Limit to 5 metrics per category panel for readability
	for i, metric := range metrics {
		if i >= 5 {
			break
		}

		targets = append(targets, Target{
			Expr:         fmt.Sprintf("%s{component=\"%s\"}", metric.Name, componentName),
			RefID:        string(rune('A' + i)),
			LegendFormat: metric.Name,
			Datasource:   &Datasource{Type: "prometheus", UID: "Prometheus"},
		})
	}

	return Panel{
		ID:      panelID,
		Title:   fmt.Sprintf("%s Metrics", strings.ToTitle(category)),
		Type:    "timeseries",
		GridPos: GridPos{H: 8, W: 12, X: x, Y: y},
		Targets: targets,
		FieldConfig: &FieldConfig{
			Defaults: &FieldDefaults{
				Color: &ColorConfig{Mode: "palette-classic"},
				Custom: &CustomConfig{
					DrawStyle:         "line",
					LineInterpolation: "linear",
					LineWidth:         1,
					FillOpacity:       0,
					GradientMode:      "none",
					SpanNulls:         false,
					PointSize:         5,
				},
				Unit: "short",
			},
		},
		Datasource:  &Datasource{Type: "prometheus", UID: "Prometheus"},
		Description: fmt.Sprintf("%s-related metrics from %s (%d metrics in this category)", category, componentName, len(metrics)),
	}
}

// Get metric category summary for display
func getMetricCategorySummary(metrics []MetricInfo) string {
	categories := make(map[string]int)
	for _, metric := range metrics {
		categories[metric.Category]++
	}

	var summary strings.Builder
	for category, count := range categories {
		summary.WriteString(fmt.Sprintf("- **%s**: %d metrics\n", strings.ToTitle(category), count))
	}

	return summary.String()
}

// Generate real metrics glossary based on discovered metrics
func generateRealMetricsGlossary(availableMetrics map[string]ComponentMetrics) string {
	var glossary strings.Builder

	glossary.WriteString("# Discovered Real Metrics\n\n")
	glossary.WriteString("This glossary shows the actual metrics discovered from your running components:\n\n")

	for componentName, componentMetrics := range availableMetrics {
		if !componentMetrics.IsAvailable || len(componentMetrics.Metrics) == 0 {
			continue
		}

		glossary.WriteString(fmt.Sprintf("## %s Component\n", strings.ToUpper(componentName)))
		glossary.WriteString(fmt.Sprintf("**Status**: Active (%d metrics)\n\n", len(componentMetrics.Metrics)))

		// Group by category
		categories := groupMetricsByCategory(componentMetrics.Metrics)
		for category, metrics := range categories {
			glossary.WriteString(fmt.Sprintf("### %s Metrics\n", strings.ToTitle(category)))
			for _, metric := range metrics {
				glossary.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", metric.Name, metric.Type, metric.Help))
			}
			glossary.WriteString("\n")
		}
	}

	if len(availableMetrics) == 0 {
		glossary.WriteString("No metrics discovered. Ensure Open5GS components are running and metrics are enabled.\n")
	}

	return glossary.String()
}

// Fixed file path handling for Docker vs non-Docker environments
func writeDashboardFiles(dashboards []Dashboard, inDocker bool) error {
	var basePath string

	if inDocker {
		// In Docker, write to mounted volume that Grafana can access
		basePath = "/etc/grafana/provisioning/dashboards"
	} else {
		// Non-Docker: write to local grafana directory
		basePath = "../grafana/provisioning/dashboards"
	}

	for _, dashboard := range dashboards {
		filename := fmt.Sprintf("%s.json", strings.ToLower(strings.ReplaceAll(dashboard.Title, " ", "_")))
		filepath := filepath.Join(basePath, filename)

		dashboardJSON, err := json.MarshalIndent(dashboard, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal dashboard %s: %w", dashboard.Title, err)
		}

		if err := os.WriteFile(filepath, dashboardJSON, 0644); err != nil {
			return fmt.Errorf("failed to write dashboard %s to %s: %w", dashboard.Title, filepath, err)
		}

		log.Printf("📊 Generated dashboard: %s", filepath)
	}

	return nil
}

// Generate dashboard provisioning configuration
func generateDashboardProvisioning(inDocker bool) error {
	provisioningConfig := `apiVersion: 1

providers:
  - name: 'om-module-dashboards'
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: /etc/grafana/provisioning/dashboards
`

	var configPath string
	if inDocker {
		configPath = "/etc/grafana/provisioning/dashboards/dashboards.yaml"
	} else {
		configPath = "../grafana/provisioning/dashboards/dashboards.yaml"
	}

	if err := os.WriteFile(configPath, []byte(provisioningConfig), 0644); err != nil {
		return fmt.Errorf("failed to write dashboard provisioning config to %s: %w", configPath, err)
	}

	log.Printf("📝 Generated dashboard provisioning config: %s", configPath)

	// Reload Grafana if running in Docker
	if inDocker {
		go reloadGrafana()
	}
	return nil
}

// Generate educational content based on deployment type
func generateEducationalContent(deploymentType string) string {
	if deploymentType == "5G" {
		return `# 5G Core Network Architecture

## Key Components
- **AMF**: Access and Mobility Management Function
- **SMF**: Session Management Function  
- **UPF**: User Plane Function
- **PCF**: Policy Control Function
- **NRF**: Network Repository Function

## Key Interfaces
- **N1**: UE ↔ AMF
- **N2**: gNodeB ↔ AMF
- **N3**: gNodeB ↔ UPF
- **N4**: SMF ↔ UPF

## Monitoring Focus
- Registration procedures
- Session establishment
- Policy enforcement
- Service discovery`
	} else {
		return `# 4G LTE Core Network Architecture

## Key Components
- **MME**: Mobility Management Entity
- **HSS**: Home Subscriber Server
- **PCRF**: Policy and Charging Rules Function
- **SGWC/SGWU**: Serving Gateway Control/User Plane

## Key Interfaces
- **S1-MME**: eNodeB ↔ MME
- **S6a**: MME ↔ HSS
- **S11**: MME ↔ SGWC
- **Gx**: PCEF ↔ PCRF

## Monitoring Focus
- Attach procedures
- Bearer management
- Handover events
- Policy enforcement`
	}
}

// Helper function for float pointer
func floatPtr(f float64) *float64 {
	return &f
}

// Reload Grafana dashboards configuration (similar to reloadPrometheus)
func reloadGrafana() {
	time.Sleep(5 * time.Second) // Give Grafana time to start and files to be written

	// Try to reload Grafana provisioning via Docker network
	grafanaIP := os.Getenv("GRAFANA_IP")
	if grafanaIP == "" {
		grafanaIP = "grafana" // Use Docker service name as fallback
	}

	// Grafana provisioning reload endpoint
	reloadURL := fmt.Sprintf("http://%s:3000/api/admin/provisioning/dashboards/reload", grafanaIP)
	log.Printf("🔄 Attempting to reload Grafana dashboards at: %s", reloadURL)

	client := &http.Client{Timeout: 10 * time.Second}

	// Create POST request
	req, err := http.NewRequest("POST", reloadURL, nil)
	if err != nil {
		log.Printf("⚠️ Failed to create Grafana reload request: %v", err)
		return
	}

	// Add admin authentication
	username := os.Getenv("GRAFANA_USERNAME")
	password := os.Getenv("GRAFANA_PASSWORD")
	if username == "" {
		username = "admin" // Default username
	}
	if password == "" {
		password = "admin" // Default password - you should change this
	}
	req.SetBasicAuth(username, password)

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("⚠️ Failed to reload Grafana dashboards: %v", err)
		return
	}
	defer func() {
		err = resp.Body.Close()
	}()

	if resp.StatusCode == 200 {
		log.Printf("✅ Grafana dashboard provisioning reloaded successfully")
	} else {
		log.Printf("⚠️ Grafana reload returned status: %d", resp.StatusCode)

		// Try alternative endpoint for older Grafana versions
		alternativeReloadURL := fmt.Sprintf("http://%s:3000/api/admin/provisioning/reload", grafanaIP)
		log.Printf("🔄 Trying alternative reload endpoint: %s", alternativeReloadURL)

		altReq, err := http.NewRequest("POST", alternativeReloadURL, nil)
		if err != nil {
			log.Printf("⚠️ Failed to create alternative reload request: %v", err)
			return
		}
		altReq.SetBasicAuth(username, password)
		altReq.Header.Set("Content-Type", "application/json")

		altResp, err := client.Do(altReq)
		if err != nil {
			log.Printf("⚠️ Failed alternative Grafana reload: %v", err)
			return
		}
		defer func() {
			err = altResp.Body.Close()
		}()

		if altResp.StatusCode == 200 {
			log.Printf("✅ Grafana provisioning reloaded successfully (alternative endpoint)")
		} else {
			log.Printf("⚠️ Alternative Grafana reload also returned status: %d", altResp.StatusCode)
		}
	}
}
