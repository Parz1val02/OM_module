package collector

import (
	"context"
	"log"
	"sync"
	"time"

	dockerclient "github.com/Parz1val02/OM_module/internal/docker"
	"github.com/Parz1val02/OM_module/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// OMLabelDomain values for om.domain
const (
	DomainCore          = "core"
	DomainRAN           = "ran"
	DomainInfra         = "infra"
	DomainObservability = "observability"
)

// ContainerData holds the latest collected data for a single container.
// Every field that comes from Docker labels is populated from om.* labels
// exclusively — no name-based heuristics.
type ContainerData struct {
	// Identity from Docker
	ID    string
	Name  string
	State string // "running" | "exited" | …
	Image string

	// om.* taxonomy labels (sourced directly from container labels)
	Domain     string // om.domain  → core | ran | infra | observability
	NF         string // om.nf      → amf | smf | upf | mme | gnb | enb | ue | …
	Generation string // om.generation → 4g | 5g | none
	Project    string // om.project → open5gs | srsran | srslte | ueransim | grafana | …

	// Resource metrics (zero if container is not running)
	CPUPercent     float64
	MemoryUsageB   uint64
	NetworkRxBytes uint64
	NetworkTxBytes uint64
	PIDs           uint64
}

// HealthValue maps Docker container state to a numeric health value.
//
//	 1 → running
//	 0 → running but unhealthy (Docker health check failed)
//	-1 → stopped / exited
func (cd *ContainerData) HealthValue() float64 {
	switch cd.State {
	case "running":
		return 1
	case "exited", "dead":
		return -1
	default:
		return 0
	}
}

// Snapshot is a thread-safe read-only view of the latest collected data.
type Snapshot struct {
	mu   sync.RWMutex
	data map[string]*ContainerData // keyed by container Name
}

func newSnapshot() *Snapshot { return &Snapshot{data: make(map[string]*ContainerData)} }

// All returns a copy of the current snapshot map.
func (s *Snapshot) All() map[string]*ContainerData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*ContainerData, len(s.data))
	for k, v := range s.data {
		cp := *v
		out[k] = &cp
	}
	return out
}

func (s *Snapshot) set(data map[string]*ContainerData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = data
}

// Collector discovers containers and collects their resource metrics
// on a fixed interval. It only considers containers that carry om.* labels.
type Collector struct {
	docker   *dockerclient.Client
	project  string
	interval time.Duration
	snap     *Snapshot
}

// New creates a Collector. project is the Docker Compose project name used
// to filter containers; interval controls how often stats are refreshed.
func New(docker *dockerclient.Client, project string, interval time.Duration) *Collector {
	return &Collector{
		docker:   docker,
		project:  project,
		interval: interval,
		snap:     newSnapshot(),
	}
}

// Snapshot returns the live, thread-safe snapshot reference.
func (c *Collector) Snapshot() *Snapshot { return c.snap }

// Run starts the collection loop. It blocks until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) {
	log.Printf("📦 Collector started (project=%q, interval=%s)", c.project, c.interval)
	c.collect(ctx) // run immediately on startup
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.collect(ctx)
		case <-ctx.Done():
			log.Printf("📦 Collector stopped")
			return
		}
	}
}

// collect performs one full discovery + stats pass.
//
// Tracing structure:
//
//	collector.collect_cycle          (root — one trace per 15s tick)
//	  ├── collector.list_containers  (single Docker API call)
//	  └── collector.get_stats        (one child span per running container)
func (c *Collector) collect(ctx context.Context) {
	// --- Root span: covers the entire collection cycle ---
	ctx, cycleSpan := tracing.Tracer().Start(ctx, "collector.collect_cycle")
	defer cycleSpan.End()

	// --- List containers ---
	ctx, listSpan := tracing.Tracer().Start(ctx, "collector.list_containers")
	containers, err := c.docker.ListContainers(ctx, c.project)
	if err != nil {
		listSpan.RecordError(err)
		listSpan.SetStatus(codes.Error, err.Error())
		listSpan.End()
		cycleSpan.RecordError(err)
		log.Printf("⚠️  Collector: ListContainers error: %v", err)
		return
	}
	listSpan.SetAttributes(attribute.Int("containers.discovered", len(containers)))
	listSpan.End()

	newData := make(map[string]*ContainerData, len(containers))

	for _, ct := range containers {
		cd := &ContainerData{
			ID:    ct.ID,
			Name:  ct.Name,
			State: ct.State,
			Image: ct.Image,

			// Read om.* labels — zero-value ("") if label absent
			Domain:     ct.Labels["om.domain"],
			NF:         ct.Labels["om.nf"],
			Generation: ct.Labels["om.generation"],
			Project:    ct.Labels["om.project"],
		}

		// Skip containers with no om.* labels — they don't belong to the
		// testbed taxonomy (e.g. unrelated system containers).
		if cd.Domain == "" && cd.NF == "" {
			continue
		}

		// Only collect resource stats for running containers.
		if ct.State == "running" {
			// One child span per container stats call so slow Docker API
			// calls are individually visible in the Tempo waterfall.
			_, statsSpan := tracing.Tracer().Start(ctx, "collector.get_stats")
			statsSpan.SetAttributes(
				attribute.String("container.name", ct.Name),
				attribute.String("container.nf", cd.NF),
				attribute.String("container.domain", cd.Domain),
				attribute.String("container.generation", cd.Generation),
			)

			if stats, err := c.docker.GetStats(ctx, ct.ID); err == nil {
				cd.CPUPercent = calcCPUPercent(stats)
				cd.MemoryUsageB = memUsage(stats)
				cd.NetworkRxBytes, cd.NetworkTxBytes = sumNetwork(stats)
				cd.PIDs = stats.PidsStats.Current

				statsSpan.SetAttributes(
					attribute.Float64("container.cpu_percent", cd.CPUPercent),
					attribute.Int("container.memory_bytes", int(cd.MemoryUsageB)),
					attribute.Int("container.pids", int(cd.PIDs)),
				)
			} else if ctx.Err() == nil {
				statsSpan.RecordError(err)
				statsSpan.SetStatus(codes.Error, err.Error())
				log.Printf("⚠️  Collector: GetStats(%s) error: %v", ct.Name, err)
			}

			statsSpan.End()
		}

		newData[ct.Name] = cd
	}

	// Summarise the cycle on the root span.
	running := 0
	for _, cd := range newData {
		if cd.State == "running" {
			running++
		}
	}
	cycleSpan.SetAttributes(
		attribute.Int("cycle.containers_total", len(newData)),
		attribute.Int("cycle.containers_running", running),
	)

	c.snap.set(newData)
}

// --- helper calculations -------------------------------------------------

// calcCPUPercent computes CPU usage % using the Docker delta formula:
// ΔcpuDelta / ΔsystemDelta × numCPUs × 100
func calcCPUPercent(s *dockerclient.RawStats) float64 {
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) -
		float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemCPUUsage) -
		float64(s.PreCPUStats.SystemCPUUsage)

	if sysDelta <= 0 || cpuDelta < 0 {
		return 0
	}

	numCPUs := float64(s.CPUStats.OnlineCPUs)
	if numCPUs == 0 {
		numCPUs = 1
	}
	return (cpuDelta / sysDelta) * numCPUs * 100.0
}

// memUsage returns working-set memory (usage − cache).
func memUsage(s *dockerclient.RawStats) uint64 {
	usage := s.MemoryStats.Usage
	cache := s.MemoryStats.Stats.Cache
	if cache > usage {
		return 0
	}
	return usage - cache
}

// sumNetwork aggregates RX/TX bytes across all network interfaces.
func sumNetwork(s *dockerclient.RawStats) (rx, tx uint64) {
	for _, iface := range s.Networks {
		rx += iface.RxBytes
		tx += iface.TxBytes
	}
	return
}
