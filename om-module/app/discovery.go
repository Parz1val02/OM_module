package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
	"github.com/Parz1val02/OM_module/export"
	"github.com/Parz1val02/OM_module/logging"
	"github.com/Parz1val02/OM_module/ui"
)

// DiscoveryApp manages the discovery mode application
type DiscoveryApp struct {
	config           *Config
	discoveryService *discovery.AutoDiscoveryService
	topology         *discovery.NetworkTopology
}

// NewDiscoveryApp creates a new discovery application
func NewDiscoveryApp(cfg *Config) (*DiscoveryApp, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Create discovery service
	discoveryService, err := discovery.NewAutoDiscoveryService(cfg.EnvFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery service: %w", err)
	}

	return &DiscoveryApp{
		config:           cfg,
		discoveryService: discoveryService,
	}, nil
}

// Run executes the discovery mode
func (app *DiscoveryApp) Run() error {
	ui.PrintBanner()
	ui.PrintDiscoveryModeDescription()

	// Create context with timeout for discovery operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Analyze topology
	if err := app.analyzeTopology(ctx); err != nil {
		return fmt.Errorf("topology analysis failed: %w", err)
	}

	// Step 2: Generate configurations
	if err := app.generateConfigurations(); err != nil {
		return fmt.Errorf("configuration generation failed: %w", err)
	}

	// Step 3: Generate logging configurations
	if err := app.generateLoggingConfigurations(); err != nil {
		log.Printf("⚠️ Failed to generate logging configurations: %v", err)
		// Continue even if logging config generation fails
	}

	// Display results
	app.displayResults()

	log.Printf("🎯 Discovery mode completed successfully!")
	log.Printf("📁 All configuration files are ready for use with Prometheus/Grafana")

	if app.config.InDocker {
		log.Printf("🐳 To start live monitoring: docker-compose restart om-module")
	} else {
		log.Printf("🚀 To start live monitoring, run: ./om-module orchestrator")
	}

	return app.Cleanup()
}

// analyzeTopology discovers and analyzes the network topology
func (app *DiscoveryApp) analyzeTopology(ctx context.Context) error {
	log.Printf("🔍 Step 1: Analyzing Docker containers and network topology...")

	topology, err := app.discoveryService.DiscoverTopology(ctx)
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	app.topology = topology

	log.Printf("✅ Step 1 Complete: Found %d components in %s deployment",
		len(topology.Components), topology.Type)

	return nil
}

// generateConfigurations generates static configuration files
func (app *DiscoveryApp) generateConfigurations() error {
	log.Printf("🔧 Step 2: Generating static configuration files...")

	// Export topology and configuration files
	export.ExportTopologyAndConfig(app.topology)

	log.Printf("✅ Step 2 Complete: Configuration files generated")
	return nil
}

// generateLoggingConfigurations generates logging configurations
func (app *DiscoveryApp) generateLoggingConfigurations() error {
	log.Printf("📝 Step 3: Generating dynamic logging configurations...")

	// Load configuration from environment
	cfg := logging.LoadConfigFromEnv()

	// Create logging service but don't start log parser in discovery mode
	cfg.ParserEnabled = false

	tempLoggingService := logging.NewLoggingService(app.topology, cfg)

	// Generate configurations without starting the full service
	if err := tempLoggingService.Start(); err != nil {
		return fmt.Errorf("failed to generate logging configurations: %w", err)
	}

	// Stop the service after configuration generation
	if err := tempLoggingService.Stop(); err != nil {
		return fmt.Errorf("failed to stop logging service: %w", err)
	}

	log.Printf("✅ Step 3 Complete: Logging configurations generated")

	// Generate educational content
	export.GenerateEducationalContent(app.topology)

	return nil
}

// displayResults displays comprehensive discovery results
func (app *DiscoveryApp) displayResults() {
	// Display discovery results
	ui.DisplayDiscoveryResults(app.topology)

	// Display logging setup results
	ui.DisplayLoggingResults()

	// Display next steps
	ui.PrintRealMetricsNextSteps()
}

// Cleanup cleans up resources
func (app *DiscoveryApp) Cleanup() error {
	if app.discoveryService != nil {
		if err := app.discoveryService.Close(); err != nil {
			return fmt.Errorf("failed to close discovery service: %w", err)
		}
	}
	return nil
}
