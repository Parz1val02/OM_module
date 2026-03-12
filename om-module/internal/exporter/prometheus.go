package exporter

import (
	"github.com/Parz1val02/OM_module/internal/collector"
	"github.com/prometheus/client_golang/prometheus"
)

// omExporter implements prometheus.Collector and emits per-container metrics
// labelled with the full om.* taxonomy:
//
//	container  — container name
//	project    — om.project  (open5gs | srsran | srslte | ueransim | grafana …)
//	domain     — om.domain   (core | ran | infra | observability)
//	nf         — om.nf       (amf | smf | upf | mme | gnb | enb | ue …)
//	generation — om.generation (4g | 5g | none)
//	image      — Docker image name
//	state      — Docker container state (running | exited | …)
type omExporter struct {
	snap    *collector.Snapshot
	project string

	// Descriptors
	cpuPercent   *prometheus.Desc
	memUsage     *prometheus.Desc
	netRx        *prometheus.Desc
	netTx        *prometheus.Desc
	pids         *prometheus.Desc
	healthStatus *prometheus.Desc
}

// labelNames is the fixed ordered set of labels attached to every metric.
var labelNames = []string{
	"container",
	"project",
	"domain",
	"nf",
	"generation",
	"image",
	"state",
}

// New registers a new omExporter in the given registry and returns it.
func New(snap *collector.Snapshot, composeProject string, reg prometheus.Registerer) {
	e := &omExporter{
		snap:    snap,
		project: composeProject,

		cpuPercent: prometheus.NewDesc(
			"container_cpu_usage_percent",
			"CPU usage percentage of the container (0–100 × numCPUs).",
			labelNames, nil,
		),
		memUsage: prometheus.NewDesc(
			"container_memory_usage_bytes",
			"Working-set memory usage of the container in bytes (usage − cache).",
			labelNames, nil,
		),
		netRx: prometheus.NewDesc(
			"container_network_rx_bytes_total",
			"Total bytes received across all container network interfaces.",
			labelNames, nil,
		),
		netTx: prometheus.NewDesc(
			"container_network_tx_bytes_total",
			"Total bytes transmitted across all container network interfaces.",
			labelNames, nil,
		),
		pids: prometheus.NewDesc(
			"container_pids",
			"Number of processes currently running inside the container.",
			labelNames, nil,
		),
		healthStatus: prometheus.NewDesc(
			"container_health_status",
			"Container health: 1 = running, 0 = degraded/unknown, -1 = stopped.",
			labelNames, nil,
		),
	}
	reg.MustRegister(e)
}

// Describe sends all metric descriptors to the channel.
func (e *omExporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.cpuPercent
	ch <- e.memUsage
	ch <- e.netRx
	ch <- e.netTx
	ch <- e.pids
	ch <- e.healthStatus
}

// Collect is called by Prometheus on every scrape.
func (e *omExporter) Collect(ch chan<- prometheus.Metric) {
	for _, cd := range e.snap.All() {
		lv := labelValues(cd)

		ch <- gauge(e.healthStatus, cd.HealthValue(), lv)

		// Resource metrics are only meaningful for running containers.
		if cd.State != "running" {
			continue
		}

		ch <- gauge(e.cpuPercent, cd.CPUPercent, lv)
		ch <- gauge(e.memUsage, float64(cd.MemoryUsageB), lv)
		ch <- counter(e.netRx, float64(cd.NetworkRxBytes), lv)
		ch <- counter(e.netTx, float64(cd.NetworkTxBytes), lv)
		ch <- gauge(e.pids, float64(cd.PIDs), lv)
	}
}

// labelValues builds the label value slice in the same order as labelNames.
func labelValues(cd *collector.ContainerData) []string {
	return []string{
		cd.Name,
		cd.Project,
		cd.Domain,
		cd.NF,
		cd.Generation,
		cd.Image,
		cd.State,
	}
}

func gauge(desc *prometheus.Desc, val float64, lv []string) prometheus.Metric {
	return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, val, lv...)
}

func counter(desc *prometheus.Desc, val float64, lv []string) prometheus.Metric {
	return prometheus.MustNewConstMetric(desc, prometheus.CounterValue, val, lv...)
}
