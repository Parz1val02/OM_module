package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Parz1val02/OM_module/internal/collector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handlers bundles the HTTP handler dependencies.
type Handlers struct {
	snap    *collector.Snapshot
	project string
	reg     *prometheus.Registry
}

// New creates a Handlers instance.
func New(snap *collector.Snapshot, project string, reg *prometheus.Registry) *Handlers {
	return &Handlers{snap: snap, project: project, reg: reg}
}

// Register wires all routes onto mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.Handle("/metrics", promhttp.HandlerFor(h.reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/topology", h.handleTopology)
	mux.HandleFunc("/ping", h.handlePing)
}

// --- /ping ---------------------------------------------------------------

func (h *Handlers) handlePing(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("pong"))
}

// --- /topology -----------------------------------------------------------

// topologyContainer is the per-container entry in the /topology response.
type topologyContainer struct {
	Name       string  `json:"name"`
	State      string  `json:"state"`
	Image      string  `json:"image"`
	Domain     string  `json:"domain"`
	NF         string  `json:"nf"`
	Generation string  `json:"generation"`
	Project    string  `json:"project"`
	Health     float64 `json:"health_status"` // 1 | 0 | -1
}

// topologyResponse is the full /topology JSON payload.
type topologyResponse struct {
	Timestamp  string              `json:"timestamp"`
	Project    string              `json:"project"`
	Status     string              `json:"status"` // "ok" | "degraded"
	Total      int                 `json:"total"`
	Running    int                 `json:"running"`
	Stopped    int                 `json:"stopped"`
	Containers []topologyContainer `json:"containers"`
}

func (h *Handlers) handleTopology(w http.ResponseWriter, _ *http.Request) {
	all := h.snap.All()

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
			Name:       cd.Name,
			State:      cd.State,
			Image:      cd.Image,
			Domain:     cd.Domain,
			NF:         cd.NF,
			Generation: cd.Generation,
			Project:    cd.Project,
			Health:     cd.HealthValue(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
