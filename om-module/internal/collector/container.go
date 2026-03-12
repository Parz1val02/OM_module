package collector

import (
	"context"
	"log"
	"sync"
	"time"

	dockerclient "github.com/Parz1val02/OM_module/internal/docker"
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
// on a fixed interval.  It only considers containers that carry om.* labels.
type Collector struct {
	docker   *dockerclient.Client
	project  string
	interval time.Duration
	snap     *Snapshot
}

// New creates a Collector.  project is the Docker Compose project name used
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
// Callers should call Snapshot().All() to get a consistent copy.
func (c *Collector) Snapshot() *Snapshot { return c.snap }

// Run starts the collection loop.  It blocks until ctx is cancelled.
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

// collect does one full discovery + stats pass.
func (c *Collector) collect(ctx context.Context) {
	containers, err := c.docker.ListContainers(ctx, c.project)
	if err != nil {
		log.Printf("⚠️  Collector: ListContainers error: %v", err)
		return
	}

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

		// Skip containers that have no om.* labels at all — they don't
		// belong to the testbed taxonomy (e.g., unrelated system containers).
		if cd.Domain == "" && cd.NF == "" {
			continue
		}

		// Only collect resource stats for running containers.
		if ct.State == "running" {
			if stats, err := c.docker.GetStats(ctx, ct.ID); err == nil {
				cd.CPUPercent = calcCPUPercent(stats)
				cd.MemoryUsageB = memUsage(stats)
				cd.NetworkRxBytes, cd.NetworkTxBytes = sumNetwork(stats)
				cd.PIDs = stats.PidsStats.Current
			} else if ctx.Err() == nil {
				log.Printf("⚠️  Collector: GetStats(%s) error: %v", ct.Name, err)
			}
		}

		newData[ct.Name] = cd
	}

	c.snap.set(newData)
}

// --- helper calculations -------------------------------------------------

// calcCPUPercent computes the CPU usage percentage using the standard
// Docker delta formula: ΔcpuDelta / ΔsystemDelta × numCPUs × 100.
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
