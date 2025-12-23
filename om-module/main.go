package main

import (
	"log"
	"os"

	"github.com/Parz1val02/OM_module/app"
	"github.com/Parz1val02/OM_module/ui"
)

func main() {
	log.Printf("🚀 Starting O&M Module for 4G/5G Educational Network Testbed")

	// Parse configuration from command line arguments
	cfg := app.NewConfig(os.Args)

	if cfg.InDocker {
		log.Printf("🐳 Running in Docker environment")
	}
	if cfg.DebugMode {
		log.Printf("🐞 Debug mode enabled")
	}

	// Run the appropriate mode
	var err error
	switch cfg.Mode {
	case "orchestrator":
		err = runOrchestrator(cfg)
	case "discovery":
		err = runDiscovery(cfg)
	default:
		ui.PrintBanner()
		ui.PrintUsage()
		os.Exit(1)
	}

	// Handle errors
	if err != nil {
		log.Fatalf("❌ Application error: %v", err)
	}
}

// runOrchestrator starts the orchestrator mode
func runOrchestrator(cfg *app.Config) error {
	// Create orchestrator application
	orchestrator, err := app.NewOrchestratorApp(cfg)
	if err != nil {
		return err
	}

	// Start the application
	return orchestrator.Start()
}

// runDiscovery starts the discovery mode
func runDiscovery(cfg *app.Config) error {
	// Create discovery application
	discoveryApp, err := app.NewDiscoveryApp(cfg)
	if err != nil {
		return err
	}

	// Run the discovery process
	return discoveryApp.Run()
}
