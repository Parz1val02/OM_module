package app

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
	"github.com/Parz1val02/OM_module/logging"
	"github.com/Parz1val02/OM_module/metrics"
	"github.com/Parz1val02/OM_module/server"
	"github.com/Parz1val02/OM_module/ui"
)

// OrchestratorApp manages the orchestrator mode application
type OrchestratorApp struct {
	config              *Config
	discoveryService    *discovery.AutoDiscoveryService
	metricsOrchestrator *metrics.RealCollectorOrchestrator
	loggingService      *logging.LoggingService
	httpServer          *server.HTTPServer
	topology            *discovery.NetworkTopology
	containerCollector  *metrics.ContainerMetricsCollector
	healthCollector     *metrics.HealthCheckCollector
	ctx                 context.Context
	cancel              context.CancelFunc
}

// NewOrchestratorApp creates a new orchestrator application
func NewOrchestratorApp(cfg *Config) (*OrchestratorApp, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Create discovery service
	discoveryService, err := discovery.NewAutoDiscoveryService(cfg.EnvFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery service: %w", err)
	}

	// Create context for application lifecycle
	ctx, cancel := context.WithCancel(context.Background())

	return &OrchestratorApp{
		config:           cfg,
		discoveryService: discoveryService,
		ctx:              ctx,
		cancel:           cancel,
	}, nil
}

// Start starts the orchestrator application
func (app *OrchestratorApp) Start() error {
	ui.PrintBanner()
	ui.PrintOrchestratorModeDescription()

	if app.config.InDocker {
		log.Printf("🐳 Docker environment detected - using Docker networking")
	}

	// Discover topology
	if err := app.discoverTopology(); err != nil {
		return fmt.Errorf("topology discovery failed: %w", err)
	}

	// Initialize metrics collection
	if err := app.initializeMetrics(); err != nil {
		return fmt.Errorf("metrics initialization failed: %w", err)
	}

	// Initialize logging service
	if err := app.initializeLogging(); err != nil {
		log.Printf("⚠️ Failed to initialize logging service: %v", err)
		// Continue without logging - not critical
	}

	// Start infrastructure collectors
	if err := app.startInfrastructureCollectors(); err != nil {
		log.Printf("⚠️ Failed to start infrastructure collectors: %v", err)
		// Continue even if some collectors fail
	}

	// Generate configurations
	if err := app.generateConfigurations(); err != nil {
		log.Printf("⚠️ Failed to generate configurations: %v", err)
		// Continue even if config generation fails
	}

	// Start HTTP server
	if err := app.startHTTPServer(); err != nil {
		log.Printf("⚠️ Failed to start HTTP server: %v", err)
		// Continue even if HTTP server fails
	}

	// Display status
	ui.DisplayRealMetricsStatus(app.metricsOrchestrator, app.loggingService, app.config.DebugMode)

	// Run until shutdown signal
	return app.run()
}

// discoverTopology discovers the network topology
func (app *OrchestratorApp) discoverTopology() error {
	log.Printf("🔄 Discovering network topology...")

	topology, err := app.discoveryService.DiscoverTopology(app.ctx)
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	app.topology = topology
	log.Printf("✅ Discovered %d components in %s deployment", len(topology.Components), topology.Type)
	return nil
}

// initializeMetrics initializes the metrics collection system
func (app *OrchestratorApp) initializeMetrics() error {
	log.Printf("🔄 Initializing real-time metrics collection system...")

	// Create orchestrator
	app.metricsOrchestrator = metrics.NewRealCollectorOrchestrator(app.discoveryService)

	// Start the orchestrator
	if err := app.metricsOrchestrator.Start(); err != nil {
		return fmt.Errorf("failed to start metrics orchestrator: %w", err)
	}

	// Wait for collectors to be healthy
	log.Printf("⏳ Performing health checks on all collectors...")
	if err := app.metricsOrchestrator.WaitForHealthy(30 * time.Second); err != nil {
		log.Printf("⚠️ Warning: Some collectors may not be fully ready: %v", err)
	}

	log.Printf("✅ Metrics collection system initialized")
	return nil
}

// initializeLogging initializes the logging service
func (app *OrchestratorApp) initializeLogging() error {
	log.Printf("📝 Initializing dynamic logging pipeline...")

	// Load configuration from environment
	cfg := logging.LoadConfigFromEnv()

	// Create logging service
	app.loggingService = logging.NewLoggingService(app.topology, cfg)

	// Start the service
	if err := app.loggingService.Start(); err != nil {
		return fmt.Errorf("failed to start logging service: %w", err)
	}

	log.Printf("✅ Logging pipeline initialized successfully")
	return nil
}

// startInfrastructureCollectors starts container and health collectors
func (app *OrchestratorApp) startInfrastructureCollectors() error {
	log.Printf("🔄 Starting infrastructure collectors...")

	// Start container metrics collector
	containerCollector, err := metrics.NewContainerMetricsCollector(8080)
	if err != nil {
		return fmt.Errorf("failed to create container collector: %w", err)
	}

	app.containerCollector = containerCollector
	go func() {
		if err := app.containerCollector.Start(app.ctx, app.topology); err != nil && err != context.Canceled {
			log.Printf("❌ Container collector error: %v", err)
		}
	}()
	log.Printf("🟢 Started container metrics collector on :8080")

	// Start health check collector
	app.healthCollector = metrics.NewHealthCheckCollector(8081)
	go func() {
		if err := app.healthCollector.Start(app.ctx, app.topology); err != nil && err != context.Canceled {
			log.Printf("❌ Health collector error: %v", err)
		}
	}()
	log.Printf("🟢 Started health check collector on :8081")

	return nil
}

// generateConfigurations generates Prometheus and Grafana configurations
func (app *OrchestratorApp) generateConfigurations() error {
	// Generate Prometheus configuration
	if err := config.GenerateDockerPrometheusConfig(app.metricsOrchestrator, app.topology, app.config.InDocker); err != nil {
		return fmt.Errorf("failed to generate Prometheus config: %w", err)
	}

	// Generate Grafana dashboards
	if err := dashboards.GenerateGrafanaDashboards(app.metricsOrchestrator, app.topology, app.config.InDocker); err != nil {
		return fmt.Errorf("failed to generate Grafana dashboards: %w", err)
	}

	log.Printf("✅ Generated Grafana dashboards successfully")
	return nil
}

// startHTTPServer starts the HTTP server for logging endpoints
func (app *OrchestratorApp) startHTTPServer() error {
	app.httpServer = server.NewHTTPServer(app.loggingService)
	app.httpServer.SetTopology(app.topology)

	go func() {
		if err := app.httpServer.Start("8083"); err != nil {
			log.Printf("⚠️ Logging HTTP server error: %v", err)
		}
	}()

	return nil
}

// run keeps the application running until a shutdown signal is received
func (app *OrchestratorApp) run() error {
	log.Printf("🌟 Orchestrator mode is now LIVE! All metrics are being collected in real-time.")
	log.Printf("📊 Access Prometheus metrics at the endpoints shown above")
	log.Printf("📝 Access logging service at http://localhost:8083/logging/*")
	if app.config.InDocker {
		log.Printf("🐳 Docker mode: Prometheus config written to shared volume")
	}
	log.Printf("⚡ Press Ctrl+C to stop the orchestrator...")

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigChan
	log.Printf("🛑 Received shutdown signal, gracefully stopping all collectors...")

	return app.Shutdown()
}

// Shutdown gracefully shuts down the application
func (app *OrchestratorApp) Shutdown() error {
	log.Printf("🔄 Shutting down orchestrator...")

	// Cancel context to stop all collectors
	app.cancel()

	// Stop metrics orchestrator
	if app.metricsOrchestrator != nil {
		app.metricsOrchestrator.Stop()
	}

	// Stop logging service
	if app.loggingService != nil {
		if err := app.loggingService.Stop(); err != nil {
			log.Printf("⚠️ Error stopping logging service: %v", err)
		}
	}

	// Close discovery service
	if app.discoveryService != nil {
		if err := app.discoveryService.Close(); err != nil {
			log.Printf("⚠️ Error closing discovery service: %v", err)
		}
	}

	// Give collectors time to stop gracefully
	time.Sleep(2 * time.Second)

	log.Printf("✅ Real-time metrics orchestrator stopped cleanly")
	return nil
}

// UpdateTopology updates the topology and propagates changes
func (app *OrchestratorApp) UpdateTopology(newTopology *discovery.NetworkTopology) error {
	app.topology = newTopology

	// Update logging service with new topology
	if app.loggingService != nil {
		if err := app.loggingService.UpdateTopology(newTopology); err != nil {
			return fmt.Errorf("failed to update logging service topology: %w", err)
		}
	}

	// Update HTTP server topology
	if app.httpServer != nil {
		app.httpServer.SetTopology(newTopology)
	}

	return nil
}
