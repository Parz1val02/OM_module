package capture

// Generation constants match the om.generation Docker label values.
const (
	Generation4G = "4g"
	Generation5G = "5g"
)

// filters holds a pair of tshark filter sets.
// Two separate tshark processes run in parallel because combining SCTP and
// UDP in a single BPF filter is unreliable across kernel/container versions.
type filters struct {
	// SCTP process: captures S1AP (4G) or NGAP (5G)
	SCTPBPF     string
	SCTPDisplay string

	// UDP process: captures GTPv2-C and/or PFCP
	UDPBPF     string
	UDPDisplay string
}

// filtersFor returns the filter pair for the given generation.
func filtersFor(generation string) filters {
	switch generation {
	case Generation5G:
		return filters{
			SCTPBPF:     "sctp port 38412",
			SCTPDisplay: "ngap",
			UDPBPF:      "udp port 8805",
			UDPDisplay:  "pfcp",
		}
	case Generation4G:
		return filters{
			SCTPBPF:     "sctp port 36412",
			SCTPDisplay: "s1ap",
			UDPBPF:      "udp port 2123 or udp port 8805",
			UDPDisplay:  "gtpv2 or pfcp",
		}
	default:
		return filters{
			SCTPBPF:     "sctp port 38412 or sctp port 36412",
			SCTPDisplay: "ngap or s1ap",
			UDPBPF:      "udp port 2123 or udp port 8805",
			UDPDisplay:  "gtpv2 or pfcp",
		}
	}
}
