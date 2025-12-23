package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Parz1val02/OM_module/config"
	"github.com/Parz1val02/OM_module/dashboards"
	"github.com/Parz1val02/OM_module/discovery"
	"github.com/Parz1val02/OM_module/export"
	"github.com/Parz1val02/OM_module/logging"
	"github.com/Parz1val02/OM_module/metrics"
	"github.com/Parz1val02/OM_module/server"
	"github.com/Parz1val02/OM_module/ui"
)

var debugMode bool
var loggingService *logging.LoggingService
var currentTopology *discovery.NetworkTopology

func main() {
	log.Printf("🚀 Starting O&M Module for 4G/5G Educational Network Testbed")

	// Parse command line arguments
	mode := "discovery"
	envFile := "../.env"

	// Detect if running in Docker and adjust env file path
	if ui.IsRunningInDocker() {
		envFile = ".env"
		log.Printf("🐳 Running in Docker environment")
	}

	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if len(os.Args) > 2 {
		if os.Args[2] == "--debug" {
			debugMode = true
			log.Printf("🐞 Debug mode enabled")
		} else {
			envFile = os.Args[2]
		}
	}

	switch mode {
	case "orchestrator":
		runRealMetricsOrchestrator(envFile)
	case "discovery":
		runDiscoveryMode(envFile)
	default:
		ui.PrintBanner()
		ui.PrintUsage()
		os.Exit(1)
	}
}

// Real-time metrics orchestrator mode - Live monitoring and collection
func runRealMetricsOrchestrator(envFile string) {
	ui.PrintBanner()
	ui.PrintOrchestratorModeDescription()

	// Detect environment
	inDocker := ui.IsRunningInDocker()
	if inDocker {
		log.Printf("🐳 Docker environment detected - using Docker networking")
	}

	// Create discovery service
	discoveryService, err := discovery.NewAutoDiscoveryService(envFile)
	if err != nil {
		log.Fatalf("❌ Failed to create discovery service: %v", err)
	}
	defer discoveryService.Close()

	// Create context for all operations
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Discover topology first
	log.Printf("🔄 Discovering network topology...")
	topology, err := discoveryService.DiscoverTopology(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to discover topology: %v", err)
	}
	currentTopology = topology

	log.Printf("🔄 Initializing real-time metrics collection system...")

	// Create real collector orchestrator
	orchestrator := metrics.NewRealCollectorOrchestrator(discoveryService)

	// Start the real Open5GS orchestrator
	if err := orchestrator.Start(); err != nil {
		log.Fatalf("❌ Failed to start real metrics orchestrator: %v", err)
	}

	// Initialize and start logging service
	log.Printf("📝 Initializing dynamic logging pipeline...")
	if err := initializeLoggingService(topology); err != nil {
		log.Printf("⚠️ Failed to initialize logging service: %v", err)
		// Continue without logging service - don't fail the entire application
	} else {
		log.Printf("✅ Logging pipeline initialized successfully")
	}

	// Start infrastructure collectors
	log.Printf("🔄 Starting infrastructure collectors...")

	// Start container metrics collector
	containerCollector, err := metrics.NewContainerMetricsCollector(8080)
	if err != nil {
		log.Printf("⚠️ Failed to create container collector: %v", err)
	} else {
		go func() {
			if err := containerCollector.Start(ctx, topology); err != nil && err != context.Canceled {
				log.Printf("❌ Container collector error: %v", err)
			}
		}()
		log.Printf("🟢 Started container metrics collector on :8080")
	}

	// Start health check collector
	healthCollector := metrics.NewHealthCheckCollector(8081)
	go func() {
		if err := healthCollector.Start(ctx, topology); err != nil && err != context.Canceled {
			log.Printf("❌ Health collector error: %v", err)
		}
	}()
	log.Printf("🟢 Started health check collector on :8081")

	// Generate Docker-aware Prometheus configuration
	if err := config.GenerateDockerPrometheusConfig(orchestrator, topology, inDocker); err != nil {
		log.Printf("⚠️ Failed to generate Prometheus config: %v", err)
	}

	// Wait for collectors to be healthy
	log.Printf("⏳ Performing health checks on all collectors...")
	if err := orchestrator.WaitForHealthy(30 * time.Second); err != nil {
		log.Printf("⚠️  Warning: Some collectors may not be fully ready: %v", err)
	}

	// Generate Grafana dashboards
	if err := dashboards.GenerateGrafanaDashboards(orchestrator, topology, inDocker); err != nil {
		log.Printf("⚠️ Failed to generate Grafana dashboards: %v", err)
	} else {
		log.Printf("✅ Generated Grafana dashboards successfully")
	}

	// Start HTTP server for logging endpoints
	go startLoggingHTTPServer(topology)

	// Display live status
	ui.DisplayRealMetricsStatus(orchestrator, loggingService, debugMode)

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("🌟 Orchestrator mode is now LIVE! All metrics are being collected in real-time.")
	log.Printf("📊 Access Prometheus metrics at the endpoints shown above")
	log.Printf("📝 Access logging service at http://localhost:8080/logging/*")
	if inDocker {
		log.Printf("🐳 Docker mode: Prometheus config written to shared volume")
	}
	log.Printf("⚡ Press Ctrl+C to stop the orchestrator...")

	// Wait for shutdown signal
	<-sigChan
	log.Printf("🛑 Received shutdown signal, gracefully stopping all collectors...")

	// Cancel context to stop all collectors
	cancel()

	// Stop services
	orchestrator.Stop()
	cleanupLoggingService()

	// Give collectors time to stop gracefully
	time.Sleep(2 * time.Second)

	log.Printf("✅ Real-time metrics orchestrator stopped cleanly")
}

// Initialize logging service
func initializeLoggingService(topology *discovery.NetworkTopology) error {
	log.Printf("🔧 Initializing Logging Service...")

	// Load configuration from environment
	cfg := logging.LoadConfigFromEnv()

	// Create logging service
	loggingService = logging.NewLoggingService(topology, cfg)

	// Start the service
	if err := loggingService.Start(); err != nil {
		return fmt.Errorf("failed to start logging service: %w", err)
	}

	log.Printf("✅ Logging Service initialized successfully")
	return nil
}

// Start HTTP server for logging endpoints
func startLoggingHTTPServer(topology *discovery.NetworkTopology) {
	httpServer := server.NewHTTPServer(loggingService)
	httpServer.SetTopology(topology)
	if err := httpServer.Start("8083"); err != nil {
		log.Printf("⚠️ Logging HTTP server error: %v", err)
	}
}

// cleanupLoggingService should be called during application shutdown
func cleanupLoggingService() {
	if loggingService != nil {
		if err := loggingService.Stop(); err != nil {
			log.Printf("⚠️ Error stopping logging service: %v", err)
		}
	}
}

// updateTopologyInLoggingService updates the logging service when topology changes
func updateTopologyInLoggingService(newTopology *discovery.NetworkTopology) {
	if loggingService != nil {
		if err := loggingService.UpdateTopology(newTopology); err != nil {
			log.Printf("⚠️ Failed to update logging service topology: %v", err)
		}
	}
}

// Discovery mode - Static analysis and configuration generation
func runDiscoveryMode(envFile string) {
	ui.PrintBanner()
	ui.PrintDiscoveryModeDescription()

	// Create discovery service
	discoveryService, err := discovery.NewAutoDiscoveryService(envFile)
	if err != nil {
		log.Fatalf("❌ Failed to create discovery service: %v", err)
	}
	defer discoveryService.Close()

	// Create context with timeout for discovery operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("🔍 Step 1: Analyzing Docker containers and network topology...")

	// Discover topology
	topology, err := discoveryService.DiscoverTopology(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to discover topology: %v", err)
	}

	log.Printf("✅ Step 1 Complete: Found %d components in %s deployment",
		len(topology.Components), topology.Type)

	log.Printf("🔧 Step 2: Generating static configuration files...")

	// Export topology and configuration files
	export.ExportTopologyAndConfig(topology)

	log.Printf("✅ Step 2 Complete: Configuration files generated")

	// Step 3 - Initialize logging service for configuration generation
	log.Printf("📝 Step 3: Generating dynamic logging configurations...")
	if err := initializeLoggingServiceForDiscovery(topology); err != nil {
		log.Printf("⚠️ Failed to generate logging configurations: %v", err)
	} else {
		log.Printf("✅ Step 3 Complete: Logging configurations generated")

		// Generate educational content
		export.GenerateEducationalContent(topology)
	}

	// Display comprehensive discovery results
	ui.DisplayDiscoveryResults(topology)

	// Display logging setup results
	ui.DisplayLoggingResults()

	// Display next steps for real metrics
	ui.PrintRealMetricsNextSteps()

	log.Printf("🎯 Discovery mode completed successfully!")
	log.Printf("📁 All configuration files are ready for use with Prometheus/Grafana")
	if ui.IsRunningInDocker() {
		log.Printf("🐳 To start live monitoring: docker-compose restart om-module")
	} else {
		log.Printf("🚀 To start live monitoring, run: ./om-module orchestrator")
	}
}

// Initialize logging service for discovery mode (config generation only)
func initializeLoggingServiceForDiscovery(topology *discovery.NetworkTopology) error {
	cfg := logging.LoadConfigFromEnv()

	// Create logging service but don't start log parser in discovery mode
	cfg.ParserEnabled = false

	tempLoggingService := logging.NewLoggingService(topology, cfg)

	// Generate configurations without starting the full service
	if err := tempLoggingService.Start(); err != nil {
		return fmt.Errorf("failed to generate logging configurations: %w", err)
	}

	// Stop the service after configuration generation
	if err := tempLoggingService.Stop(); err != nil {
		return fmt.Errorf("failed to stop logging configurations: %w", err)
	}

	return nil
}

// getCurrentTopology returns the current topology
func getCurrentTopology() *discovery.NetworkTopology {
	return currentTopology
}
