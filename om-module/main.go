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
	"github.com/Parz1val02/OM_module/internal/capture"
	"github.com/Parz1val02/OM_module/internal/collector"
	dockerclient "github.com/Parz1val02/OM_module/internal/docker"
	"github.com/Parz1val02/OM_module/internal/exporter"
	"github.com/Parz1val02/OM_module/internal/pipeline"
	"github.com/Parz1val02/OM_module/internal/tracing"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	cfg := config.Load()

	log.Printf("╔══════════════════════════════════════════╗")
	log.Printf("║   O&M Module — 4G/5G Educational Testbed ║")
	log.Printf("╚══════════════════════════════════════════╝")
	log.Printf("Port              : %s", cfg.Port)
	log.Printf("Docker socket     : %s", cfg.DockerSocket)
	log.Printf("Compose project   : %s", cfg.ComposeProject)
	log.Printf("Tempo endpoint    : %s", cfg.TempoEndpoint)
	log.Printf("Capture enabled   : %v", cfg.CaptureEnabled)
	log.Printf("Capture interface : %s", cfg.CaptureInterface)
	log.Printf("MCC/MNC           : %s/%s", cfg.MCC, cfg.MNC)

	// --- Context with graceful shutdown ---
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Distributed tracing → Grafana Tempo ---
	shutdownTracing, err := tracing.Init(ctx, cfg.TempoEndpoint)
	if err != nil {
		log.Printf("⚠️  Tracing init failed (continuing without traces): %v", err)
	} else {
		defer func() {
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdownTracing(flushCtx); err != nil {
				log.Printf("⚠️  Tracing shutdown error: %v", err)
			}
			log.Printf("✅ Tracing shut down cleanly")
		}()
	}

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

	// --- Container collector ---
	coll := collector.New(dockerClient, cfg.ComposeProject, 15*time.Second)
	go coll.Run(ctx)

	// --- Prometheus registry ---
	reg := prometheus.NewRegistry()
	exporter.New(coll.Snapshot(), cfg.ComposeProject, reg)
	log.Printf("✅ Prometheus exporter registered")

	// --- Capture manager and pipeline (optional) ---
	var capManager *capture.Manager

	if cfg.CaptureEnabled {
		capManager = capture.NewManager(
			dockerClient,
			coll.Snapshot(),
			cfg.MCC,
			cfg.MNC,
			cfg.CaptureInterface,
		)

		pipeMetrics := pipeline.NewMetrics(reg)
		pipe := pipeline.New(cfg.MCC, cfg.MNC, dockerClient, coll.Snapshot(), pipeMetrics)

		// Start capture manager — self-retries until generation detected.
		go capManager.Run(ctx)

		// Start pipeline — reads from capture manager and emits one span per packet.
		go func() {
			for {
				pipe.Run(ctx, capManager.Packets())
				if ctx.Err() != nil {
					return
				}
				time.Sleep(time.Second)
			}
		}()

		log.Printf("✅ Capture pipeline started (interface=%s)", cfg.CaptureInterface)
	} else {
		log.Printf("⚠️  Capture pipeline disabled (CAPTURE_ENABLED=false)")
	}

	// --- HTTP server ---
	mux := http.NewServeMux()
	handlers := api.New(
		coll.Snapshot(),
		cfg.ComposeProject,
		reg,
		capManager,
	)
	handlers.Register(mux)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("🚀 HTTP server listening on :%s", cfg.Port)
		log.Printf("   GET /metrics                           → Prometheus scrape endpoint")
		log.Printf("   GET /topology                          → Testbed topology + health (JSON)")
		log.Printf("   GET /ping                              → Liveness probe")
		log.Printf("   GET /capture/status                    → Capture pipeline health")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("🛑 Shutdown signal received — stopping gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	log.Printf("✅ O&M Module stopped cleanly")
}
