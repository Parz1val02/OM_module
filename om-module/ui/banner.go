package ui

import (
	"fmt"
	"os"
)

// PrintBanner displays the application banner
func PrintBanner() {
	fmt.Printf("\n")
	fmt.Printf("╔═══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                    📡 4G/5G Network O&M Module v2.0                           ║\n")
	fmt.Printf("║                   Real-Time Monitoring & Educational Platform                 ║\n")
	fmt.Printf("║                    🎓 Industry-Grade Observability for Labs                   ║\n")
	if IsRunningInDocker() {
		fmt.Printf("║                          🐳 Docker Integration Mode                           ║\n")
	}
	fmt.Printf("╚═══════════════════════════════════════════════════════════════════════════════╝\n")
	fmt.Printf("\n")
}

// PrintUsage displays usage information
func PrintUsage() {
	fmt.Printf("\n📖 USAGE INFORMATION\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("Usage: %s [mode] [options]\n\n", os.Args[0])

	fmt.Printf("🔍 DISCOVERY MODE (default):\n")
	fmt.Printf("   %s discovery [env_file]\n", os.Args[0])
	fmt.Printf("   • Analyzes your network topology without starting collectors\n")
	fmt.Printf("   • Generates Prometheus configurations and documentation\n")
	fmt.Printf("   • Creates dynamic Promtail configurations for logging\n")
	fmt.Printf("   • Perfect for understanding your setup before monitoring\n")
	fmt.Printf("   • Outputs: config files, topology analysis, setup guides\n\n")

	fmt.Printf("🎬 ORCHESTRATOR MODE:\n")
	fmt.Printf("   %s orchestrator [env_file]\n", os.Args[0])
	fmt.Printf("   • Starts live real-time metrics collection from all components\n")
	fmt.Printf("   • Provides HTTP endpoints for Prometheus scraping\n")
	fmt.Printf("   • Starts log parser for real-time log processing\n")
	fmt.Printf("   • Continuously monitors and adapts to topology changes\n")
	fmt.Printf("   • Use this when you want active monitoring and data collection\n\n")

	fmt.Printf("🐞 DEBUG OPTIONS:\n")
	fmt.Printf("   %s [mode] --debug    # Enable detailed debugging output\n", os.Args[0])
	fmt.Printf("   Creates additional debug files for troubleshooting\n\n")

	fmt.Printf("📁 ENV FILE:\n")
	if IsRunningInDocker() {
		fmt.Printf("   Docker mode: .env (container environment)\n")
	} else {
		fmt.Printf("   Default: ../env (Docker Compose environment)\n")
		fmt.Printf("   Custom: Specify path to your .env file\n")
	}
	fmt.Printf("\n")

	fmt.Printf("💡 EXAMPLES:\n")
	fmt.Printf("   %s discovery                    # Analyze topology\n", os.Args[0])
	fmt.Printf("   %s orchestrator                 # Start live monitoring\n", os.Args[0])
	if !IsRunningInDocker() {
		fmt.Printf("   %s discovery /path/to/.env      # Custom env file\n", os.Args[0])
	}
	fmt.Printf("   %s orchestrator --debug         # Debug mode\n", os.Args[0])
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
}

// IsRunningInDocker detects if running in Docker environment
func IsRunningInDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return false
}
