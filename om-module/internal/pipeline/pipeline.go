package pipeline

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Parz1val02/OM_module/internal/capture"
	"github.com/Parz1val02/OM_module/internal/collector"
	dockerclient "github.com/Parz1val02/OM_module/internal/docker"
	"github.com/Parz1val02/OM_module/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const networkName = "docker_open5gs_default"

// Pipeline reads packets from the capture manager and emits one span per packet.
type Pipeline struct {
	mcc     string
	mnc     string
	docker  *dockerclient.Client
	snap    *collector.Snapshot
	metrics *Metrics
}

// New creates a Pipeline. metrics may be nil if Prometheus is not enabled.
func New(mcc, mnc string, docker *dockerclient.Client, snap *collector.Snapshot, metrics *Metrics) *Pipeline {
	return &Pipeline{
		mcc:     mcc,
		mnc:     mnc,
		docker:  docker,
		snap:    snap,
		metrics: metrics,
	}
}

// Run reads packets from pkts and emits one span per packet to Tempo.
// Blocks until ctx is cancelled or pkts is closed.
func (p *Pipeline) Run(ctx context.Context, pkts <-chan capture.Packet) {
	log.Printf("📡 Pipeline started — emitting one span per packet")

	// Build IP→NF map once at start; refresh every 60 seconds.
	ipToNF := p.buildIPToNFMap(ctx)
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case pkt, ok := <-pkts:
			if !ok {
				return
			}
			// Ignore heartbeats and keepalives — they add noise with no value
			if isHeartbeat(pkt) {
				continue
			}
			go p.emitSpan(ctx, pkt, ipToNF)

		case <-ticker.C:
			// Refresh NF map periodically in case containers restart
			ipToNF = p.buildIPToNFMap(ctx)

		case <-ctx.Done():
			log.Printf("📡 Pipeline stopped")
			return
		}
	}
}

// emitSpan creates and immediately closes one OpenTelemetry span for a packet.
func (p *Pipeline) emitSpan(ctx context.Context, pkt capture.Packet, ipToNF map[string]string) {
	tracer := tracing.Tracer()

	name := spanName(pkt)
	if name == "" {
		return
	}

	// Resolve NF names from IPs
	srcNF := ipToNF[pkt.SrcIP]
	if srcNF == "" {
		srcNF = pkt.SrcIP
	}
	dstNF := ipToNF[pkt.DstIP]
	if dstNF == "" {
		dstNF = pkt.DstIP
	}

	// Resolve generation from packet or from IP if not set (PFCP has no generation)
	generation := pkt.Generation
	if generation == "" {
		generation = resolveGeneration(pkt.SrcIP, pkt.DstIP, ipToNF)
	}

	// Record Prometheus metrics
	if p.metrics != nil {
		protocol := strings.ToUpper(pkt.Protocol)
		p.metrics.PacketsTotal.WithLabelValues(protocol, generation, srcNF, dstNF).Inc()

		if pkt.Protocol == "sbi" && pkt.SBIMethod != "" {
			p.metrics.PacketsByService.WithLabelValues(
				pkt.SBIService, pkt.SBIMethod, srcNF, dstNF,
			).Inc()
		}

		if isErrorCause(pkt) {
			p.metrics.ErrorsTotal.WithLabelValues(protocol, generation, srcNF, dstNF).Inc()
		}
	}

	// Collect IMSI from whichever protocol field has it
	imsi := packetIMSI(pkt)

	// Determine message direction based on whether src is the core NF
	direction := messageDirection(pkt, ipToNF)

	// Extract protocol-specific attributes
	procedure, nasMsg, teid, seid, apn, cause := protocolAttrs(pkt)

	attrs := []attribute.KeyValue{
		attribute.String("source", "capture"),
		attribute.String("generation", generation),
		attribute.String("protocol", strings.ToUpper(pkt.Protocol)),
		attribute.String("src_ip", pkt.SrcIP),
		attribute.String("dst_ip", pkt.DstIP),
		attribute.String("src_nf", srcNF),
		attribute.String("dst_nf", dstNF),
		attribute.String("mcc", p.mcc),
		attribute.String("mnc", p.mnc),
		attribute.String("imsi", imsi),
		attribute.String("procedure", procedure),
		attribute.String("message_direction", direction),
	}

	// Only add optional attributes if non-empty to keep spans clean
	if nasMsg != "" {
		attrs = append(attrs, attribute.String("nas_message", nasMsg))
	}
	if teid != "" {
		attrs = append(attrs, attribute.String("teid", teid))
	}
	if seid != "" {
		attrs = append(attrs, attribute.String("seid", seid))
	}
	if apn != "" {
		attrs = append(attrs, attribute.String("apn_dnn", apn))
	}
	if cause != "" {
		attrs = append(attrs, attribute.String("cause", cause))
	}
	if pkt.Protocol == "sbi" {
		if pkt.SBIPath != "" {
			attrs = append(attrs, attribute.String("sbi_path", pkt.SBIPath))
		}
		if pkt.SBIUserAgent != "" {
			attrs = append(attrs, attribute.String("sbi_nf", pkt.SBIUserAgent))
		}
	}

	// Duration: single packets have no inherent duration.
	// We give each span 1ms so Tempo renders it visibly in the waterfall.
	start := pkt.Timestamp
	end := start.Add(time.Millisecond)

	_, span := tracer.Start(ctx, name,
		oteltrace.WithTimestamp(start),
		oteltrace.WithAttributes(attrs...),
	)

	// Mark error spans (cause != 16/success for GTPv2, cause != 1 for PFCP)
	if isErrorCause(pkt) {
		span.SetStatus(codes.Error, fmt.Sprintf("cause=%s", cause))
	}

	span.End(oteltrace.WithTimestamp(end))
}

// --- Helpers ----------------------------------------------------------------

// isHeartbeat returns true for PFCP heartbeats, GTPv2 echo, Diameter
// Device-Watchdog, and SBI NRF heartbeat PATCH messages which add noise.
func isHeartbeat(pkt capture.Packet) bool {
	switch pkt.Protocol {
	case "pfcp":
		return pkt.PFCPMessageType == 1 || pkt.PFCPMessageType == 2
	case "gtpv2":
		return pkt.GTPv2MessageType == 1 || pkt.GTPv2MessageType == 2
	case "diameter":
		return pkt.DiameterCmdCode == 280 // Device-Watchdog
	case "sbi":
		// NRF heartbeat: PATCH /nnrf-nfm/v1/nf-instances/{id}
		return pkt.SBIMethod == "PATCH" &&
			strings.Contains(pkt.SBIPath, "/nnrf-nfm/") &&
			strings.Contains(pkt.SBIPath, "/nf-instances/")
	}
	return false
}

// spanName returns a human-readable name for the span based on protocol.
func spanName(pkt capture.Packet) string {
	switch pkt.Protocol {
	case "ngap":
		return ngapSpanName(pkt)
	case "s1ap":
		return s1apSpanName(pkt)
	case "gtpv2":
		return gtpv2SpanName(pkt)
	case "pfcp":
		return pfcpSpanName(pkt)
	case "diameter":
		return diameterSpanName(pkt)
	case "sbi":
		return sbiSpanName(pkt)
	}
	return ""
}

// ngapSpanName builds the span name for an NGAP packet.
func ngapSpanName(pkt capture.Packet) string {
	proc := ngapProcedureName(pkt.NGAPProcedureCode)
	nas := nasMM5GName(pkt.NASMMType)
	if nas != "" {
		return fmt.Sprintf("NGAP:%s / NAS:%s", proc, nas)
	}
	return fmt.Sprintf("NGAP:%s", proc)
}

// s1apSpanName builds the span name for an S1AP packet.
func s1apSpanName(pkt capture.Packet) string {
	proc := s1apProcedureName(pkt.S1APProcedureCode)
	nas := nasEMMName(pkt.NASEMMType)
	if nas != "" {
		return fmt.Sprintf("S1AP:%s / NAS:%s", proc, nas)
	}
	return fmt.Sprintf("S1AP:%s", proc)
}

// gtpv2SpanName builds the span name for a GTPv2 packet.
func gtpv2SpanName(pkt capture.Packet) string {
	names := map[int]string{
		32:  "CreateSessionRequest",
		33:  "CreateSessionResponse",
		34:  "ModifyBearerRequest",
		35:  "ModifyBearerResponse",
		36:  "DeleteSessionRequest",
		37:  "DeleteSessionResponse",
		70:  "DownlinkDataNotification",
		71:  "DownlinkDataNotificationAck",
		170: "DeleteBearerRequest",
		171: "DeleteBearerResponse",
		176: "CreateBearerRequest",
		177: "CreateBearerResponse",
	}
	if n, ok := names[pkt.GTPv2MessageType]; ok {
		return fmt.Sprintf("GTPv2:%s", n)
	}
	return fmt.Sprintf("GTPv2:type_%d", pkt.GTPv2MessageType)
}

// pfcpSpanName builds the span name for a PFCP packet.
func pfcpSpanName(pkt capture.Packet) string {
	names := map[int]string{
		1:  "HeartbeatRequest",
		2:  "HeartbeatResponse",
		50: "SessionEstablishmentRequest",
		51: "SessionEstablishmentResponse",
		52: "SessionModificationRequest",
		53: "SessionModificationResponse",
		54: "SessionDeletionRequest",
		55: "SessionDeletionResponse",
		56: "SessionReportRequest",
		57: "SessionReportResponse",
	}
	if n, ok := names[pkt.PFCPMessageType]; ok {
		return fmt.Sprintf("PFCP:%s", n)
	}
	return fmt.Sprintf("PFCP:type_%d", pkt.PFCPMessageType)
}

// packetIMSI extracts the IMSI from whichever protocol field carries it.
func packetIMSI(pkt capture.Packet) string {
	switch pkt.Protocol {
	case "s1ap":
		return pkt.IMSI
	case "ngap":
		return pkt.IMSI
	case "gtpv2":
		return pkt.GTPv2IMSI
	case "pfcp":
		return pkt.PFCPIMSI
	case "diameter":
		return pkt.DiameterIMSI
	case "sbi":
		return pkt.SBIIMSI
	}
	return ""
}

// protocolAttrs extracts protocol-specific span attributes.
func protocolAttrs(pkt capture.Packet) (procedure, nasMsg, teid, seid, apn, cause string) {
	switch pkt.Protocol {
	case "ngap":
		procedure = ngapProcedureName(pkt.NGAPProcedureCode)
		nasMsg = nasMM5GName(pkt.NASMMType)
	case "s1ap":
		procedure = s1apProcedureName(pkt.S1APProcedureCode)
		nasMsg = nasEMMName(pkt.NASEMMType)
	case "gtpv2":
		procedure = gtpv2SpanName(pkt)
		teid = pkt.GTPv2TEID
		apn = pkt.GTPv2APN
		cause = pkt.GTPv2Cause
	case "pfcp":
		procedure = pfcpSpanName(pkt)
		seid = pkt.PFCPSEID
		apn = pkt.PFCPDNN
		cause = pkt.PFCPCause
	case "diameter":
		procedure = diameterSpanName(pkt)
		cause = pkt.DiameterResultCode
	case "sbi":
		procedure = pkt.SBIService
		cause = pkt.SBIStatus
	}
	return
}

// messageDirection returns "request" or "response" based on protocol.
// For NGAP/S1AP/GTPv2/PFCP: based on whether src is a core NF.
// For Diameter: directly from the R flag in the Diameter header.
func messageDirection(pkt capture.Packet, ipToNF map[string]string) string {
	if pkt.Protocol == "diameter" {
		if pkt.DiameterIsRequest {
			return "request"
		}
		return "response"
	}
	srcNF := ipToNF[pkt.SrcIP]
	coreNFs := map[string]bool{
		"amf": true, "mme": true, "smf": true,
		"pgw": true, "sgw": true, "upf": true,
	}
	if coreNFs[srcNF] {
		return "response"
	}
	return "request"
}

// diameterSpanName builds the span name for a Diameter packet.
func diameterSpanName(pkt capture.Packet) string {
	names := map[int]string{
		257: "Capabilities-Exchange",
		258: "Re-Auth",
		272: "Credit-Control", // Gx: PGW↔PCRF
		274: "Abort-Session",
		275: "Session-Termination",
		282: "Disconnect-Peer",
		316: "Update-Location",            // S6a: MME↔HSS
		318: "Authentication-Information", // S6a: MME↔HSS
		321: "Insert-Subscriber-Data",
		322: "Delete-Subscriber-Data",
		323: "Purge-UE",
	}
	direction := "Response"
	if pkt.DiameterIsRequest {
		direction = "Request"
	}
	if n, ok := names[pkt.DiameterCmdCode]; ok {
		return fmt.Sprintf("Diameter:%s%s", n, direction)
	}
	return fmt.Sprintf("Diameter:cmd_%d%s", pkt.DiameterCmdCode, direction)
}

// sbiSpanName builds the span name for a 5G SBI HTTP/2 packet.
// Format: SBI:METHOD /service e.g. "SBI:POST nausf-auth"
// For responses: "SBI:200 nausf-auth"
func sbiSpanName(pkt capture.Packet) string {
	service := pkt.SBIService
	if service == "" {
		service = "unknown"
	}
	if pkt.SBIMethod != "" {
		return fmt.Sprintf("SBI:%s %s", pkt.SBIMethod, service)
	}
	if pkt.SBIStatus != "" {
		return fmt.Sprintf("SBI:%s %s", pkt.SBIStatus, service)
	}
	return fmt.Sprintf("SBI:%s", service)
}
func isErrorCause(pkt capture.Packet) bool {
	switch pkt.Protocol {
	case "gtpv2":
		return pkt.GTPv2Cause != "" && pkt.GTPv2Cause != "16"
	case "pfcp":
		return pkt.PFCPCause != "" && pkt.PFCPCause != "1"
	case "diameter":
		return pkt.DiameterResultCode != "" && pkt.DiameterResultCode != "2001"
	case "sbi":
		if pkt.SBIStatus != "" {
			var code int
			fmt.Sscanf(pkt.SBIStatus, "%d", &code)
			return code >= 400
		}
	}
	return false
}

// buildIPToNFMap joins Docker network IPs with collector snapshot NF labels.
func (p *Pipeline) buildIPToNFMap(ctx context.Context) map[string]string {
	ipToName, err := p.docker.GetNetworkContainerIPs(ctx, networkName)
	if err != nil {
		log.Printf("⚠️  IP→NF resolution failed: %v", err)
		return map[string]string{}
	}
	nameToNF := p.snap.NameToNFMap()

	// RAN container name → friendly label
	ranNames := map[string]string{
		// srsRAN 4G (E1/E2)
		"srsenb_zmq":         "enb",
		"srsenb_zmq2":        "enb",
		"srsenb_zmq3":        "enb",
		"srsenb_zmq4":        "enb",
		"srsue_zmq":          "ue",
		"srsue_zmq_bad_ki":   "ue",
		"srsue_zmq_bad_imsi": "ue",
		"srsue_zmq_bad_apn":  "ue",
		// srsRAN 5G (E3/E4)
		"srsgnb_zmq":   "gnb",
		"srsue_5g_zmq": "ue",
		// UERANSIM (E4)
		"nr_gnb":         "gnb",
		"nr_gnb2":        "gnb",
		"nr_ue":          "ue",
		"nr_ue2":         "ue",
		"nr_ue3":         "ue",
		"nr_ue_bad_supi": "ue",
		"nr_ue_bad_ki":   "ue",
		"nr_ue_bad_dnn":  "ue",
		"nr_ue_bad_sst":  "ue",
	}

	result := make(map[string]string, len(ipToName))
	for ip, name := range ipToName {
		if nf, ok := nameToNF[name]; ok {
			result[ip] = nf
		} else if ran, ok := ranNames[name]; ok {
			result[ip] = ran
		} else {
			result[ip] = name
		}
	}
	return result
}

// --- Protocol name lookup tables --------------------------------------------

func ngapProcedureName(code int) string {
	names := map[int]string{
		0:  "AMFConfigurationUpdate",
		4:  "DownlinkNASTransport",
		14: "InitialContextSetup",
		15: "InitialUEMessage",
		20: "NGReset",
		21: "NGSetup",
		24: "UEContextModification",
		25: "PathSwitchRequest",
		29: "PDUSessionResourceSetup",
		38: "PDUSessionResourceRelease",
		40: "UEContextRelease",
		41: "UERadioCapabilityCheck",
		42: "UERadioCapabilityIDMapping",
		43: "UERadioCapabilityInfoIndication",
		44: "UECapabilityInfoIndication",
		46: "UplinkNASTransport",
		48: "RerouteNASRequest",
		52: "LocationReportingControl",
		60: "PDUSessionResourceNotify",
		63: "HandoverCancel",
		64: "HandoverRequired",
		65: "HandoverCommand",
	}
	if n, ok := names[code]; ok {
		return n
	}
	return fmt.Sprintf("proc_%d", code)
}

func s1apProcedureName(code int) string {
	names := map[int]string{
		0:  "HandoverPreparation",
		1:  "HandoverResourceAllocation",
		9:  "InitialContextSetup",
		11: "DownlinkNASTransport",
		12: "InitialUEMessage",
		13: "UplinkNASTransport",
		17: "S1Setup",
		18: "UEContextRelease",
		22: "UECapabilityInfoIndication",
		23: "UEContextReleaseRequest",
	}
	if n, ok := names[code]; ok {
		return n
	}
	return fmt.Sprintf("proc_%d", code)
}

func nasMM5GName(msgType string) string {
	names := map[string]string{
		"0x41": "Registration Request",
		"0x42": "Registration Accept",
		"0x43": "Registration Complete",
		"0x44": "Registration Reject",
		"0x45": "Deregistration Request",
		"0x46": "Deregistration Accept",
		"0x56": "Authentication Request",
		"0x57": "Authentication Response",
		"0x58": "Authentication Reject",
		"0x5a": "Authentication Failure",
		"0x5c": "Identity Request",
		"0x5d": "Security Mode Command",
		"0x5e": "Security Mode Complete",
		"0x5f": "Security Mode Reject",
	}
	if n, ok := names[strings.ToLower(msgType)]; ok {
		return n
	}
	return ""
}

// resolveGeneration infers 4g or 5g from the NF names involved in a packet.
// Used for PFCP packets which have no generation set by the parser.
// 4G PFCP: sgwc/sgwu (Sxa) and smf/upf acting as pgwc/pgwu (Sxb)
// 5G PFCP: smf/upf (N4)
// Note: smf and upf appear in both — we disambiguate by checking if sgwc/sgwu
// are also present in the ipToNF map (meaning 4G core is running).
func resolveGeneration(srcIP, dstIP string, ipToNF map[string]string) string {
	srcNF := ipToNF[srcIP]
	dstNF := ipToNF[dstIP]

	nf4g := map[string]bool{
		"mme": true, "sgwc": true, "sgwu": true,
		"pgwc": true, "pgwu": true, "hss": true, "pcrf": true,
	}
	nf5g := map[string]bool{
		"amf": true, "ausf": true, "udm": true, "udr": true,
		"pcf": true, "nrf": true, "nssf": true, "scp": true, "bsf": true,
	}

	for _, nf := range []string{srcNF, dstNF} {
		if nf4g[nf] {
			return "4g"
		}
		if nf5g[nf] {
			return "5g"
		}
	}

	// smf and upf exist in both generations — check if any 4G-specific NF
	// is present in the overall IP map to determine which core is running
	for _, nf := range ipToNF {
		if nf == "sgwc" || nf == "sgwu" || nf == "mme" {
			return "4g"
		}
	}

	return "5g" // default to 5g if only smf/upf visible
}

func nasEMMName(msgType string) string {
	names := map[string]string{
		"0x41": "Attach Request",
		"0x42": "Attach Accept",
		"0x43": "Attach Complete",
		"0x44": "Attach Reject",
		"0x45": "Detach Request",
		"0x46": "Detach Accept",
		"0x50": "Tracking Area Update Request",
		"0x51": "Tracking Area Update Accept",
		"0x52": "Authentication Request",
		"0x53": "Authentication Response",
		"0x54": "Authentication Reject",
		"0x55": "Identity Request",
		"0x56": "Identity Response",
		"0x5d": "Security Mode Command",
		"0x5e": "Security Mode Complete",
		"0x5f": "Security Mode Reject",
		"0x61": "EMM Information",
	}
	if n, ok := names[strings.ToLower(msgType)]; ok {
		return n
	}
	return ""
}
