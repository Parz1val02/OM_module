package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parz1val02/OM_module/internal/capture"
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
	snap       *collector.Snapshot
	project    string
	reg        *prometheus.Registry
	recCfg     reconstructor.Config
	capManager *capture.Manager
	lokiURL    string
}

// New creates a Handlers instance.
func New(
	snap *collector.Snapshot,
	project string,
	reg *prometheus.Registry,
	recCfg reconstructor.Config,
	capManager *capture.Manager,
	_ interface{}, // correlator removed — kept for call-site compatibility
	lokiURL string,
) *Handlers {
	return &Handlers{
		snap:       snap,
		project:    project,
		reg:        reg,
		recCfg:     recCfg,
		capManager: capManager,
		lokiURL:    lokiURL,
	}
}

// Register wires all routes onto mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.Handle("/metrics", promhttp.HandlerFor(h.reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/topology", h.handleTopology)
	mux.HandleFunc("/ping", h.handlePing)
	mux.HandleFunc("/traces/reconstruct", h.handleReconstruct)
	mux.HandleFunc("/traces/search", h.handleTraceSearch)
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

// --- /traces/reconstruct -------------------------------------------------

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

// --- /traces/search ------------------------------------------------------
//
// Queries Tempo for recent captured packets matching optional filters.
//
// Query parameters:
//   generation  (optional) "4g" | "5g"
//   imsi        (optional) subscriber identity
//   limit       (optional) default 20
//   since       (optional) duration string, default "10m"

type traceSearchResult struct {
	TraceID    string            `json:"trace_id"`
	RootName   string            `json:"root_name"`
	StartTime  string            `json:"start_time"`
	DurationMs int64             `json:"duration_ms"`
	Tags       map[string]string `json:"tags"`
}

type traceSearchResponse struct {
	Traces []traceSearchResult `json:"traces"`
	Total  int                 `json:"total"`
}

func (h *Handlers) handleTraceSearch(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.Tracer().Start(r.Context(), "http.GET /traces/search")
	defer span.End()

	generation := strings.TrimSpace(r.URL.Query().Get("generation"))
	imsi := strings.TrimPrefix(strings.TrimSpace(r.URL.Query().Get("imsi")), "imsi-")
	limitStr := r.URL.Query().Get("limit")
	sinceStr := r.URL.Query().Get("since")

	limit := 20
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	since := 10 * time.Minute
	if sinceStr != "" {
		if d, err := time.ParseDuration(sinceStr); err == nil {
			since = d
		}
	}

	now := time.Now()
	start := now.Add(-since)

	var tags []string
	if generation != "" {
		tags = append(tags, fmt.Sprintf("generation=%s", generation))
	}
	if imsi != "" {
		tags = append(tags, fmt.Sprintf("imsi=%s", imsi))
	}
	tags = append(tags, "source=capture")

	tempoURL := fmt.Sprintf("http://tempo:3200/api/search?start=%d&end=%d&limit=%d&tags=%s",
		start.Unix(), now.Unix(), limit, strings.Join(tags, "%20"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tempoURL, nil)
	if err != nil {
		http.Error(w, `{"error":"failed to build tempo request"}`, http.StatusInternalServerError)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"tempo unreachable: %v"}`, err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read tempo response"}`, http.StatusInternalServerError)
		return
	}

	var tempoResp struct {
		Traces []struct {
			TraceID           string `json:"traceID"`
			RootTraceName     string `json:"rootTraceName"`
			StartTimeUnixNano string `json:"startTimeUnixNano"`
			DurationMs        int64  `json:"durationMs"`
			SpanSets          []struct {
				Spans []struct {
					Attributes []struct {
						Key   string `json:"key"`
						Value struct {
							StringValue string `json:"stringValue"`
						} `json:"value"`
					} `json:"attributes"`
				} `json:"spans"`
			} `json:"spanSets"`
		} `json:"traces"`
	}

	if err := json.Unmarshal(body, &tempoResp); err != nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}

	results := make([]traceSearchResult, 0, len(tempoResp.Traces))
	for _, t := range tempoResp.Traces {
		tags := make(map[string]string)
		if len(t.SpanSets) > 0 && len(t.SpanSets[0].Spans) > 0 {
			for _, attr := range t.SpanSets[0].Spans[0].Attributes {
				tags[attr.Key] = attr.Value.StringValue
			}
		}
		startNs, _ := strconv.ParseInt(t.StartTimeUnixNano, 10, 64)
		results = append(results, traceSearchResult{
			TraceID:    t.TraceID,
			RootName:   t.RootTraceName,
			StartTime:  time.Unix(0, startNs).UTC().Format(time.RFC3339),
			DurationMs: t.DurationMs,
			Tags:       tags,
		})
	}

	span.SetAttributes(attribute.Int("traces.found", len(results)))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(traceSearchResponse{
		Traces: results,
		Total:  len(results),
	})
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
