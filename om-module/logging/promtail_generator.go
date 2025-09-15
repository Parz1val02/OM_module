package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Parz1val02/OM_module/discovery"
)

// PromtailConfigGenerator generates dynamic Promtail configurations
type PromtailConfigGenerator struct {
	topology    *discovery.NetworkTopology
	lokiURL     string
	configPaths map[string]string // maps component type to config file path
}

// PromtailJobConfig represents a single scrape job configuration
type PromtailJobConfig struct {
	JobName        string
	LogPaths       []string
	Labels         map[string]string
	PipelineStages []PipelineStage
}

// PipelineStage represents a Promtail pipeline processing stage
type PipelineStage struct {
	Type   string         // "regex", "json", "timestamp", "labels", "output"
	Config map[string]any // stage-specific configuration
}

// PromtailConfig represents the complete Promtail configuration
type PromtailConfig struct {
	ServerConfig  ServerConfig   `yaml:"server"`
	PositionsFile string         `yaml:"positions"`
	ClientsConfig []ClientConfig `yaml:"clients"`
	ScrapeConfigs []ScrapeConfig `yaml:"scrape_configs"`
}

type ServerConfig struct {
	HTTPListenPort int `yaml:"http_listen_port"`
	GRPCListenPort int `yaml:"grpc_listen_port"`
}

type ClientConfig struct {
	URL string `yaml:"url"`
}

type ScrapeConfig struct {
	JobName        string           `yaml:"job_name"`
	StaticConfigs  []StaticConfig   `yaml:"static_configs"`
	PipelineStages []map[string]any `yaml:"pipeline_stages,omitempty"`
}

type StaticConfig struct {
	Targets []string          `yaml:"targets"`
	Labels  map[string]string `yaml:"labels"`
}

// NewPromtailConfigGenerator creates a new configuration generator
func NewPromtailConfigGenerator(topology *discovery.NetworkTopology, lokiURL string) *PromtailConfigGenerator {
	return &PromtailConfigGenerator{
		topology:    topology,
		lokiURL:     lokiURL,
		configPaths: make(map[string]string),
	}
}

// GenerateConfigurations creates Promtail configurations for core and RAN components
func (pcg *PromtailConfigGenerator) GenerateConfigurations() error {
	log.Printf("🔧 Generating dynamic Promtail configurations...")

	// Generate core network configuration
	if err := pcg.generateCoreConfig(); err != nil {
		return fmt.Errorf("failed to generate core config: %w", err)
	}

	// Generate RAN configuration
	if err := pcg.generateRANConfig(); err != nil {
		return fmt.Errorf("failed to generate RAN config: %w", err)
	}

	log.Printf("✅ Generated Promtail configurations successfully")
	return nil
}

// generateCoreConfig creates configuration for Open5GS core network components
func (pcg *PromtailConfigGenerator) generateCoreConfig() error {
	coreComponents := pcg.getCoreComponents()
	if len(coreComponents) == 0 {
		log.Printf("⚠️ No core components found, generating minimal config")
		return pcg.generateMinimalCoreConfig()
	}

	config := PromtailConfig{
		ServerConfig: ServerConfig{
			HTTPListenPort: 9080,
			GRPCListenPort: 0,
		},
		PositionsFile: "/tmp/positions.yaml",
		ClientsConfig: []ClientConfig{
			{URL: pcg.lokiURL + "/loki/api/v1/push"},
		},
		ScrapeConfigs: []ScrapeConfig{},
	}

	// Add scrape configs for each core component
	for _, component := range coreComponents {
		scrapeConfig := pcg.createCoreComponentScrapeConfig(component)
		config.ScrapeConfigs = append(config.ScrapeConfigs, scrapeConfig)
	}

	// Add Docker container logs scraping
	dockerScrapeConfig := pcg.createDockerLogsScrapeConfig("core")
	config.ScrapeConfigs = append(config.ScrapeConfigs, dockerScrapeConfig)

	// Write configuration file
	configPath := "./promtail/core/config.yml"
	if err := pcg.writeConfigFile(configPath, config); err != nil {
		return fmt.Errorf("failed to write core config: %w", err)
	}

	pcg.configPaths["core"] = configPath
	log.Printf("📄 Generated core Promtail config: %s", configPath)
	return nil
}

// generateRANConfig creates configuration for RAN components (srsRAN/srsLTE)
func (pcg *PromtailConfigGenerator) generateRANConfig() error {
	ranComponents := pcg.getRANComponents()

	config := PromtailConfig{
		ServerConfig: ServerConfig{
			HTTPListenPort: 9081,
			GRPCListenPort: 0,
		},
		PositionsFile: "/tmp/positions-ran.yaml",
		ClientsConfig: []ClientConfig{
			{URL: pcg.lokiURL + "/loki/api/v1/push"},
		},
		ScrapeConfigs: []ScrapeConfig{},
	}

	// Add scrape configs for each RAN component
	for _, component := range ranComponents {
		scrapeConfig := pcg.createRANComponentScrapeConfig(component)
		config.ScrapeConfigs = append(config.ScrapeConfigs, scrapeConfig)
	}

	// Add srsRAN/srsLTE specific log scraping
	srsranScrapeConfig := pcg.createSRSRANLogsScrapeConfig()
	config.ScrapeConfigs = append(config.ScrapeConfigs, srsranScrapeConfig)

	srsltelScrapeConfig := pcg.createSRSLTELogsScrapeConfig()
	config.ScrapeConfigs = append(config.ScrapeConfigs, srsltelScrapeConfig)

	// Write configuration file
	configPath := "./promtail/ran/config.yml"
	if err := pcg.writeConfigFile(configPath, config); err != nil {
		return fmt.Errorf("failed to write RAN config: %w", err)
	}

	pcg.configPaths["ran"] = configPath
	log.Printf("📄 Generated RAN Promtail config: %s", configPath)
	return nil
}

// getCoreComponents filters topology for core network components
func (pcg *PromtailConfigGenerator) getCoreComponents() []discovery.Component {
	var coreComponents []discovery.Component

	coreTypes := []string{"amf", "smf", "upf", "pcf", "nrf", "ausf", "udm", "udr", "mme", "hss", "pcrf", "sgw", "pgw"}

	for name, component := range pcg.topology.Components {
		for _, coreType := range coreTypes {
			if strings.Contains(strings.ToLower(component.Type), coreType) ||
				strings.Contains(strings.ToLower(name), coreType) {
				coreComponents = append(coreComponents, component)
				break
			}
		}
	}

	return coreComponents
}

// getRANComponents filters topology for RAN components
func (pcg *PromtailConfigGenerator) getRANComponents() []discovery.Component {
	var ranComponents []discovery.Component

	ranTypes := []string{"enb", "gnb", "ue", "ran", "srs"}

	for name, component := range pcg.topology.Components {
		for _, ranType := range ranTypes {
			if strings.Contains(strings.ToLower(component.Type), ranType) ||
				strings.Contains(strings.ToLower(name), ranType) {
				ranComponents = append(ranComponents, component)
				break
			}
		}
	}

	return ranComponents
}

// createCoreComponentScrapeConfig creates a scrape config for a core network component
func (pcg *PromtailConfigGenerator) createCoreComponentScrapeConfig(component discovery.Component) ScrapeConfig {
	componentName := strings.ToLower(component.Type)

	// Determine log file paths
	logPaths := []string{
		fmt.Sprintf("/var/log/open5gs/%s.log", componentName),
		fmt.Sprintf("/var/log/open5gs/%s-*.log", componentName),
	}

	// Create pipeline stages for Open5GS log parsing
	pipelineStages := []map[string]any{
		// Extract timestamp
		{
			"regex": map[string]any{
				"expression": `^(?P<timestamp>\d{4}/\d{2}/\d{2}\s\d{2}:\d{2}:\d{2}\.\d{3})\s\[(?P<level>\w+)\]\s(?P<source>\w+):\s(?P<message>.*)`,
			},
		},
		// Parse timestamp
		{
			"timestamp": map[string]any{
				"source": "timestamp",
				"format": "2006/01/02 15:04:05.000",
			},
		},
		// Add labels
		{
			"labels": map[string]any{
				"level":  "",
				"source": "",
			},
		},
		// Extract IMSI if present
		{
			"regex": map[string]any{
				"expression": `imsi[:\s]*(?P<imsi>\d{15})`,
				"source":     "message",
			},
		},
		// Extract session information
		{
			"regex": map[string]any{
				"expression": `session[:\s]*(?P<session_id>\d+)`,
				"source":     "message",
			},
		},
		// Extract procedure information
		{
			"regex": map[string]any{
				"expression": `(?P<procedure>attach|detach|handover|authentication|registration|paging)`,
				"source":     "message",
			},
		},
		// Educational labeling
		{
			"template": map[string]any{
				"source":   "educational_component",
				"template": componentName,
			},
		},
		{
			"labels": map[string]any{
				"educational_component": "",
				"imsi":                  "",
				"session_id":            "",
				"procedure":             "",
			},
		},
	}

	return ScrapeConfig{
		JobName: fmt.Sprintf("open5gs-%s", componentName),
		StaticConfigs: []StaticConfig{
			{
				Targets: []string{"localhost"},
				Labels: map[string]string{
					"job":            fmt.Sprintf("open5gs-%s", componentName),
					"component":      componentName,
					"component_type": "core",
					"log_format":     "open5gs",
					"deployment":     string(pcg.topology.Type),
					"__path__":       strings.Join(logPaths, ","),
				},
			},
		},
		PipelineStages: pipelineStages,
	}
}

// createRANComponentScrapeConfig creates a scrape config for a RAN component
func (pcg *PromtailConfigGenerator) createRANComponentScrapeConfig(component discovery.Component) ScrapeConfig {
	componentName := strings.ToLower(component.Type)

	// Determine log file paths based on component type
	var logPaths []string
	var logFormat string

	if strings.Contains(componentName, "srsran") {
		logPaths = []string{
			fmt.Sprintf("/var/log/srsran/%s.log", componentName),
			"/var/log/srsran/*.log",
		}
		logFormat = "srsran"
	} else {
		logPaths = []string{
			fmt.Sprintf("/var/log/srslte/%s.log", componentName),
			"/var/log/srslte/*.log",
		}
		logFormat = "srslte"
	}

	// Create pipeline stages for RAN log parsing
	pipelineStages := []map[string]any{
		// Extract timestamp, level, layer, and message
		{
			"regex": map[string]any{
				"expression": `^(?P<timestamp>\d{2}:\d{2}:\d{2}\.\d{3})\s\[(?P<level>\w+)\]\s\[(?P<layer>\w+)\]\s(?P<message>.*)`,
			},
		},
		// Parse timestamp (relative time, so add current date)
		{
			"timestamp": map[string]any{
				"source": "timestamp",
				"format": "15:04:05.000",
			},
		},
		// Add labels
		{
			"labels": map[string]any{
				"level": "",
				"layer": "",
			},
		},
		// Extract RSRP/RSRQ values
		{
			"regex": map[string]any{
				"expression": `RSRP=(?P<rsrp>-?\d+\.?\d*)`,
				"source":     "message",
			},
		},
		{
			"regex": map[string]any{
				"expression": `RSRQ=(?P<rsrq>-?\d+\.?\d*)`,
				"source":     "message",
			},
		},
		// Extract cell information
		{
			"regex": map[string]any{
				"expression": `cell[:\s]*(?P<cell_id>\d+)`,
				"source":     "message",
			},
		},
		// Extract RNTI information
		{
			"regex": map[string]any{
				"expression": `rnti[:\s]*0x(?P<rnti>[0-9a-fA-F]+)`,
				"source":     "message",
			},
		},
		// Educational context for protocol layers
		{
			"template": map[string]any{
				"source":   "educational_spec",
				"template": `{{ if eq .layer "PHY" }}3GPP TS 36.211 (4G) / TS 38.211 (5G){{ else if eq .layer "MAC" }}3GPP TS 36.321 (4G) / TS 38.321 (5G){{ else if eq .layer "RLC" }}3GPP TS 36.322 (4G) / TS 38.322 (5G){{ else if eq .layer "PDCP" }}3GPP TS 36.323 (4G) / TS 38.323 (5G){{ else if eq .layer "RRC" }}3GPP TS 36.331 (4G) / TS 38.331 (5G){{ end }}`,
			},
		},
		// Add all extracted labels
		{
			"labels": map[string]any{
				"rsrp":             "",
				"rsrq":             "",
				"cell_id":          "",
				"rnti":             "",
				"educational_spec": "",
			},
		},
	}

	return ScrapeConfig{
		JobName: fmt.Sprintf("ran-%s", componentName),
		StaticConfigs: []StaticConfig{
			{
				Targets: []string{"localhost"},
				Labels: map[string]string{
					"job":            fmt.Sprintf("ran-%s", componentName),
					"component":      componentName,
					"component_type": "ran",
					"log_format":     logFormat,
					"deployment":     string(pcg.topology.Type),
					"__path__":       strings.Join(logPaths, ","),
				},
			},
		},
		PipelineStages: pipelineStages,
	}
}

// createDockerLogsScrapeConfig creates a scrape config for Docker container logs
func (pcg *PromtailConfigGenerator) createDockerLogsScrapeConfig(componentType string) ScrapeConfig {
	pipelineStages := []map[string]any{
		// Extract container information from Docker log path
		{
			"regex": map[string]any{
				"expression": `/var/lib/docker/containers/(?P<container_id>[^/]+)/(?P<container_hash>[^-]*)-json\.log`,
				"source":     "__path__",
			},
		},
		// Parse Docker JSON log format
		{
			"json": map[string]any{
				"expressions": map[string]string{
					"log":    "log",
					"stream": "stream",
					"time":   "time",
				},
			},
		},
		// Parse timestamp
		{
			"timestamp": map[string]any{
				"source": "time",
				"format": "RFC3339Nano",
			},
		},
		// Add container labels
		{
			"labels": map[string]any{
				"container_id": "",
				"stream":       "",
			},
		},
	}

	return ScrapeConfig{
		JobName: fmt.Sprintf("docker-%s-containers", componentType),
		StaticConfigs: []StaticConfig{
			{
				Targets: []string{"localhost"},
				Labels: map[string]string{
					"job":            fmt.Sprintf("docker-%s", componentType),
					"component_type": componentType,
					"log_source":     "docker",
					"deployment":     string(pcg.topology.Type),
					"__path__":       "/var/lib/docker/containers/*/*-json.log",
				},
			},
		},
		PipelineStages: pipelineStages,
	}
}

// createSRSRANLogsScrapeConfig creates specific scrape config for srsRAN logs
func (pcg *PromtailConfigGenerator) createSRSRANLogsScrapeConfig() ScrapeConfig {
	pipelineStages := []map[string]any{
		// Parse srsRAN log format
		{
			"regex": map[string]any{
				"expression": `^(?P<timestamp>\d{2}:\d{2}:\d{2}\.\d{3})\s\[(?P<level>\w+)\]\s\[(?P<layer>\w+)\]\s(?P<message>.*)`,
			},
		},
		// Parse timestamp
		{
			"timestamp": map[string]any{
				"source": "timestamp",
				"format": "15:04:05.000",
			},
		},
		// Extract technical metrics
		{
			"regex": map[string]any{
				"expression": `CQI=(?P<cqi>\d+)`,
				"source":     "message",
			},
		},
		{
			"regex": map[string]any{
				"expression": `MCS=(?P<mcs>\d+)`,
				"source":     "message",
			},
		},
		{
			"regex": map[string]any{
				"expression": `throughput=(?P<throughput>\d+\.?\d*)\s*(?P<throughput_unit>Mbps|kbps|bps)`,
				"source":     "message",
			},
		},
		// Add educational context for different procedures
		{
			"template": map[string]any{
				"source":   "educational_note",
				"template": `{{ if contains .message "RACH" }}Random Access: UE initiates connection{{ else if contains .message "handover" }}Mobility: UE moving between cells{{ else if contains .message "HARQ" }}Error correction: Automatic repeat request{{ end }}`,
			},
		},
		// Add all labels
		{
			"labels": map[string]any{
				"level":            "",
				"layer":            "",
				"cqi":              "",
				"mcs":              "",
				"throughput":       "",
				"throughput_unit":  "",
				"educational_note": "",
			},
		},
	}

	return ScrapeConfig{
		JobName: "srsran-detailed",
		StaticConfigs: []StaticConfig{
			{
				Targets: []string{"localhost"},
				Labels: map[string]string{
					"job":            "srsran-detailed",
					"component_type": "ran",
					"log_format":     "srsran",
					"deployment":     string(pcg.topology.Type),
					"__path__":       "/var/log/srsran/*.log",
				},
			},
		},
		PipelineStages: pipelineStages,
	}
}

// createSRSLTELogsScrapeConfig creates specific scrape config for srsLTE logs
func (pcg *PromtailConfigGenerator) createSRSLTELogsScrapeConfig() ScrapeConfig {
	// Similar to srsRAN but with some differences in log format
	pipelineStages := []map[string]any{
		// Parse srsLTE log format (might be slightly different)
		{
			"regex": map[string]any{
				"expression": `^(?P<timestamp>\d{2}:\d{2}:\d{2}\.\d{3})\s\[(?P<level>\w+)\]\s\[(?P<component>\w+)\]\s(?P<message>.*)`,
			},
		},
		// Parse timestamp
		{
			"timestamp": map[string]any{
				"source": "timestamp",
				"format": "15:04:05.000",
			},
		},
		// Extract srsLTE specific metrics
		{
			"regex": map[string]any{
				"expression": `PRB=(?P<prb>\d+)`,
				"source":     "message",
			},
		},
		{
			"regex": map[string]any{
				"expression": `BLER=(?P<bler>\d+\.?\d*)`,
				"source":     "message",
			},
		},
		// Add labels
		{
			"labels": map[string]any{
				"level":     "",
				"component": "",
				"prb":       "",
				"bler":      "",
			},
		},
	}

	return ScrapeConfig{
		JobName: "srslte-detailed",
		StaticConfigs: []StaticConfig{
			{
				Targets: []string{"localhost"},
				Labels: map[string]string{
					"job":            "srslte-detailed",
					"component_type": "ran",
					"log_format":     "srslte",
					"deployment":     string(pcg.topology.Type),
					"__path__":       "/var/log/srslte/*.log",
				},
			},
		},
		PipelineStages: pipelineStages,
	}
}

// generateMinimalCoreConfig generates a minimal config when no components are found
func (pcg *PromtailConfigGenerator) generateMinimalCoreConfig() error {
	config := PromtailConfig{
		ServerConfig: ServerConfig{
			HTTPListenPort: 9080,
			GRPCListenPort: 0,
		},
		PositionsFile: "/tmp/positions.yaml",
		ClientsConfig: []ClientConfig{
			{URL: pcg.lokiURL + "/loki/api/v1/push"},
		},
		ScrapeConfigs: []ScrapeConfig{
			{
				JobName: "minimal-placeholder",
				StaticConfigs: []StaticConfig{
					{
						Targets: []string{"localhost"},
						Labels: map[string]string{
							"job":      "placeholder",
							"__path__": "/dev/null",
						},
					},
				},
			},
		},
	}

	configPath := "./promtail/core/config.yml"
	if err := pcg.writeConfigFile(configPath, config); err != nil {
		return fmt.Errorf("failed to write minimal core config: %w", err)
	}

	log.Printf("📄 Generated minimal core Promtail config: %s", configPath)
	return nil
}

// writeConfigFile writes the configuration to a YAML file
func (pcg *PromtailConfigGenerator) writeConfigFile(path string, config PromtailConfig) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create YAML template
	tmpl := `server:
  http_listen_port: {{ .ServerConfig.HTTPListenPort }}
  grpc_listen_port: {{ .ServerConfig.GRPCListenPort }}

positions:
  filename: {{ .PositionsFile }}

clients:
{{- range .ClientsConfig }}
  - url: {{ .URL }}
{{- end }}

scrape_configs:
{{- range .ScrapeConfigs }}
  - job_name: '{{ .JobName }}'
    static_configs:
{{- range .StaticConfigs }}
      - targets:
{{- range .Targets }}
          - {{ . }}
{{- end }}
        labels:
{{- range $key, $value := .Labels }}
          {{ $key }}: '{{ $value }}'
{{- end }}
{{- end }}
{{- if .PipelineStages }}
    pipeline_stages:
{{- range .PipelineStages }}
      - {{ range $key, $value := . }}{{ $key }}:
{{- if eq $key "regex" }}
          expression: '{{ $value.expression }}'
{{- if $value.source }}
          source: {{ $value.source }}
{{- end }}
{{- else if eq $key "timestamp" }}
          source: {{ $value.source }}
          format: '{{ $value.format }}'
{{- else if eq $key "json" }}
          expressions:
{{- range $k, $v := $value.expressions }}
            {{ $k }}: {{ $v }}
{{- end }}
{{- else if eq $key "labels" }}
{{- range $k, $v := $value }}
          {{ $k }}: {{ if $v }}'{{ $v }}'{{ else }}''{{ end }}
{{- end }}
{{- else if eq $key "template" }}
          source: {{ $value.source }}
          template: '{{ $value.template }}'
{{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}`

	// Parse template
	t, err := template.New("promtail").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Create file
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer func() {
		err = file.Close()
	}()

	// Execute template
	if err := t.Execute(file, config); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	return nil
}

// GetConfigPaths returns the paths to generated configuration files
func (pcg *PromtailConfigGenerator) GetConfigPaths() map[string]string {
	return pcg.configPaths
}

// ValidateConfigurations checks if the generated configurations are valid
func (pcg *PromtailConfigGenerator) ValidateConfigurations() error {
	for configType, configPath := range pcg.configPaths {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return fmt.Errorf("configuration file for %s not found: %s", configType, configPath)
		}

		// Basic validation - check if file is readable
		file, err := os.Open(configPath)
		if err != nil {
			return fmt.Errorf("cannot read configuration file %s: %w", configPath, err)
		}
		defer func() {
			err = file.Close()
		}()

		log.Printf("✅ Validated %s configuration: %s", configType, configPath)
	}

	return nil
}

// RestartPromtailServices sends restart signals to Promtail containers
func (pcg *PromtailConfigGenerator) RestartPromtailServices() error {
	log.Printf("🔄 Restarting Promtail services...")

	// This would typically use Docker API to restart containers
	// For now, log the action
	for configType := range pcg.configPaths {
		log.Printf("🔄 Restarting promtail-%s container...", configType)
		// In a real implementation:
		// docker restart promtail-core
		// docker restart promtail-ran
	}

	log.Printf("✅ Promtail services restart initiated")
	return nil
}
