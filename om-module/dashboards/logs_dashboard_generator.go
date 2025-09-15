// om-module/dashboards/log_dashboard_generator.go
package dashboards

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Parz1val02/OM_module/discovery"
	"github.com/Parz1val02/OM_module/logging"
)

// GridPosition represents panel position and size
type GridPosition struct {
	H int `json:"h"` // height
	W int `json:"w"` // width
	X int `json:"x"` // x position
	Y int `json:"y"` // y position
}

// DashboardVariable represents a Grafana dashboard variable
type DashboardVariable struct {
	Name       string   `json:"name"`
	Label      string   `json:"label"`
	Type       string   `json:"type"`
	Datasource string   `json:"datasource,omitempty"`
	Query      string   `json:"query,omitempty"`
	Options    []string `json:"options,omitempty"`
	Multi      bool     `json:"multi"`
}

// LogBasedDashboard represents a dashboard built from log analysis
type LogBasedDashboard struct {
	Title       string              `json:"title"`
	Tags        []string            `json:"tags"`
	Panels      []LogPanel          `json:"panels"`
	Variables   []DashboardVariable `json:"templating"`
	Time        TimeRange           `json:"time"`
	Refresh     string              `json:"refresh"`
	Description string              `json:"description"`
}

// LogPanel represents a panel that uses Loki queries
type LogPanel struct {
	ID          int            `json:"id"`
	Title       string         `json:"title"`
	Type        string         `json:"type"`
	Targets     []LokiTarget   `json:"targets"`
	GridPos     GridPosition   `json:"gridPos"`
	Options     map[string]any `json:"options"`
	FieldConfig FieldConfig    `json:"fieldConfig"`
	Description string         `json:"description"`
}

// LokiTarget represents a Loki query
type LokiTarget struct {
	Expr         string `json:"expr"`
	RefID        string `json:"refId"`
	Datasource   string `json:"datasource"`
	QueryType    string `json:"queryType"`
	Resolution   int    `json:"resolution"`
	MaxLines     int    `json:"maxLines"`
	LegendFormat string `json:"legendFormat"`
}

// GenerateLogBasedDashboards creates dashboards based on discovered log patterns
func GenerateLogBasedDashboards(topology *discovery.NetworkTopology, loggingInsights map[string]any) ([]LogBasedDashboard, error) {
	log.Printf("📊 Generating log-based Grafana dashboards...")

	var dashboards []LogBasedDashboard

	// 1. Protocol Flow Analysis Dashboard
	protocolDashboard := generateProtocolFlowDashboard(topology, loggingInsights)
	dashboards = append(dashboards, protocolDashboard)

	// 2. Session Tracking Dashboard
	sessionDashboard := generateSessionTrackingDashboard(topology, loggingInsights)
	dashboards = append(dashboards, sessionDashboard)

	// 3. Performance Metrics Dashboard (from logs)
	performanceDashboard := generatePerformanceLogDashboard(topology, loggingInsights)
	dashboards = append(dashboards, performanceDashboard)

	// 4. Troubleshooting Dashboard
	troubleshootingDashboard := generateTroubleshootingDashboard(topology, loggingInsights)
	dashboards = append(dashboards, troubleshootingDashboard)

	// 5. Educational Flow Dashboard
	educationalDashboard := generateEducationalFlowDashboard(topology, loggingInsights)
	dashboards = append(dashboards, educationalDashboard)

	log.Printf("✅ Generated %d log-based dashboards", len(dashboards))
	return dashboards, nil
}

// generateProtocolFlowDashboard creates a dashboard for protocol layer analysis
func generateProtocolFlowDashboard(topology *discovery.NetworkTopology, insights map[string]any) LogBasedDashboard {
	panels := []LogPanel{
		{
			ID:    1,
			Title: "Protocol Layer Distribution",
			Type:  "piechart",
			Targets: []LokiTarget{
				{
					Expr:       `sum by (protocol_layer) (count_over_time({component_type="core"} | json | protocol_layer != "" [5m]))`,
					RefID:      "A",
					Datasource: "Loki-Advanced",
					QueryType:  "",
					MaxLines:   1000,
				},
			},
			GridPos:     GridPosition{H: 8, W: 12, X: 0, Y: 0},
			Description: "Distribution of log entries by protocol layer (NAS, RRC, S1AP, etc.)",
		},
		{
			ID:    2,
			Title: "NAS Message Timeline",
			Type:  "logs",
			Targets: []LokiTarget{
				{
					Expr:       `{component_type="core"} | json | protocol_layer="NAS" | line_format "{{.timestamp}} [{{.level}}] {{.component}}: {{.message}}"`,
					RefID:      "A",
					Datasource: "Loki-Advanced",
					MaxLines:   500,
				},
			},
			GridPos:     GridPosition{H: 8, W: 12, X: 12, Y: 0},
			Description: "Real-time NAS (Non-Access Stratum) message flow",
		},
		{
			ID:    3,
			Title: "RRC Connection Events",
			Type:  "stat",
			Targets: []LokiTarget{
				{
					Expr:       `sum(count_over_time({component_type="ran"} | json | protocol_layer="RRC" |~ "Connection" [1h]))`,
					RefID:      "A",
					Datasource: "Loki-Advanced",
				},
			},
			GridPos:     GridPosition{H: 4, W: 6, X: 0, Y: 8},
			Description: "RRC connection establishment events in the last hour",
		},
		{
			ID:    4,
			Title: "S1AP/NGAP Procedures",
			Type:  "table",
			Targets: []LokiTarget{
				{
					Expr:       `{component_type="core"} | json | protocol_layer=~"S1AP|NGAP" | line_format "{{.procedure}} {{.component}} {{.timestamp}}"`,
					RefID:      "A",
					Datasource: "Loki-Advanced",
					MaxLines:   100,
				},
			},
			GridPos:     GridPosition{H: 8, W: 18, X: 6, Y: 8},
			Description: "S1AP (4G) and NGAP (5G) procedure tracking",
		},
	}

	return LogBasedDashboard{
		Title:  "Protocol Flow Analysis",
		Tags:   []string{"telecom", "protocols", "education", "logs"},
		Panels: panels,
		Variables: []DashboardVariable{
			{
				Name:       "component",
				Label:      "Component",
				Type:       "query",
				Datasource: "Loki-Advanced",
				Query:      `label_values(component)`,
				Multi:      true,
			},
			{
				Name:    "protocol_layer",
				Label:   "Protocol Layer",
				Type:    "custom",
				Options: []string{"NAS", "RRC", "S1AP", "NGAP", "PHY", "MAC", "RLC", "PDCP"},
				Multi:   true,
			},
		},
		Time: TimeRange{
			From: "now-1h",
			To:   "now",
		},
		Refresh:     "30s",
		Description: "Analyze protocol layer interactions in your 4G/5G network. Track NAS, RRC, S1AP, and NGAP message flows with educational context.",
	}
}

// generateSessionTrackingDashboard creates a dashboard for complete session flows
func generateSessionTrackingDashboard(topology *discovery.NetworkTopology, insights map[string]any) LogBasedDashboard {
	panels := []LogPanel{
		{
			ID:    1,
			Title: "Active UE Sessions",
			Type:  "stat",
			Targets: []LokiTarget{
				{
					Expr:       `count(count by (imsi) (count_over_time({component_type="core"} | json | imsi != "" [5m])))`,
					RefID:      "A",
					Datasource: "Loki-SessionFlow",
				},
			},
			GridPos:     GridPosition{H: 4, W: 6, X: 0, Y: 0},
			Description: "Number of active UE sessions (tracked by IMSI)",
		},
		{
			ID:    2,
			Title: "Session Establishment Flow",
			Type:  "logs",
			Targets: []LokiTarget{
				{
					Expr:       `{component_type="core"} | json | procedure="attach" | line_format "{{.timestamp}} [{{.flow_step}}] {{.component}}: {{.student_note}}"`,
					RefID:      "A",
					Datasource: "Loki-SessionFlow",
					MaxLines:   200,
				},
			},
			GridPos:     GridPosition{H: 8, W: 18, X: 6, Y: 0},
			Description: "Step-by-step UE attachment procedure with educational notes",
		},
		{
			ID:    3,
			Title: "Session Success Rate",
			Type:  "gauge",
			Targets: []LokiTarget{
				{
					Expr:       `(sum(count_over_time({component_type="core"} | json | procedure="attach" |~ "Complete" [1h])) / sum(count_over_time({component_type="core"} | json | procedure="attach" |~ "Request" [1h]))) * 100`,
					RefID:      "A",
					Datasource: "Loki-SessionFlow",
				},
			},
			GridPos: GridPosition{H: 8, W: 6, X: 0, Y: 4},
			Options: map[string]any{
				"min": 0,
				"max": 100,
				"thresholds": map[string]any{
					"steps": []map[string]any{
						{"color": "red", "value": 0},
						{"color": "yellow", "value": 80},
						{"color": "green", "value": 95},
					},
				},
			},
			Description: "Percentage of successful UE attachments",
		},
		{
			ID:    4,
			Title: "Handover Events",
			Type:  "timeseries",
			Targets: []LokiTarget{
				{
					Expr:         `sum by (component) (count_over_time({component_type="core"} | json | procedure="handover" [1m]))`,
					RefID:        "A",
					Datasource:   "Loki-SessionFlow",
					LegendFormat: "{{component}}",
				},
			},
			GridPos:     GridPosition{H: 8, W: 24, X: 0, Y: 12},
			Description: "UE handover events over time by component",
		},
	}

	return LogBasedDashboard{
		Title:  "Session Flow Tracking",
		Tags:   []string{"telecom", "sessions", "ue", "flows", "education"},
		Panels: panels,
		Variables: []DashboardVariable{
			{
				Name:       "imsi",
				Label:      "IMSI",
				Type:       "query",
				Datasource: "Loki-SessionFlow",
				Query:      `label_values(imsi)`,
				Multi:      false,
			},
			{
				Name:    "procedure",
				Label:   "Procedure",
				Type:    "custom",
				Options: []string{"attach", "detach", "handover", "authentication", "paging"},
				Multi:   true,
			},
		},
		Time: TimeRange{
			From: "now-1h",
			To:   "now",
		},
		Refresh:     "10s",
		Description: "Track complete UE session flows from attachment to detachment. Follow individual IMSIs through the network with educational context.",
	}
}

// generatePerformanceLogDashboard creates a dashboard for performance metrics extracted from logs
func generatePerformanceLogDashboard(topology *discovery.NetworkTopology, insights map[string]any) LogBasedDashboard {
	panels := []LogPanel{
		{
			ID:    1,
			Title: "RSRP Signal Quality",
			Type:  "timeseries",
			Targets: []LokiTarget{
				{
					Expr:         `avg_over_time({component_type="ran"} | json | rsrp != "" | unwrap rsrp [1m])`,
					RefID:        "A",
					Datasource:   "Loki-Advanced",
					LegendFormat: "RSRP (dBm)",
				},
			},
			GridPos: GridPosition{H: 8, W: 12, X: 0, Y: 0},
			Options: map[string]any{
				"tooltip": map[string]any{
					"mode": "multi",
				},
				"legend": map[string]any{
					"displayMode": "table",
					"values":      []string{"min", "max", "mean"},
				},
			},
			Description: "Reference Signal Received Power over time (typical range: -140 to -44 dBm)",
		},
		{
			ID:    2,
			Title: "RSRQ Signal Quality",
			Type:  "timeseries",
			Targets: []LokiTarget{
				{
					Expr:         `avg_over_time({component_type="ran"} | json | rsrq != "" | unwrap rsrq [1m])`,
					RefID:        "A",
					Datasource:   "Loki-Advanced",
					LegendFormat: "RSRQ (dB)",
				},
			},
			GridPos:     GridPosition{H: 8, W: 12, X: 12, Y: 0},
			Description: "Reference Signal Received Quality over time (typical range: -19.5 to -3 dB)",
		},
		{
			ID:    3,
			Title: "Channel Quality Indicator",
			Type:  "bargauge",
			Targets: []LokiTarget{
				{
					Expr:         `avg by (cell_id) (avg_over_time({component_type="ran"} | json | cqi != "" | unwrap cqi [5m]))`,
					RefID:        "A",
					Datasource:   "Loki-Advanced",
					LegendFormat: "Cell {{cell_id}}",
				},
			},
			GridPos: GridPosition{H: 8, W: 8, X: 0, Y: 8},
			Options: map[string]any{
				"orientation": "horizontal",
				"displayMode": "gradient",
			},
			Description: "CQI values by cell (0-15 scale, higher is better)",
		},
		{
			ID:    4,
			Title: "Throughput Metrics",
			Type:  "stat",
			Targets: []LokiTarget{
				{
					Expr:       `avg_over_time({component_type="ran"} | json | throughput != "" | unwrap throughput [5m])`,
					RefID:      "A",
					Datasource: "Loki-Advanced",
				},
			},
			GridPos: GridPosition{H: 4, W: 8, X: 8, Y: 8},
			Options: map[string]any{
				"colorMode":   "background",
				"graphMode":   "area",
				"justifyMode": "center",
			},
			Description: "Average throughput extracted from RAN logs",
		},
		{
			ID:    5,
			Title: "Error Rate Analysis",
			Type:  "timeseries",
			Targets: []LokiTarget{
				{
					Expr:         `sum by (component) (rate({component_type=~"core|ran"} | json | level="error" [1m]))`,
					RefID:        "A",
					Datasource:   "Loki-Advanced",
					LegendFormat: "{{component}} errors/sec",
				},
			},
			GridPos:     GridPosition{H: 8, W: 16, X: 8, Y: 12},
			Description: "Error rate trends across all network components",
		},
	}

	return LogBasedDashboard{
		Title:  "Performance Metrics from Logs",
		Tags:   []string{"telecom", "performance", "kpi", "logs", "ran"},
		Panels: panels,
		Variables: []DashboardVariable{
			{
				Name:       "cell_id",
				Label:      "Cell ID",
				Type:       "query",
				Datasource: "Loki-Advanced",
				Query:      `label_values(cell_id)`,
				Multi:      true,
			},
		},
		Time: TimeRange{
			From: "now-1h",
			To:   "now",
		},
		Refresh:     "30s",
		Description: "Performance KPIs extracted from network logs: RSRP, RSRQ, CQI, throughput, and error rates.",
	}
}

// generateTroubleshootingDashboard creates a dashboard focused on common issues
func generateTroubleshootingDashboard(topology *discovery.NetworkTopology, insights map[string]any) LogBasedDashboard {
	panels := []LogPanel{
		{
			ID:    1,
			Title: "Authentication Failures",
			Type:  "logs",
			Targets: []LokiTarget{
				{
					Expr:       `{component_type="core"} | json | level="error" | procedure="authentication" | line_format "{{.timestamp}} {{.component}}: {{.message}} (IMSI: {{.imsi}})"`,
					RefID:      "A",
					Datasource: "Loki-Advanced",
					MaxLines:   100,
				},
			},
			GridPos:     GridPosition{H: 8, W: 12, X: 0, Y: 0},
			Description: "Failed authentication attempts with IMSI details",
		},
		{
			ID:    2,
			Title: "Timeout Events",
			Type:  "timeseries",
			Targets: []LokiTarget{
				{
					Expr:         `sum by (component) (count_over_time({component_type=~"core|ran"} | json |~ "timeout" [1m]))`,
					RefID:        "A",
					Datasource:   "Loki-Advanced",
					LegendFormat: "{{component}} timeouts",
				},
			},
			GridPos:     GridPosition{H: 8, W: 12, X: 12, Y: 0},
			Description: "Network timeout events by component",
		},
		{
			ID:    3,
			Title: "Common Error Patterns",
			Type:  "table",
			Targets: []LokiTarget{
				{
					Expr:       `topk(10, sum by (error_type) (count_over_time({component_type=~"core|ran"} | json | level="error" [1h])))`,
					RefID:      "A",
					Datasource: "Loki-Advanced",
				},
			},
			GridPos:     GridPosition{H: 8, W: 12, X: 0, Y: 8},
			Description: "Most frequent error types in the network",
		},
		{
			ID:    4,
			Title: "Radio Issues",
			Type:  "stat",
			Targets: []LokiTarget{
				{
					Expr:       `count(count_over_time({component_type="ran"} | json | rsrp != "" | rsrp < -120 [5m]))`,
					RefID:      "A",
					Datasource: "Loki-Advanced",
				},
			},
			GridPos: GridPosition{H: 4, W: 6, X: 12, Y: 8},
			Options: map[string]any{
				"colorMode": "background",
				"thresholds": map[string]any{
					"steps": []map[string]any{
						{"color": "green", "value": 0},
						{"color": "yellow", "value": 5},
						{"color": "red", "value": 20},
					},
				},
			},
			Description: "UEs with poor signal quality (RSRP < -120 dBm)",
		},
		{
			ID:      5,
			Title:   "Troubleshooting Guide",
			Type:    "text",
			GridPos: GridPosition{H: 8, W: 6, X: 18, Y: 8},
			Options: map[string]any{
				"content": `## Quick Troubleshooting Steps

### Authentication Issues:
1. Check HSS/UDM subscriber data
2. Verify shared secret keys
3. Check network configuration

### Radio Problems:
1. Check antenna positioning
2. Verify RF configuration  
3. Check for interference

### Timeout Issues:
1. Check network connectivity
2. Verify component health
3. Review resource usage

### Session Failures:
1. Check complete IMSI flow
2. Verify bearer establishment
3. Review security procedures`,
			},
			Description: "Common troubleshooting procedures",
		},
	}

	return LogBasedDashboard{
		Title:  "Network Troubleshooting",
		Tags:   []string{"telecom", "troubleshooting", "errors", "issues"},
		Panels: panels,
		Variables: []DashboardVariable{
			{
				Name:    "time_range",
				Label:   "Time Range",
				Type:    "interval",
				Options: []string{"5m", "15m", "1h", "3h", "6h", "12h", "24h"},
			},
		},
		Time: TimeRange{
			From: "now-1h",
			To:   "now",
		},
		Refresh:     "1m",
		Description: "Identify and troubleshoot common network issues using log analysis. Track authentication failures, timeouts, and radio problems.",
	}
}

// generateEducationalFlowDashboard creates a dashboard for learning 3GPP procedures
func generateEducationalFlowDashboard(topology *discovery.NetworkTopology, insights map[string]any) LogBasedDashboard {
	panels := []LogPanel{
		{
			ID:    1,
			Title: "UE Attach Procedure Steps",
			Type:  "logs",
			Targets: []LokiTarget{
				{
					Expr:       `{component_type="core", imsi="$imsi"} | json | procedure="attach" | line_format "[Step {{.flow_step}}] {{.timestamp}} {{.component}}: {{.student_note}}"`,
					RefID:      "A",
					Datasource: "Loki-SessionFlow",
					MaxLines:   50,
				},
			},
			GridPos:     GridPosition{H: 12, W: 16, X: 0, Y: 0},
			Description: "Follow complete UE attachment with educational explanations",
		},
		{
			ID:      2,
			Title:   "3GPP Specification Reference",
			Type:    "text",
			GridPos: GridPosition{H: 12, W: 8, X: 16, Y: 0},
			Options: map[string]any{
				"content": `## 3GPP Specifications

### 4G Core (EPC):
- **TS 23.401**: Architecture
- **TS 24.301**: NAS Protocol  
- **TS 29.274**: GTPv2-C
- **TS 36.413**: S1AP

### 5G Core:
- **TS 23.501**: Architecture
- **TS 24.501**: NAS Protocol
- **TS 29.244**: PFCP Protocol
- **TS 38.413**: NGAP

### Radio Access:
- **TS 36.331**: LTE RRC
- **TS 38.331**: NR RRC
- **TS 36.321**: LTE MAC
- **TS 38.321**: NR MAC

### Procedures:
- **Attach**: Initial network registration
- **Authentication**: MILENAGE/5G-AKA
- **Handover**: X2/Xn interface
- **Paging**: Idle mode contact`,
			},
			Description: "3GPP specification references for procedures",
		},
		{
			ID:    3,
			Title: "Protocol Message Flow",
			Type:  "nodeGraph",
			Targets: []LokiTarget{
				{
					Expr:       `{component_type="core"} | json | protocol_layer != "" | line_format "{{.component}} -> {{.protocol_layer}} -> {{.procedure}}"`,
					RefID:      "A",
					Datasource: "Loki-Advanced",
					MaxLines:   100,
				},
			},
			GridPos:     GridPosition{H: 8, W: 24, X: 0, Y: 12},
			Description: "Visual representation of protocol message flows between components",
		},
		{
			ID:    4,
			Title: "Learning Objectives Tracker",
			Type:  "stat",
			Targets: []LokiTarget{
				{
					Expr:       `count(count by (procedure) (count_over_time({component_type="core"} | json | procedure != "" [1h])))`,
					RefID:      "A",
					Datasource: "Loki-SessionFlow",
				},
			},
			GridPos: GridPosition{H: 4, W: 12, X: 0, Y: 20},
			Options: map[string]any{
				"colorMode": "background",
				"textMode":  "value_and_name",
			},
			Description: "Different procedures observed in the last hour",
		},
	}

	return LogBasedDashboard{
		Title:  "Educational Protocol Flows",
		Tags:   []string{"education", "3gpp", "procedures", "learning"},
		Panels: panels,
		Variables: []DashboardVariable{
			{
				Name:       "imsi",
				Label:      "Select UE (IMSI)",
				Type:       "query",
				Datasource: "Loki-SessionFlow",
				Query:      `label_values(imsi)`,
				Multi:      false,
			},
			{
				Name:    "specification",
				Label:   "3GPP Specification",
				Type:    "custom",
				Options: []string{"TS 24.301", "TS 24.501", "TS 36.331", "TS 38.331", "TS 36.413", "TS 38.413"},
			},
		},
		Time: TimeRange{
			From: "now-30m",
			To:   "now",
		},
		Refresh:     "30s",
		Description: "Learn 3GPP procedures through real network logs. Follow UE flows with specification references and educational context.",
	}
}

// Helper function to write dashboard JSON files
func WriteLogBasedDashboards(dashboards []LogBasedDashboard) error {
	for _, dashboard := range dashboards {
		filename := fmt.Sprintf(
			"./grafana/dashboards/%s.json",
			strings.ReplaceAll(strings.ToLower(dashboard.Title), " ", "-"),
		)

		jsonData, err := json.MarshalIndent(dashboard, "", "  ")

		if err != nil {
			return fmt.Errorf("failed to marshal dashboard %s: %w", dashboard.Title, err)
		}

		if err := os.WriteFile(filename, jsonData, 0o644); err != nil {
			return fmt.Errorf("failed to write dashboard file %s: %w", filename, err)

		}

		log.Printf("📄 Generated log-based dashboard: %s", filename)
	}

	return nil
}

// Integration with existing dashboard generator
func EnhanceWithLogDashboards(topology *discovery.NetworkTopology, loggingService *logging.LoggingService) error {
	if loggingService == nil {
		log.Printf("⚠️ Logging service not available, skipping log-based dashboards")
		return nil
	}

	// Get logging insights
	insights := logging.GetEducationalInsights(topology)

	// Generate log-based dashboards
	logDashboards, err := GenerateLogBasedDashboards(topology, insights)
	if err != nil {
		return fmt.Errorf("failed to generate log-based dashboards: %w", err)
	}

	// Write dashboard files
	if err := WriteLogBasedDashboards(logDashboards); err != nil {
		return fmt.Errorf("failed to write log-based dashboards: %w", err)
	}

	log.Printf("✅ Enhanced Grafana with %d log-based dashboards", len(logDashboards))
	return nil
}
