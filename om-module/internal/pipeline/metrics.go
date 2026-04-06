package pipeline

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds all Prometheus metrics emitted by the capture pipeline.
// One counter per captured packet, labelled by protocol, generation, src_nf, dst_nf.
type Metrics struct {
	// PacketsTotal counts every captured packet by protocol, generation,
	// source NF and destination NF.
	PacketsTotal *prometheus.CounterVec

	// PacketsByService counts SBI packets additionally labelled by service name
	// (e.g. nausf-auth, nudm-sdm) so students can see which N-interfaces are
	// most active over time.
	PacketsByService *prometheus.CounterVec

	// ErrorsTotal counts packets with error cause codes (GTPv2 cause!=16,
	// PFCP cause!=1, Diameter result!=2001, SBI status>=400).
	ErrorsTotal *prometheus.CounterVec
}

// NewMetrics registers and returns all pipeline metrics on the given registry.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		PacketsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "om",
			Subsystem: "capture",
			Name:      "packets_total",
			Help:      "Total number of captured packets by protocol, generation, source NF and destination NF.",
		}, []string{"protocol", "generation", "src_nf", "dst_nf"}),

		PacketsByService: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "om",
			Subsystem: "capture",
			Name:      "sbi_requests_total",
			Help:      "Total number of captured SBI HTTP/2 requests by service name and method.",
		}, []string{"service", "method", "src_nf", "dst_nf"}),

		ErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "om",
			Subsystem: "capture",
			Name:      "errors_total",
			Help:      "Total number of captured packets with error cause codes.",
		}, []string{"protocol", "generation", "src_nf", "dst_nf"}),
	}

	reg.MustRegister(m.PacketsTotal, m.PacketsByService, m.ErrorsTotal)
	return m
}
