package capture

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// Packet is the parsed, normalised representation of a single tshark EK packet.
// Only the fields the correlator needs are extracted; everything else is dropped.
type Packet struct {
	// Timestamp is derived from the frame epoch time in the EK output.
	// It has nanosecond resolution and reflects the actual capture time.
	Timestamp time.Time

	// Generation is "4g" or "5g" based on which protocol layer is present.
	Generation string

	// SrcIP and DstIP from the IP layer.
	SrcIP string
	DstIP string

	// --- S1AP fields (4G) ---
	S1APProcedureCode int    // e.g. 12=InitialUEMessage, 11=DownlinkNAS, 13=UplinkNAS
	ENBUUES1APID      string // ENB-UE-S1AP-ID, present from InitialUEMessage onward
	MMEUUES1APID      string // MME-UE-S1AP-ID, present from DownlinkNASTransport onward
	NASEMMType        string // hex string e.g. "0x41" for Attach Request
	NASESMType        string // hex string for ESM messages
	IMSI              string // only present in Identity Response (NAS 0x56)

	// --- NGAP fields (5G) ---
	NGAPProcedureCode int    // e.g. 15=InitialUEMessage, 4=DownlinkNAS, 46=UplinkNAS
	RANUENGAPId       string // RAN-UE-NGAP-ID, present from InitialUEMessage onward
	AMFUENGAPId       string // AMF-UE-NGAP-ID, present from DownlinkNASTransport onward
	NASMMType         string // hex string e.g. "0x41" for Registration Request
	SUCIMsin          string // MSIN portion of SUCI, only in Registration Request
}

// ekPacket is the raw EK JSON structure emitted by tshark -T ek.
// The format alternates: an index line ({"index":{...}}) then a data line
// ({"timestamp":...,"layers":{...}}). We only care about data lines.
type ekPacket struct {
	Timestamp	json.RawMessage            `json:"timestamp"` // milliseconds since epoch
	Layers		map[string]json.RawMessage `json:"layers"`
}

// tsharkProcess wraps a running tshark subprocess and its stdout pipe.
type tsharkProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	out    chan Packet
	errc   chan error
}

// startTshark launches tshark on the given interface with the given filters
// and returns a channel of parsed Packet values. The subprocess runs until
// the provided context is cancelled or the process exits.
//
// The returned error channel receives at most one value — the exit error
// (nil if the process was killed cleanly by context cancellation).
func startTshark(ctx context.Context, iface string, f filters) (<-chan Packet, <-chan error) {
	out := make(chan Packet, 256)
	errc := make(chan error, 1)

	procCtx, cancel := context.WithCancel(ctx)

	args := []string{
		"-i", iface,
		"-f", f.BPF,
		"-Y", f.Display,
		"-T", "ek",
		"-l", // flush after each packet
		"-n", // disable name resolution
	}

	cmd := exec.CommandContext(procCtx, "tshark", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		errc <- fmt.Errorf("tshark: stdout pipe: %w", err)
		close(out)
		close(errc)
		return out, errc
	}

	if err := cmd.Start(); err != nil {
		cancel()
		errc <- fmt.Errorf("tshark: start: %w", err)
		close(out)
		close(errc)
		return out, errc
	}

	log.Printf("🦈 tshark started (pid=%d iface=%s bpf=%q display=%q)",
		cmd.Process.Pid, iface, f.BPF, f.Display)

	go func() {
		defer cancel()
		defer close(out)
		defer close(errc)

		scanner := bufio.NewScanner(stdout)
		// tshark EK lines can be large for some protocols; give 4MB buffer.
		scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			// EK format alternates index lines and data lines.
			// Index lines start with {"index": and carry no packet data.
			if strings.HasPrefix(line, `{"index"`) {
				continue
			}

			pkt, err := parseEKLine(line)
			if err != nil {
				// Non-fatal: log and continue. Malformed lines happen during
				// SCTP reassembly at capture start.
				log.Printf("⚠️  tshark: parse error: %v", err)
				continue
			}

			// Drop packets we could not identify as either generation.
			if pkt.Generation == "" {
				continue
			}

			select {
			case out <- *pkt:
			case <-procCtx.Done():
				return
			}
		}

		if err := scanner.Err(); err != nil && procCtx.Err() == nil {
			errc <- fmt.Errorf("tshark: scanner: %w", err)
			return
		}

		if err := cmd.Wait(); err != nil && procCtx.Err() == nil {
			errc <- fmt.Errorf("tshark: exited: %w", err)
			return
		}

		errc <- nil
	}()

	return out, errc
}

// parseEKLine parses a single tshark EK data line into a Packet.
func parseEKLine(line string) (*Packet, error) {
	var raw ekPacket
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, err
	}

	// EK data lines without a layers field are index lines that slipped through.
	if raw.Layers == nil {
		return nil, fmt.Errorf("no layers field")
	}

	pkt := &Packet{}

	// Timestamp: EK provides milliseconds; convert to time.Time with ms precision.
	// Frame time from the layers gives nanosecond precision — prefer that.
	// Timestamp may be a string or integer depending on tshark version.
	var tsMillis int64
	if err := json.Unmarshal(raw.Timestamp, &tsMillis); err != nil {
		// Try as quoted string
		var tsStr string
		if err2 := json.Unmarshal(raw.Timestamp, &tsStr); err2 == nil {
			fmt.Sscanf(tsStr, "%d", &tsMillis)
		}
	}
	pkt.Timestamp = time.UnixMilli(tsMillis).UTC()
	if frameRaw, ok := raw.Layers["frame"]; ok {
		var frame map[string]interface{}
		if err := json.Unmarshal(frameRaw, &frame); err == nil {
			if ts, ok := frame["frame_frame_time_epoch"].(string); ok {
				if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
					pkt.Timestamp = t
				}
			}
		}
	}

	// IP addresses.
	if ipRaw, ok := raw.Layers["ip"]; ok {
		var ip map[string]interface{}
		if err := json.Unmarshal(ipRaw, &ip); err == nil {
			pkt.SrcIP = strField(ip, "ip_ip_src")
			pkt.DstIP = strField(ip, "ip_ip_dst")
		}
	}

	// Determine generation from which protocol layer is present.
	if ngapRaw, ok := raw.Layers["ngap"]; ok {
		pkt.Generation = Generation5G
		parseNGAP(ngapRaw, pkt)
	} else if s1apRaw, ok := raw.Layers["s1ap"]; ok {
		pkt.Generation = Generation4G
		parseS1AP(s1apRaw, pkt)
	}

	log.Printf("🔬 parsed packet gen=%s proc4g=%d proc5g=%d nasType=%s",
    pkt.Generation, pkt.S1APProcedureCode, pkt.NGAPProcedureCode, pkt.NASMMType)
	return pkt, nil
}

// parseNGAP extracts NGAP and nested NAS-5GS fields.
// The ngap layer may be a JSON object or a JSON array (multiple PDUs per SCTP chunk).
// In the array case we process each element and merge the results — the first
// element that has a UE-relevant procedure code wins for procedure routing.
func parseNGAP(raw json.RawMessage, pkt *Packet) {
	// Try array first.
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, elem := range arr {
			parseNGAPObject(elem, pkt)
			// Stop after we have found a UE procedure code.
			if pkt.NGAPProcedureCode != 0 {
				return
			}
		}
		return
	}

	// Single object.
	parseNGAPObject(raw, pkt)
}

func parseNGAPObject(raw json.RawMessage, pkt *Packet) {
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return
	}

	pkt.NGAPProcedureCode = intField(obj, "ngap_ngap_procedureCode")
	pkt.RANUENGAPId = strField(obj, "ngap_ngap_RAN_UE_NGAP_ID")
	pkt.AMFUENGAPId = strField(obj, "ngap_ngap_AMF_UE_NGAP_ID")

	// NAS-5GS is nested inside the ngap object under the key "nas-5gs".
	if nasRaw, ok := obj["nas-5gs"]; ok {
    var nas map[string]interface{}
    nasBytes, _ := json.Marshal(nasRaw)
    if err := json.Unmarshal(nasBytes, &nas); err == nil {
        pkt.NASMMType = strField(nas, "nas-5gs_nas_5gs_mm_message_type")
        pkt.SUCIMsin = strField(nas, "nas-5gs_nas_5gs_mm_suci_msin")
        log.Printf("🔬 NAS extracted: mmType=%s suci=%s nasKeys=%v",
            pkt.NASMMType, pkt.SUCIMsin, func() []string {
                keys := make([]string, 0, len(nas))
                for k := range nas { keys = append(keys, k) }
                return keys
            }())
    } else {
        log.Printf("🔬 NAS unmarshal error: %v", err)
    }
} else {
    log.Printf("🔬 no nas-5gs key in ngap obj, keys present: %v", func() []string {
        keys := make([]string, 0, len(obj))
        for k := range obj { keys = append(keys, k) }
        return keys
    }())
}
}

// parseS1AP extracts S1AP and nested NAS-EPS fields.
// Same list-or-object handling as NGAP.
func parseS1AP(raw json.RawMessage, pkt *Packet) {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, elem := range arr {
			parseS1APObject(elem, pkt)
			if pkt.S1APProcedureCode != 0 {
				return
			}
		}
		return
	}
	parseS1APObject(raw, pkt)
}

func parseS1APObject(raw json.RawMessage, pkt *Packet) {
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return
	}

	pkt.S1APProcedureCode = intField(obj, "s1ap_s1ap_procedureCode")
	pkt.ENBUUES1APID = strField(obj, "s1ap_s1ap_ENB_UE_S1AP_ID")
	pkt.MMEUUES1APID = strField(obj, "s1ap_s1ap_MME_UE_S1AP_ID")

	// NAS-EPS is nested inside the s1ap object under "nas-eps".
	if nasRaw, ok := obj["nas-eps"]; ok {
		var nas map[string]interface{}
		nasBytes, _ := json.Marshal(nasRaw)
		if err := json.Unmarshal(nasBytes, &nas); err == nil {
			pkt.NASEMMType = strField(nas, "nas-eps_nas_eps_nas_msg_emm_type")
			pkt.NASESMType = strField(nas, "nas-eps_nas_eps_nas_msg_esm_type")
			pkt.IMSI = strField(nas, "e212_e212_imsi")
		}
	}
}

// --- helpers ----------------------------------------------------------------

// strField extracts a string value from a map, returning "" if absent or wrong type.
func strField(m map[string]interface{}, key string) string {
    v, ok := m[key]
    if !ok {
        return ""
    }
    if v == nil {
        return ""
    }
    switch s := v.(type) {
    case string:
        return s
    case float64:
        return fmt.Sprintf("%g", s)
    case bool:
        return fmt.Sprintf("%v", s)
    case json.Number:
        return s.String()
    case []interface{}:
        if len(s) > 0 {
            if str, ok := s[0].(string); ok {
                return str
            }
            return fmt.Sprintf("%v", s[0])
        }
        return ""
    case map[string]interface{}:
        return ""
    default:
        return fmt.Sprintf("%v", v)
    }
}
// intField extracts an integer value from a map. JSON numbers unmarshal as
// float64 by default with interface{}, so we handle that explicitly.
func intField(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		var i int
		fmt.Sscanf(n, "%d", &i)
		return i
	default:
		return 0
	}
}
