package logging

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
)

// LoggingService manages the complete logging pipeline
type LoggingService struct {
	topology          *discovery.NetworkTopology
	logParser         *LogParser
	promtailGenerator *PromtailConfigGenerator
	ctx               context.Context
	cancel            context.CancelFunc
	mu                sync.RWMutex

	// Configuration
	lokiURL         string
	educationalMode bool
	parserEnabled   bool

	// Status tracking
	isRunning        bool
	startTime        time.Time
	lastConfigUpdate time.Time
}

// LoggingServiceConfig holds configuration for the logging service
type LoggingServiceConfig struct {
	LokiURL         string
	EducationalMode bool
	ParserEnabled   bool
}

// NewLoggingService creates a new logging service instance
func NewLoggingService(topology *discovery.NetworkTopology, config LoggingServiceConfig) *LoggingService {
	ctx, cancel := context.WithCancel(context.Background())

	return &LoggingService{
		topology:         topology,
		ctx:              ctx,
		cancel:           cancel,
		lokiURL:          config.LokiURL,
		educationalMode:  config.EducationalMode,
		parserEnabled:    config.ParserEnabled,
		lastConfigUpdate: time.Now(),
	}
}

// Start initializes and starts the complete logging pipeline
func (ls *LoggingService) Start() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.isRunning {
		return fmt.Errorf("logging service is already running")
	}

	log.Printf("🚀 Starting Logging Service...")
	log.Printf("   ├─ Educational Mode: %v", ls.educationalMode)
	log.Printf("   ├─ Parser Enabled: %v", ls.parserEnabled)
	log.Printf("   └─ Loki URL: %s", ls.lokiURL)

	// Step 1: Generate Promtail configurations
	if err := ls.generatePromtailConfigs(); err != nil {
		return fmt.Errorf("failed to generate Promtail configs: %w", err)
	}

	// Step 2: Start log parser if enabled
	if ls.parserEnabled {
		if err := ls.startLogParser(); err != nil {
			return fmt.Errorf("failed to start log parser: %w", err)
		}
	}

	// Step 3: Validate and restart Promtail services
	if err := ls.restartPromtailServices(); err != nil {
		log.Printf("⚠️ Failed to restart Promtail services: %v", err)
		// Don't fail the entire service for this
	}

	ls.isRunning = true
	ls.startTime = time.Now()

	log.Printf("✅ Logging Service started successfully")

	// Start background monitoring
	go ls.monitorService()

	return nil
}

// generatePromtailConfigs creates dynamic Promtail configurations
func (ls *LoggingService) generatePromtailConfigs() error {
	log.Printf("🔧 Generating Promtail configurations...")

	ls.promtailGenerator = NewPromtailConfigGenerator(ls.topology, ls.lokiURL)

	if err := ls.promtailGenerator.GenerateConfigurations(); err != nil {
		return fmt.Errorf("failed to generate configurations: %w", err)
	}

	// Validate generated configurations
	if err := ls.promtailGenerator.ValidateConfigurations(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	ls.lastConfigUpdate = time.Now()
	log.Printf("✅ Promtail configurations generated and validated")

	return nil
}

// startLogParser initializes and starts the log parser
func (ls *LoggingService) startLogParser() error {
	log.Printf("🔄 Starting log parser...")

	ls.logParser = NewLogParser(ls.topology, ls.lokiURL, ls.educationalMode)

	if err := ls.logParser.Start(); err != nil {
		return fmt.Errorf("failed to start log parser: %w", err)
	}

	log.Printf("✅ Log parser started successfully")
	return nil
}

// restartPromtailServices restarts Promtail containers with new configurations
func (ls *LoggingService) restartPromtailServices() error {
	if ls.promtailGenerator == nil {
		return fmt.Errorf("promtail generator not initialized")
	}

	return ls.promtailGenerator.RestartPromtailServices()
}

// monitorService runs background monitoring and health checks
func (ls *LoggingService) monitorService() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ls.performHealthChecks()

		case <-ls.ctx.Done():
			return
		}
	}
}

// performHealthChecks checks the health of all logging components
func (ls *LoggingService) performHealthChecks() {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if !ls.isRunning {
		return
	}

	// Check if Loki is accessible
	go ls.checkLokiHealth()

	// Check log parser health if enabled
	if ls.parserEnabled && ls.logParser != nil {
		go ls.checkParserHealth()
	}

	// Check configuration files
	go ls.checkConfigurationHealth()
}

// checkLokiHealth verifies Loki connectivity
func (ls *LoggingService) checkLokiHealth() {
	// This would implement actual health check
	// For now, just log the check
	log.Printf("🏥 Health check: Loki connectivity")
}

// checkParserHealth verifies log parser status
func (ls *LoggingService) checkParserHealth() {
	// This would check parser HTTP endpoint
	log.Printf("🏥 Health check: Log parser status")
}

// checkConfigurationHealth verifies configuration files
func (ls *LoggingService) checkConfigurationHealth() {
	if ls.promtailGenerator == nil {
		return
	}

	if err := ls.promtailGenerator.ValidateConfigurations(); err != nil {
		log.Printf("⚠️ Configuration health check failed: %v", err)
	}
}

// UpdateTopology updates the topology and regenerates configurations
func (ls *LoggingService) UpdateTopology(newTopology *discovery.NetworkTopology) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	log.Printf("🔄 Updating topology and regenerating configurations...")

	ls.topology = newTopology

	// Regenerate Promtail configurations
	if err := ls.generatePromtailConfigs(); err != nil {
		return fmt.Errorf("failed to regenerate configurations: %w", err)
	}

	// Update log parser if running
	if ls.parserEnabled && ls.logParser != nil {
		// Stop current parser
		ls.logParser.Stop()

		// Start new parser with updated topology
		if err := ls.startLogParser(); err != nil {
			log.Printf("⚠️ Failed to restart log parser: %v", err)
			// Continue without parser
		}
	}

	// Restart Promtail services
	if err := ls.restartPromtailServices(); err != nil {
		log.Printf("⚠️ Failed to restart Promtail services: %v", err)
	}

	log.Printf("✅ Topology updated and configurations regenerated")
	return nil
}

// GetStatus returns the current status of the logging service
func (ls *LoggingService) GetStatus() map[string]any {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	status := map[string]any{
		"running":            ls.isRunning,
		"educational_mode":   ls.educationalMode,
		"parser_enabled":     ls.parserEnabled,
		"loki_url":           ls.lokiURL,
		"last_config_update": ls.lastConfigUpdate.Format(time.RFC3339),
	}

	if ls.isRunning {
		status["uptime"] = time.Since(ls.startTime).String()
	}

	if ls.topology != nil {
		status["components_count"] = len(ls.topology.Components)
		status["deployment_type"] = ls.topology.Type
	}

	if ls.promtailGenerator != nil {
		status["config_paths"] = ls.promtailGenerator.GetConfigPaths()
	}

	return status
}

// GetPromtailConfigs returns the generated Promtail configuration content
func (ls *LoggingService) GetPromtailConfigs() (map[string]string, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if ls.promtailGenerator == nil {
		return nil, fmt.Errorf("promtail generator not initialized")
	}

	configs := make(map[string]string)
	configPaths := ls.promtailGenerator.GetConfigPaths()

	for configType, path := range configPaths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read config %s: %w", configType, err)
		}
		configs[configType] = string(content)
	}

	return configs, nil
}

// Stop gracefully stops the logging service
func (ls *LoggingService) Stop() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if !ls.isRunning {
		return fmt.Errorf("logging service is not running")
	}

	log.Printf("🛑 Stopping Logging Service...")

	// Stop log parser
	if ls.logParser != nil {
		ls.logParser.Stop()
	}

	// Cancel context to stop background tasks
	ls.cancel()

	ls.isRunning = false

	log.Printf("✅ Logging Service stopped")
	return nil
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() LoggingServiceConfig {
	config := LoggingServiceConfig{
		LokiURL:         getEnvString("LOKI_URL", "http://loki:3100"),
		EducationalMode: getEnvBool("EDUCATIONAL_MODE", true),
		ParserEnabled:   getEnvBool("LOG_PARSER_ENABLED", true),
	}

	return config
}

// Helper functions for environment variable parsing
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// Educational helper functions for log analysis
func GetEducationalInsights(topology *discovery.NetworkTopology) map[string]any {
	insights := map[string]any{
		"learning_objectives": []string{
			"Understand 4G/5G protocol stack interactions",
			"Analyze network function communication flows",
			"Troubleshoot common telecom issues",
			"Monitor key performance indicators (KPIs)",
		},
		"protocol_layers": map[string]string{
			"NAS":  "Non-Access Stratum - Authentication and session management",
			"RRC":  "Radio Resource Control - Radio bearer establishment",
			"PDCP": "Packet Data Convergence Protocol - Header compression",
			"RLC":  "Radio Link Control - Automatic repeat request",
			"MAC":  "Medium Access Control - Resource scheduling",
			"PHY":  "Physical Layer - Radio signal processing",
		},
		"key_procedures": map[string]string{
			"attach":         "UE initial network registration",
			"handover":       "UE mobility between cells",
			"authentication": "Network security verification",
			"paging":         "Network-initiated UE contact",
		},
		"troubleshooting_tips": []string{
			"Check IMSI consistency across log entries",
			"Monitor RSRP/RSRQ values for signal quality",
			"Track session IDs for complete call flows",
			"Look for error patterns in authentication flows",
		},
	}

	if topology != nil {
		insights["discovered_components"] = len(topology.Components)
		insights["deployment_architecture"] = topology.Type

		// Analyze component types for educational context
		componentTypes := make(map[string]int)
		for _, component := range topology.Components {
			componentTypes[component.Type]++
		}
		insights["component_distribution"] = componentTypes
	}

	return insights
}

// GenerateEducationalDashboard creates educational content for students
func GenerateEducationalDashboard(topology *discovery.NetworkTopology) string {
	dashboard := `# 📚 O&M Educational Dashboard

## 🎯 Learning Objectives
- Understand real-time telecom network operations
- Analyze protocol interactions in 4G/5G networks
- Practice troubleshooting methodologies
- Monitor network performance indicators

## 🔍 What You're Monitoring

### Core Network Components
`

	if topology != nil {
		coreComponents := []string{}
		ranComponents := []string{}

		for name, component := range topology.Components {
			if isCoreComponent(component.Type) {
				coreComponents = append(coreComponents, fmt.Sprintf("- **%s** (%s): %s", name, component.Type, component.IP))
			} else if isRANComponent(component.Type) {
				ranComponents = append(ranComponents, fmt.Sprintf("- **%s** (%s): %s", name, component.Type, component.IP))
			}
		}

		for _, comp := range coreComponents {
			dashboard += comp + "\n"
		}

		dashboard += "\n### Radio Access Network (RAN)\n"
		for _, comp := range ranComponents {
			dashboard += comp + "\n"
		}
	}

	dashboard += `
## 📊 Key Metrics to Watch

### Signal Quality (RAN)
- **RSRP**: Reference Signal Received Power (-140 to -44 dBm)
- **RSRQ**: Reference Signal Received Quality (-19.5 to -3 dB)
- **CQI**: Channel Quality Indicator (0-15, higher is better)

### Session Management (Core)
- **Attach Success Rate**: % of successful UE attachments
- **Authentication Failures**: Failed security procedures
- **Handover Success Rate**: % of successful mobility events

## 🔧 Troubleshooting Workflow

1. **Identify the Issue**
   - Check error logs for specific IMSI
   - Look for timeout patterns
   - Monitor success/failure rates

2. **Analyze Protocol Flow**
   - Trace session from attach to release
   - Check each protocol layer interaction
   - Verify message sequence correctness

3. **Performance Analysis**
   - Compare metrics against thresholds
   - Look for degradation patterns
   - Correlate multiple component logs

## 📖 Protocol Quick Reference

### NAS Messages (Non-Access Stratum)
- **Attach Request**: UE requests network access
- **Authentication Request/Response**: Security verification
- **Security Mode Command**: Encryption activation
- **Attach Complete**: Successful registration

### RRC Messages (Radio Resource Control)
- **RRC Connection Request**: UE requests radio resources
- **RRC Connection Setup**: Network allocates resources
- **RRC Connection Reconfiguration**: Modify radio parameters
- **RRC Connection Release**: Release radio resources

## 🚨 Common Issues to Look For

### Authentication Problems
Pattern: "Authentication failed" + specific IMSI
Cause: Wrong security keys or network configuration
Solution: Check HSS/UDM subscriber data

### Radio Issues
Pattern: Poor RSRP/RSRQ + frequent handovers
Cause: Radio coverage problems
Solution: Check antenna configuration or positioning

### Core Network Overload
Pattern: Timeouts + high session counts
Cause: Component capacity limits
Solution: Monitor CPU/memory usage, scale if needed

## 💡 Educational Tips

- **Follow Complete Flows**: Don't just look at error logs, trace the complete procedure
- **Correlate Timestamps**: Match events across different components using timestamps
- **Understand Normal vs Abnormal**: Learn what normal patterns look like first
- **Use IMSI as Thread**: Follow specific UE through entire network interaction

## 🔗 Specifications Reference

- **3GPP TS 24.301**: NAS Protocol for 4G
- **3GPP TS 24.501**: NAS Protocol for 5G  
- **3GPP TS 36.331**: RRC Protocol for 4G
- **3GPP TS 38.331**: RRC Protocol for 5G
- **3GPP TS 36.413**: S1AP Interface
- **3GPP TS 38.413**: NGAP Interface
`

	return dashboard
}

// Helper functions for component classification
func isCoreComponent(componentType string) bool {
	coreTypes := []string{"amf", "smf", "upf", "pcf", "nrf", "ausf", "udm", "udr", "mme", "hss", "pcrf", "sgw", "pgw"}
	compType := strings.ToLower(componentType)

	for _, coreType := range coreTypes {
		if strings.Contains(compType, coreType) {
			return true
		}
	}
	return false
}

func isRANComponent(componentType string) bool {
	ranTypes := []string{"enb", "gnb", "ue", "ran", "srs"}
	compType := strings.ToLower(componentType)

	for _, ranType := range ranTypes {
		if strings.Contains(compType, ranType) {
			return true
		}
	}
	return false
}
