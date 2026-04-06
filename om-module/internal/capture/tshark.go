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

	// Protocol identifies the protocol of this packet:
	// "s1ap", "ngap", "gtpv2", "pfcp"
	Protocol string

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

	// --- GTPv2-C fields (4G only, UDP 2123) ---
	GTPv2MessageType int    // 32=CreateSessionReq, 33=CreateSessionResp, 34=ModifyBearerReq, 35=ModifyBearerResp
	GTPv2Seq         string // hex sequence number e.g. "0x000001" — correlation key
	GTPv2TEID        string // tunnel endpoint ID, "0x00000000" on first request
	GTPv2IMSI        string // only in Create Session Request
	GTPv2APN         string // APN/DNN e.g. "internet"
	GTPv2Cause       string // "16" = Request Accepted
	GTPv2UEIP        string // UE IP address, assigned in Create Session Response
	GTPv2EBI         string // EPS Bearer ID

	// --- PFCP fields (4G and 5G, UDP 8805) ---
	PFCPMessageType int    // 50=EstReq, 51=EstResp, 52=ModReq, 53=ModResp, 54=DelReq, 55=DelResp
	PFCPSeqNo       int    // sequence number — correlation key before SEID known
	PFCPSEID        string // session endpoint ID (may be array on establishment)
	PFCPIMSI        string // only in Session Establishment Request
	PFCPUEIP        string // UE IP address
	PFCPDNN         string // data network name e.g. "internet"
	PFCPCause       string // "1" = success

	// --- Diameter fields (4G only, TCP 3868/3873/5868) ---
	DiameterCmdCode    int    // 318=AIR, 316=ULR, 272=CCR, 275=STR, 280=DWR
	DiameterIsRequest  bool   // true if R flag set in Diameter flags
	DiameterIMSI       string // from e212_e212_imsi or Subscription-Id-Data
	DiameterSessionID  string // Diameter Session-Id AVP
	DiameterResultCode string // "2001" = DIAMETER_SUCCESS
	DiameterOriginHost string // origin NF hostname e.g. "mme.epc..."

	// --- SBI HTTP/2 fields (5G only, TCP 7777) ---
	SBIMethod    string // HTTP method: GET, POST, PUT, PATCH, DELETE
	SBIPath      string // full API path e.g. /nausf-auth/v1/ue-authentications
	SBIStatus    string // HTTP status code on responses e.g. "200"
	SBIService   string // service name extracted from path e.g. "nausf-auth"
	SBIUserAgent string // NF name from user-agent header e.g. "AMF"
	SBIIMSI      string // IMSI extracted from path if present
}

// ekPacket is the raw EK JSON structure emitted by tshark -T ek.
// The format alternates: an index line ({"index":{...}}) then a data line
// ({"timestamp":...,"layers":{...}}). We only care about data lines.
// Timestamp is json.RawMessage because tshark version determines whether
// it is emitted as an integer or a quoted string.
type ekPacket struct {
	Timestamp json.RawMessage            `json:"timestamp"`
	Layers    map[string]json.RawMessage `json:"layers"`
}

// tsharkProcess wraps a running tshark subprocess and its stdout pipe.
type tsharkProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	out    chan Packet
	errc   chan error
}

// startTshark launches tshark on the given interface with explicit BPF capture
// filter and display filter, returning a channel of parsed Packet values.
// The returned error channel receives at most one value on exit.
func startTshark(ctx context.Context, iface, bpf, display string) (<-chan Packet, <-chan error) {
	out := make(chan Packet, 256)
	errc := make(chan error, 1)

	procCtx, cancel := context.WithCancel(ctx)

	args := []string{
		"-i", iface,
		"-f", bpf,
		"-Y", display,
		"-T", "ek",
		"-l", // flush after each packet
		"-n", // disable name resolution
		// Decode TCP port 7777 as HTTP/2 (Open5GS SBI uses h2c without upgrade)
		"-d", "tcp.port==7777,http2",
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
		cmd.Process.Pid, iface, bpf, display)

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

			// Drop packets we could not identify at all.
			// PFCP packets have no generation set here — the correlator
			// determines generation from IP addresses. Allow them through.
			if pkt.Generation == "" && pkt.Protocol == "" {
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
	var tsMillis int64
	if err := json.Unmarshal(raw.Timestamp, &tsMillis); err != nil {
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

	// Determine protocol and generation from which layer is present.
	// Priority: ngap > s1ap > gtpv2 > pfcp > diameter > http2
	if ngapRaw, ok := raw.Layers["ngap"]; ok {
		pkt.Generation = Generation5G
		pkt.Protocol = "ngap"
		parseNGAP(ngapRaw, pkt)
	} else if s1apRaw, ok := raw.Layers["s1ap"]; ok {
		pkt.Generation = Generation4G
		pkt.Protocol = "s1ap"
		parseS1AP(s1apRaw, pkt)
	} else if gtpv2Raw, ok := raw.Layers["gtpv2"]; ok {
		pkt.Generation = Generation4G
		pkt.Protocol = "gtpv2"
		parseGTPv2(gtpv2Raw, pkt)
	} else if pfcpRaw, ok := raw.Layers["pfcp"]; ok {
		pkt.Protocol = "pfcp"
		parseGTPv2OrPFCP_PFCP(pfcpRaw, pkt)
	} else if diameterRaw, ok := raw.Layers["diameter"]; ok {
		pkt.Generation = Generation4G
		pkt.Protocol = "diameter"
		parseDiameter(diameterRaw, pkt)
	} else if http2Raw, ok := raw.Layers["http2"]; ok {
		pkt.Generation = Generation5G
		pkt.Protocol = "sbi"
		parseSBI(http2Raw, pkt)
	}

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
			pkt.NASMMType = strField(nas, "nas-5gs_nas-5gs_mm_message_type")
			pkt.SUCIMsin = strField(nas, "nas-5gs_nas-5gs_mm_suci_msin")
		}
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
			pkt.NASEMMType = strField(nas, "nas-eps_nas-eps_nas_msg_emm_type")
			pkt.NASESMType = strField(nas, "nas-eps_nas-eps_nas_msg_esm_type")
			pkt.IMSI = strField(nas, "e212_e212_imsi")
		}
	}
}

// --- helpers ----------------------------------------------------------------

// strField extracts a string value from a map, returning "" if absent or wrong type.
// Handles arrays by returning the first element (tshark sometimes wraps values in arrays).
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
		// tshark sometimes wraps single values in an array — take the first element
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

// --- GTPv2-C parser ---------------------------------------------------------

// parseGTPv2 extracts GTPv2-C fields from the gtpv2 layer.
func parseGTPv2(raw json.RawMessage, pkt *Packet) {
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return
	}

	pkt.GTPv2MessageType = intField(obj, "gtpv2_gtpv2_message_type")
	pkt.GTPv2Seq = strField(obj, "gtpv2_gtpv2_seq")
	pkt.GTPv2TEID = strField(obj, "gtpv2_gtpv2_teid")
	pkt.GTPv2APN = strField(obj, "gtpv2_gtpv2_apn")
	pkt.GTPv2EBI = strField(obj, "gtpv2_gtpv2_ebi")
	pkt.GTPv2UEIP = strField(obj, "gtpv2_gtpv2_pdn_addr_and_prefix_ipv4")

	// Cause may be a scalar or array — strField handles arrays
	pkt.GTPv2Cause = strField(obj, "gtpv2_gtpv2_cause")

	// IMSI is nested under e212 fields at the gtpv2 layer level
	pkt.GTPv2IMSI = strField(obj, "e212_e212_imsi")
}

// --- Diameter parser --------------------------------------------------------

// parseDiameter extracts Diameter fields from the diameter layer.
func parseDiameter(raw json.RawMessage, pkt *Packet) {
	// Diameter may appear as array (multiple messages per TCP segment)
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		parseDiameterObject(arr[0], pkt)
		return
	}
	parseDiameterObject(raw, pkt)
}

func parseDiameterObject(raw json.RawMessage, pkt *Packet) {
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return
	}

	pkt.DiameterCmdCode = intField(obj, "diameter_diameter_cmd_code")
	pkt.DiameterSessionID = strField(obj, "diameter_diameter_Session-Id")
	pkt.DiameterResultCode = strField(obj, "diameter_diameter_Result-Code")
	pkt.DiameterOriginHost = strField(obj, "diameter_diameter_Origin-Host")

	// Determine request vs response from flags byte (bit 7 = R flag)
	flags := strField(obj, "diameter_diameter_flags")
	if flags != "" {
		var flagVal int64
		if len(flags) > 2 && flags[:2] == "0x" {
			fmt.Sscanf(flags[2:], "%x", &flagVal)
		} else {
			fmt.Sscanf(flags, "%d", &flagVal)
		}
		pkt.DiameterIsRequest = flagVal&0x80 != 0
	}

	// IMSI from e212 field (present in AIR, ULR, CCR requests)
	pkt.DiameterIMSI = strField(obj, "e212_e212_imsi")
	if pkt.DiameterIMSI == "" {
		// Fallback: Subscription-Id-Data AVP
		pkt.DiameterIMSI = strField(obj, "diameter_diameter_Subscription-Id-Data")
	}
}

// parseGTPv2OrPFCP_PFCP extracts PFCP fields from the pfcp layer.
// Named with the longer prefix to avoid collision — called parseGTPv2OrPFCP_PFCP
// because PFCP generation (4g/5g) is determined by the correlator from IP.
func parseGTPv2OrPFCP_PFCP(raw json.RawMessage, pkt *Packet) {
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return
	}

	pkt.PFCPMessageType = intField(obj, "pfcp_pfcp_msg_type")
	pkt.PFCPSeqNo = intField(obj, "pfcp_pfcp_seqno")
	pkt.PFCPDNN = strField(obj, "pfcp_pfcp_apn_dnn")
	pkt.PFCPUEIP = strField(obj, "pfcp_pfcp_ue_ip_addr_ipv4")
	pkt.PFCPCause = strField(obj, "pfcp_pfcp_cause")
	pkt.PFCPIMSI = strField(obj, "e212_e212_imsi")

	// SEID can be a scalar (on modification/deletion) or an array of two
	// values on session establishment (local SEID and remote SEID).
	// We store the first non-zero SEID value.
	seid := strField(obj, "pfcp_pfcp_seid")
	if seid == "" || seid == "0x0000000000000000" {
		// Try array form
		if arr, ok := obj["pfcp_pfcp_seid"].([]interface{}); ok {
			for _, s := range arr {
				if str, ok := s.(string); ok && str != "0x0000000000000000" {
					seid = str
					break
				}
			}
		}
	}
	pkt.PFCPSEID = seid
}

// --- SBI HTTP/2 parser ------------------------------------------------------

// parseSBI extracts 5G SBI fields from the http2 layer.
// Open5GS uses HTTP/2 with prior knowledge (h2c) on TCP port 7777.
// tshark must be invoked with -d tcp.port==7777,http2 to dissect it.
func parseSBI(raw json.RawMessage, pkt *Packet) {
	var obj map[string]interface{}

	// http2 layer may be an array when multiple frames share a TCP segment
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		// Find the first frame that has a method or status (HEADERS frame)
		for _, elem := range arr {
			var o map[string]interface{}
			if err := json.Unmarshal(elem, &o); err != nil {
				continue
			}
			if strField(o, "http2_http2_headers_method") != "" ||
				strField(o, "http2_http2_headers_status") != "" {
				obj = o
				break
			}
		}
		if obj == nil {
			return
		}
	} else {
		if err := json.Unmarshal(raw, &obj); err != nil {
			return
		}
	}

	pkt.SBIMethod = strField(obj, "http2_http2_headers_method")
	pkt.SBIPath = strField(obj, "http2_http2_headers_path")
	pkt.SBIStatus = strField(obj, "http2_http2_headers_status")
	pkt.SBIUserAgent = strField(obj, "http2_http2_headers_user_agent")

	// Skip packets with no method and no status — these are DATA frames
	// or SETTINGS/PING frames with no signalling value
	if pkt.SBIMethod == "" && pkt.SBIStatus == "" {
		pkt.Protocol = "" // will be dropped by the packet filter
		return
	}

	// Only process request frames (those with a method).
	// Response frames have no path so the service is unknown — they add noise.
	if pkt.SBIMethod == "" {
		pkt.Protocol = "" // drop response-only frames
		return
	}

	// Extract service name from path e.g. /nausf-auth/v1/... → nausf-auth
	if pkt.SBIPath != "" {
		parts := strings.SplitN(strings.TrimPrefix(pkt.SBIPath, "/"), "/", 3)
		if len(parts) >= 1 {
			pkt.SBIService = parts[0]
		}
	}

	// Extract IMSI from path if present e.g. /nudm-sdm/v2/imsi-001011234567895/...
	if idx := strings.Index(pkt.SBIPath, "imsi-"); idx >= 0 {
		rest := pkt.SBIPath[idx+5:]
		// IMSI ends at next / or end of string
		end := strings.IndexByte(rest, '/')
		if end < 0 {
			end = len(rest)
		}
		// Strip query string if present
		if q := strings.IndexByte(rest[:end], '?'); q >= 0 {
			end = q
		}
		pkt.SBIIMSI = rest[:end]
	}
}
