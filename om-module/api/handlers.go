package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Parz1val02/OM_module/internal/collector"
	"github.com/Parz1val02/OM_module/internal/reconstructor"
	"github.com/Parz1val02/OM_module/internal/tracing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Handlers bundles the HTTP handler dependencies.
type Handlers struct {
	snap    *collector.Snapshot
	project string
	reg     *prometheus.Registry
	recCfg  reconstructor.Config
}

// New creates a Handlers instance.
func New(snap *collector.Snapshot, project string, reg *prometheus.Registry, recCfg reconstructor.Config) *Handlers {
	return &Handlers{snap: snap, project: project, reg: reg, recCfg: recCfg}
}

// Register wires all routes onto mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.Handle("/metrics", promhttp.HandlerFor(h.reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/topology", h.handleTopology)
	mux.HandleFunc("/ping", h.handlePing)
	mux.HandleFunc("/traces/reconstruct", h.handleReconstruct)
}

// --- /ping ---------------------------------------------------------------

func (h *Handlers) handlePing(w http.ResponseWriter, r *http.Request) {
	_, span := tracing.Tracer().Start(r.Context(), "http.GET /ping")
	defer span.End()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("pong"))
}

// --- /topology -----------------------------------------------------------

type topologyContainer struct {
	Name       string  `json:"name"`
	State      string  `json:"state"`
	Image      string  `json:"image"`
	Domain     string  `json:"domain"`
	NF         string  `json:"nf"`
	Generation string  `json:"generation"`
	Project    string  `json:"project"`
	Health     float64 `json:"health_status"`
}

type topologyResponse struct {
	Timestamp  string              `json:"timestamp"`
	Project    string              `json:"project"`
	Status     string              `json:"status"`
	Total      int                 `json:"total"`
	Running    int                 `json:"running"`
	Stopped    int                 `json:"stopped"`
	Containers []topologyContainer `json:"containers"`
}

func (h *Handlers) handleTopology(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.Tracer().Start(r.Context(), "http.GET /topology")
	defer span.End()

	_, snapSpan := tracing.Tracer().Start(ctx, "topology.read_snapshot")
	all := h.snap.All()
	snapSpan.SetAttributes(attribute.Int("snapshot.container_count", len(all)))
	snapSpan.End()

	_, buildSpan := tracing.Tracer().Start(ctx, "topology.build_response")
	resp := topologyResponse{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Project:    h.project,
		Status:     "ok",
		Containers: make([]topologyContainer, 0, len(all)),
	}

	for _, cd := range all {
		resp.Total++
		if cd.State == "running" {
			resp.Running++
		} else {
			resp.Stopped++
			resp.Status = "degraded"
		}
		resp.Containers = append(resp.Containers, topologyContainer{
			Name: cd.Name, State: cd.State, Image: cd.Image,
			Domain: cd.Domain, NF: cd.NF, Generation: cd.Generation,
			Project: cd.Project, Health: cd.HealthValue(),
		})
	}

	buildSpan.SetAttributes(
		attribute.Int("topology.total", resp.Total),
		attribute.Int("topology.running", resp.Running),
		attribute.Int("topology.stopped", resp.Stopped),
		attribute.String("topology.status", resp.Status),
	)
	if resp.Status == "degraded" {
		span.SetStatus(codes.Error, "one or more containers stopped")
	}
	buildSpan.End()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// --- /traces/reconstruct -------------------------------------------------
//
// Reconstructs a synthetic distributed trace from Loki logs for a given IMSI.
//
// Query parameters:
//   imsi       (required) - 15-digit IMSI, e.g. "001011234567895"
//   generation (optional) - "4g" or "5g" (default: auto-detect)
//   window     (optional) - how far back to look, e.g. "15m"
//
// Example:
//   GET /traces/reconstruct?imsi=001011234567895&generation=5g
//
// Returns JSON with trace_id, procedure name, span count and event list.
// The trace is simultaneously exported to Grafana Tempo.

func (h *Handlers) handleReconstruct(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.Tracer().Start(r.Context(), "http.GET /traces/reconstruct")
	defer span.End()

	imsi := strings.TrimSpace(r.URL.Query().Get("imsi"))
	if imsi == "" {
		http.Error(w, `{"error":"imsi query parameter is required"}`, http.StatusBadRequest)
		span.SetStatus(codes.Error, "missing imsi")
		return
	}
	imsi = strings.TrimPrefix(imsi, "imsi-")

	generation := strings.TrimSpace(r.URL.Query().Get("generation"))

	cfg := h.recCfg
	if wParam := r.URL.Query().Get("window"); wParam != "" {
		if d, err := time.ParseDuration(wParam); err == nil {
			cfg.QueryWindow = d
		}
	}

	span.SetAttributes(
		attribute.String("imsi", imsi),
		attribute.String("generation", generation),
		attribute.String("query_window", cfg.QueryWindow.String()),
	)

	result, err := reconstructor.Reconstruct(ctx, cfg, imsi, generation)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	span.SetAttributes(
		attribute.String("trace_id", result.TraceID),
		attribute.Int("span_count", result.SpanCount),
		attribute.String("procedure", result.Procedure),
	)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
