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
	"github.com/Parz1val02/OM_module/internal/correlator"
	dockerclient "github.com/Parz1val02/OM_module/internal/docker"
	"github.com/Parz1val02/OM_module/internal/exporter"
	"github.com/Parz1val02/OM_module/internal/reconstructor"
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
	log.Printf("Loki URL          : %s", cfg.LokiURL)
	log.Printf("Trace window      : %s", cfg.TraceQueryWindow)
	log.Printf("Capture enabled   : %v", cfg.CaptureEnabled)
	log.Printf("Capture interface : %s", cfg.CaptureInterface)
	log.Printf("Procedure timeout : %s", cfg.ProcedureTimeout)
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

	// --- Trace reconstructor config ---
	queryWindow, err := time.ParseDuration(cfg.TraceQueryWindow)
	if err != nil {
		queryWindow = 10 * time.Minute
	}
	recCfg := reconstructor.Config{
		LokiURL:     cfg.LokiURL,
		QueryWindow: queryWindow,
	}

	// --- Procedure timeout ---
	procTimeout, err := time.ParseDuration(cfg.ProcedureTimeout)
	if err != nil {
		procTimeout = 30 * time.Second
	}

	// --- Capture manager and correlator (optional) ---
	var capManager *capture.Manager
	var corr *correlator.Correlator

	if cfg.CaptureEnabled {
		capManager = capture.NewManager(
			dockerClient,
			coll.Snapshot(),
			cfg.MCC,
			cfg.MNC,
			cfg.CaptureInterface,
		)

		corr = correlator.New(cfg.MCC, cfg.MNC, procTimeout, recCfg)

		// Start capture manager — self-retries until generation detected.
		go capManager.Run(ctx)

		// Start correlator — reads from capture manager's packet channel.
		// We run this in a loop so if the capture manager restarts and closes
		// its channel, the correlator loop restarts too.
		go func() {
			for {
				corr.Run(ctx, capManager.Packets())
				if ctx.Err() != nil {
					return
				}
				// Brief pause before re-attaching to allow manager restart.
				time.Sleep(time.Second)
			}
		}()

		log.Printf("✅ Capture pipeline started (interface=%s timeout=%s)",
			cfg.CaptureInterface, cfg.ProcedureTimeout)
	} else {
		log.Printf("⚠️  Capture pipeline disabled (CAPTURE_ENABLED=false)")
	}

	// --- HTTP server ---
	mux := http.NewServeMux()
	handlers := api.New(
		coll.Snapshot(),
		cfg.ComposeProject,
		reg,
		recCfg,
		capManager,
		corr,
		cfg.LokiURL,
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
		log.Printf("   GET /traces/reconstruct?imsi=<IMSI>    → Log-based trace reconstruction → Tempo")
		log.Printf("   GET /traces/search                     → Search recent live traces in Tempo")
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
