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
	log.Printf("Pipeline started — emitting one span per packet")

	// Build IP→NF map once at start; refresh every 60 seconds.
	ipToNF := p.buildIPToNFMap(ctx)
	ticker := time.NewTicker(10 * time.Second)
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
			if ipToNF[pkt.SrcIP] == "" || ipToNF[pkt.DstIP] == "" {
				ipToNF = p.buildIPToNFMap(ctx)
			}
			go p.emitSpan(ctx, pkt, ipToNF)

		case <-ticker.C:
			// Refresh NF map periodically in case containers restart
			ipToNF = p.buildIPToNFMap(ctx)

		case <-ctx.Done():
			log.Printf("Pipeline stopped")
			return
		}
	}
}

// emitSpan creates and immediately closes one OpenTelemetry span for a packet.
func (p *Pipeline) emitSpan(ctx context.Context, pkt capture.Packet, ipToNF map[string]string) {
	tracer := tracing.Tracer()

	info := packetSpanInfo(pkt)
	if info.name == "" {
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
	imsi := packetIMSI(pkt, p.mcc, p.mnc)

	// Determine message direction based on whether src is the core NF
	direction := messageDirection(pkt, ipToNF)

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
		attribute.String("procedure", info.procedure),
		attribute.String("message_direction", direction),
	}

	// Only add optional attributes if non-empty to keep spans clean
	if info.nasMsg != "" {
		attrs = append(attrs, attribute.String("nas_message", info.nasMsg))
	}
	if info.teid != "" {
		attrs = append(attrs, attribute.String("teid", info.teid))
	}
	if info.seid != "" {
		attrs = append(attrs, attribute.String("seid", info.seid))
	}
	if info.apn != "" {
		attrs = append(attrs, attribute.String("apn_dnn", info.apn))
	}
	if info.cause != "" {
		attrs = append(attrs, attribute.String("cause", info.cause))
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

	_, span := tracer.Start(ctx, info.name,
		oteltrace.WithTimestamp(start),
		oteltrace.WithAttributes(attrs...),
	)

	// Mark error spans (cause != 16/success for GTPv2, cause != 1 for PFCP)
	if isErrorCause(pkt) {
		span.SetStatus(codes.Error, fmt.Sprintf("cause=%s", info.cause))
	}

	span.End(oteltrace.WithTimestamp(end))
}

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

// spanInfo bundles the span name and protocol-specific attributes for one packet.
type spanInfo struct {
	name      string
	procedure string
	nasMsg    string
	teid      string
	seid      string
	apn       string
	cause     string
}

// packetSpanInfo returns a zero spanInfo (name == "") for packets that
// shouldn't be emitted as spans.
func packetSpanInfo(pkt capture.Packet) spanInfo {
	switch pkt.Protocol {
	case "ngap":
		proc := ngapProcedureName(pkt.NGAPProcedureCode)
		nas := nasMM5GName(pkt.NASMMType)
		name := fmt.Sprintf("NGAP:%s", proc)
		if nas != "" {
			name = fmt.Sprintf("NGAP:%s / NAS:%s", proc, nas)
		}
		return spanInfo{name: name, procedure: proc, nasMsg: nas}
	case "s1ap":
		proc := s1apProcedureName(pkt.S1APProcedureCode)
		nas := nasEMMName(pkt.NASEMMType)
		name := fmt.Sprintf("S1AP:%s", proc)
		if nas != "" {
			name = fmt.Sprintf("S1AP:%s / NAS:%s", proc, nas)
		}
		return spanInfo{name: name, procedure: proc, nasMsg: nas}
	case "gtpv2":
		name := gtpv2SpanName(pkt)
		return spanInfo{
			name: name, procedure: name,
			teid: pkt.GTPv2TEID, apn: pkt.GTPv2APN, cause: pkt.GTPv2Cause,
		}
	case "pfcp":
		name := pfcpSpanName(pkt)
		return spanInfo{
			name: name, procedure: name,
			seid: pkt.PFCPSEID, apn: pkt.PFCPDNN, cause: pkt.PFCPCause,
		}
	case "diameter":
		name := diameterSpanName(pkt)
		return spanInfo{name: name, procedure: name, cause: pkt.DiameterResultCode}
	case "sbi":
		return spanInfo{name: sbiSpanName(pkt), procedure: pkt.SBIService, cause: pkt.SBIStatus}
	}
	return spanInfo{}
}

// gtpv2MessageNames maps GTPv2-C message type codes to human-readable names.
var gtpv2MessageNames = map[int]string{
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

// gtpv2SpanName builds the span name for a GTPv2 packet.
func gtpv2SpanName(pkt capture.Packet) string {
	if n, ok := gtpv2MessageNames[pkt.GTPv2MessageType]; ok {
		return fmt.Sprintf("GTPv2:%s", n)
	}
	return fmt.Sprintf("GTPv2:type_%d", pkt.GTPv2MessageType)
}

// pfcpMessageNames maps PFCP message type codes to human-readable names.
var pfcpMessageNames = map[int]string{
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

// pfcpSpanName builds the span name for a PFCP packet.
func pfcpSpanName(pkt capture.Packet) string {
	if n, ok := pfcpMessageNames[pkt.PFCPMessageType]; ok {
		return fmt.Sprintf("PFCP:%s", n)
	}
	return fmt.Sprintf("PFCP:type_%d", pkt.PFCPMessageType)
}

// packetIMSI extracts or reconstructs the IMSI from whichever protocol field
// carries it. NGAP only carries the SUCI MSIN, so the full IMSI is rebuilt
// as MCC+MNC+MSIN.
func packetIMSI(pkt capture.Packet, mcc, mnc string) string {
	switch pkt.Protocol {
	case "s1ap":
		return pkt.IMSI
	case "ngap":
		if pkt.SUCIMsin == "" {
			return ""
		}
		return mcc + mnc + pkt.SUCIMsin
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

// coreNFs identifies which NF names are core-side, used to infer message
// direction for NGAP/S1AP/GTPv2/PFCP packets.
var coreNFs = map[string]bool{
	"amf": true, "mme": true, "smf": true, "smf2": true,
	"pgw": true, "sgw": true, "upf": true, "upf2": true,
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
	if coreNFs[srcNF] {
		return "response"
	}
	return "request"
}

// diameterCmdNames maps Diameter command codes to human-readable names.
var diameterCmdNames = map[int]string{
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

// diameterSpanName builds the span name for a Diameter packet.
func diameterSpanName(pkt capture.Packet) string {
	direction := "Response"
	if pkt.DiameterIsRequest {
		direction = "Request"
	}
	if n, ok := diameterCmdNames[pkt.DiameterCmdCode]; ok {
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
		log.Printf("IP→NF resolution failed: %v", err)
		return map[string]string{}
	}
	nameToNF := p.snap.NameToNFMap()

	// All testbed containers (core and RAN) carry an om.nf label, so
	// NameToNFMap already resolves every name we care about; fall back to
	// the raw container name only for anything genuinely unlabelled.
	result := make(map[string]string, len(ipToName))
	for ip, name := range ipToName {
		if nf, ok := nameToNF[name]; ok {
			result[ip] = nf
		} else {
			result[ip] = name
		}
	}
	return result
}

var ngapProcedureNames = map[int]string{
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

func ngapProcedureName(code int) string {
	if n, ok := ngapProcedureNames[code]; ok {
		return n
	}
	return fmt.Sprintf("proc_%d", code)
}

var s1apProcedureNames = map[int]string{
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

func s1apProcedureName(code int) string {
	if n, ok := s1apProcedureNames[code]; ok {
		return n
	}
	return fmt.Sprintf("proc_%d", code)
}

var nasMM5GNames = map[string]string{
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

func nasMM5GName(msgType string) string {
	return nasMM5GNames[strings.ToLower(msgType)]
}

// nf4GNames and nf5GNames disambiguate PFCP packets (which carry no
// generation of their own) by which NF names appear on either side.
var nf4GNames = map[string]bool{
	"mme": true, "sgwc": true, "sgwu": true,
	"pgwc": true, "pgwu": true, "hss": true, "pcrf": true,
}
var nf5GNames = map[string]bool{
	"amf": true, "ausf": true, "udm": true, "udr": true,
	"pcf": true, "nrf": true, "nssf": true, "scp": true, "bsf": true,
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

	for _, nf := range []string{srcNF, dstNF} {
		if nf4GNames[nf] {
			return "4g"
		}
		if nf5GNames[nf] {
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

var nasEMMNames = map[string]string{
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

func nasEMMName(msgType string) string {
	return nasEMMNames[strings.ToLower(msgType)]
}
