package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Parz1val02/OM_module/internal/capture"
	"github.com/Parz1val02/OM_module/internal/collector"
	"github.com/Parz1val02/OM_module/internal/tracing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Handlers bundles the HTTP handler dependencies.
type Handlers struct {
	snap       *collector.Snapshot
	project    string
	reg        *prometheus.Registry
	capManager *capture.Manager
}

// New creates a Handlers instance.
func New(
	snap *collector.Snapshot,
	project string,
	reg *prometheus.Registry,
	capManager *capture.Manager,
) *Handlers {
	return &Handlers{
		snap:       snap,
		project:    project,
		reg:        reg,
		capManager: capManager,
	}
}

// Register wires all routes onto mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.Handle("/metrics", promhttp.HandlerFor(h.reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/topology", h.handleTopology)
	mux.HandleFunc("/ping", h.handlePing)
	mux.HandleFunc("/capture/status", h.handleCaptureStatus)
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

// --- /capture/status -----------------------------------------------------

type captureStatusResponse struct {
	Running       bool    `json:"running"`
	Interface     string  `json:"interface"`
	Generation    string  `json:"generation"`
	PacketsTotal  uint64  `json:"packets_total"`
	Packets4G     uint64  `json:"packets_4g"`
	Packets5G     uint64  `json:"packets_5g"`
	RestartCount  uint64  `json:"restart_count"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	ActiveProcs   int     `json:"active_procedures"`
}

func (h *Handlers) handleCaptureStatus(w http.ResponseWriter, r *http.Request) {
	_, span := tracing.Tracer().Start(r.Context(), "http.GET /capture/status")
	defer span.End()

	var resp captureStatusResponse

	if h.capManager != nil {
		s := h.capManager.Status()
		resp = captureStatusResponse{
			Running:       s.Running,
			Interface:     s.Interface,
			Generation:    s.Generation,
			PacketsTotal:  s.PacketsTotal,
			Packets4G:     s.Packets4G,
			Packets5G:     s.Packets5G,
			RestartCount:  s.RestartCount,
			UptimeSeconds: s.UptimeSeconds,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
