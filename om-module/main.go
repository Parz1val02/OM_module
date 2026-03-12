package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Parz1val02/OM_module/api"
	"github.com/Parz1val02/OM_module/config"
	"github.com/Parz1val02/OM_module/internal/collector"
	dockerclient "github.com/Parz1val02/OM_module/internal/docker"
	"github.com/Parz1val02/OM_module/internal/exporter"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	cfg := config.Load()

	log.Printf("╔══════════════════════════════════════════╗")
	log.Printf("║   O&M Module — 4G/5G Educational Testbed ║")
	log.Printf("╚══════════════════════════════════════════╝")
	log.Printf("Port            : %s", cfg.Port)
	log.Printf("Docker socket   : %s", cfg.DockerSocket)
	log.Printf("Compose project : %s", cfg.ComposeProject)

	// --- Docker client ---
	dockerClient, err := dockerclient.New(cfg.DockerSocket)
	if err != nil {
		log.Fatalf("Cannot connect to Docker: %v", err)
	}
	defer func() {
		if err := dockerClient.Close(); err != nil {
			log.Printf("⚠️  Docker client close error: %v", err)
		}
	}()
	log.Printf("✅ Connected to Docker daemon")

	// --- Context with graceful shutdown ---
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Container collector ---
	// Discovery is driven entirely by om.* Docker labels:
	//   om.domain     → "core" | "ran" | "infra" | "observability"
	//   om.nf         → "amf" | "smf" | "upf" | "mme" | "gnb" | "enb" | "ue" | …
	//   om.generation → "4g" | "5g" | "none"
	//   om.project    → "open5gs" | "srsran" | "srslte" | "ueransim" | …
	//
	// Containers that carry none of these labels are silently ignored,
	// so unrelated system containers never pollute the topology view.
	coll := collector.New(dockerClient, cfg.ComposeProject, 15*time.Second)

	// Start collector in background
	go coll.Run(ctx)

	// --- Prometheus registry ---
	// Use a custom registry so we control exactly what is exposed.
	reg := prometheus.NewRegistry()

	// Testbed container metrics — labelled with the full om.* taxonomy:
	//   container_cpu_usage_percent{container, project, domain, nf, generation, image, state}
	//   container_memory_usage_bytes{…}
	//   container_network_rx_bytes_total{…}
	//   container_network_tx_bytes_total{…}
	//   container_pids{…}
	//   container_health_status{…}   → 1=running, 0=degraded, -1=stopped
	exporter.New(coll.Snapshot(), cfg.ComposeProject, reg)
	log.Printf("✅ Prometheus exporter registered")

	// --- HTTP server ---
	mux := http.NewServeMux()

	// Pass the same registry to the API layer so /metrics uses it
	handlers := api.New(coll.Snapshot(), cfg.ComposeProject, reg)
	handlers.Register(mux)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Run HTTP server in background
	go func() {
		log.Printf("🚀 HTTP server listening on :%s", cfg.Port)
		log.Printf("   GET /metrics   → Prometheus scrape endpoint (infra + health metrics)")
		log.Printf("   GET /topology  → Full testbed topology + health status (JSON)")
		log.Printf("   GET /ping      → Liveness probe")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	log.Printf("🛑 Shutdown signal received — stopping gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Printf("✅ O&M Module stopped cleanly")
}
