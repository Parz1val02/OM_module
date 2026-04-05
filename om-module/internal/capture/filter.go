package capture

// Generation constants match the om.generation Docker label values.
const (
	Generation4G = "4g"
	Generation5G = "5g"
)

// filters holds the tshark capture and display filter pair for a generation.
type filters struct {
	// BPF is a kernel-level capture filter passed via -f flag.
	// Only packets matching this filter are copied from the kernel to userspace.
	BPF string

	// Display is a tshark dissector-level filter passed via -Y flag.
	// Only packets successfully dissected as this protocol emit JSON output.
	Display string
}

// filtersFor returns the appropriate tshark filter pair for the given generation.
// Only Phase 1 protocols (NGAP for 5G, S1AP for 4G) are included.
// Phase 2 (GTPv2-C, PFCP) will be added here when the correlator supports them.
func filtersFor(generation string) filters {
	switch generation {
	case Generation5G:
		return filters{
			// NGAP runs over SCTP port 38412 between gNB and AMF.
			BPF:     "sctp port 38412",
			Display: "ngap",
		}
	case Generation4G:
		return filters{
			// S1AP runs over SCTP port 36412 between eNB and MME.
			BPF:     "sctp port 36412",
			Display: "s1ap",
		}
	default:
		// Fallback: capture both so the manager can log what it sees
		// while waiting for generation detection to settle.
		return filters{
			BPF:     "sctp port 38412 or sctp port 36412",
			Display: "ngap or s1ap",
		}
	}
}
