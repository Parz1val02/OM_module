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
			// NGAP (N2): SCTP 38412 — gNB ↔ AMF
			SCTPBPF:     "sctp port 38412",
			SCTPDisplay: "ngap",
			// PFCP (N4): UDP 8805 — SMF ↔ UPF
			// SBI HTTP/2 (all N-interfaces): TCP 7777 — NF ↔ NF via SCP
			UDPBPF:     "udp port 8805 or tcp port 7777",
			UDPDisplay: "pfcp or http2",
		}
	case Generation4G:
		return filters{
			// S1AP (S1-MME): SCTP 36412 — eNB ↔ MME
			// Diameter S6a/Gx: SCTP 3868/3873/5868 — MME↔HSS / PGW↔PCRF
			SCTPBPF:     "sctp port 36412 or sctp port 3868 or sctp port 3873 or sctp port 5868",
			SCTPDisplay: "s1ap or diameter",
			// GTPv2-C (S11/S5): UDP 2123
			// PFCP (Sxa/Sxb): UDP 8805
			UDPBPF:     "udp port 2123 or udp port 8805",
			UDPDisplay: "gtpv2 or pfcp",
		}
	default:
		return filters{
			SCTPBPF:     "sctp port 38412 or sctp port 36412 or sctp port 3868 or sctp port 3873 or sctp port 5868",
			SCTPDisplay: "ngap or s1ap or diameter",
			UDPBPF:      "udp port 2123 or udp port 8805 or tcp port 7777",
			UDPDisplay:  "gtpv2 or pfcp or http2",
		}
	}
}
