package correlator

import (
	"fmt"
	"strings"

	"github.com/Parz1val02/OM_module/internal/capture"
)

// S1AP procedure codes (4G)
const (
	S1APProcS1Setup                 = 17
	S1APProcInitialUEMessage        = 12
	S1APProcDownlinkNASTransport    = 11
	S1APProcUplinkNASTransport      = 13
	S1APProcInitialContextSetup     = 9
	S1APProcUECapabilityInfoInd     = 22
	S1APProcUEContextRelease        = 18
	S1APProcUEContextReleaseRequest = 23
)

// NGAP procedure codes (5G)
const (
	NGAPProcNGSetup                 = 21
	NGAPProcInitialUEMessage        = 15
	NGAPProcDownlinkNASTransport    = 4
	NGAPProcUplinkNASTransport      = 46
	NGAPProcInitialContextSetup     = 14
	NGAPProcUECapabilityInfoInd     = 44
	NGAPProcPDUSessionResourceSetup = 29
)

// NAS-EPS EMM message types (4G) — from nas-eps_nas-eps_nas_msg_emm_type
const (
	NASEMMAttachRequest    = "0x41"
	NASEMMAttachAccept     = "0x42"
	NASEMMAttachComplete   = "0x43"
	NASEMMIdentityRequest  = "0x55"
	NASEMMIdentityResponse = "0x56"
	NASEMMAuthRequest      = "0x52"
	NASEMMAuthResponse     = "0x53"
	NASEMMSecModeCommand   = "0x5d"
	NASEMMSecModeComplete  = "0x5e"
	NASEMMInformation      = "0x61"
)

// NAS-5GS MM message types (5G) — from nas-5gs_nas-5gs_mm_message_type
const (
	NASMMRegistrationRequest  = "0x41"
	NASMMRegistrationAccept   = "0x42"
	NASMMRegistrationComplete = "0x43"
	NASMMAuthRequest          = "0x56"
	NASMMAuthResponse         = "0x57"
	NASMMSecModeCommand       = "0x5d"
	NASMMSecModeComplete      = "0x5e"
)

// correlationKey4G returns the string used to look up or create a procedure
// entry for a 4G packet. Before the IMSI is known we use the ENB-UE-S1AP-ID.
// Once IMSI is available the procedure entry is re-keyed via upgradeKey4G.
func correlationKey4G(pkt *capture.Packet) string {
	if pkt.ENBUUES1APID != "" {
		return "4g:enb:" + pkt.ENBUUES1APID
	}
	return ""
}

// upgradeKey4G returns the stable IMSI-based key for a 4G procedure.
func upgradeKey4G(imsi string) string {
	return "4g:imsi:" + imsi
}

// correlationKey5G returns the string used to look up or create a procedure
// entry for a 5G packet. RAN-UE-NGAP-ID is stable for the lifetime of the
// procedure and is present from the very first InitialUEMessage.
func correlationKey5G(pkt *capture.Packet) string {
	if pkt.RANUENGAPId != "" {
		return "5g:ran:" + pkt.RANUENGAPId
	}
	return ""
}

// reconstructIMSI builds a full 15-digit IMSI from the SUCI MSIN.
// In 5G the Registration Request carries only the MSIN portion of the SUCI
// (e.g. "1234567895"). The full IMSI is MCC + MNC + MSIN.
//
// mcc example: "001", mnc example: "01"
func reconstructIMSI(mcc, mnc, msin string) string {
	return mcc + mnc + msin
}

// spanNameFor returns a human-readable span name for a packet,
// used as the OpenTelemetry span name in Tempo.
func spanNameFor(pkt *capture.Packet) string {
	switch pkt.Generation {
	case capture.Generation4G:
		return spanName4G(pkt)
	case capture.Generation5G:
		return spanName5G(pkt)
	default:
		return "Unknown"
	}
}

func spanName4G(pkt *capture.Packet) string {
	proc := s1apProcName(pkt.S1APProcedureCode)
	nas := nasEMMName(pkt.NASEMMType)
	if nas != "" {
		return fmt.Sprintf("S1AP:%s / NAS:%s", proc, nas)
	}
	return fmt.Sprintf("S1AP:%s", proc)
}

func spanName5G(pkt *capture.Packet) string {
	proc := ngapProcName(pkt.NGAPProcedureCode)
	nas := nasMMName(pkt.NASMMType)
	if nas != "" {
		return fmt.Sprintf("NGAP:%s / NAS:%s", proc, nas)
	}
	return fmt.Sprintf("NGAP:%s", proc)
}

func s1apProcName(code int) string {
	names := map[int]string{
		S1APProcS1Setup:                 "S1Setup",
		S1APProcInitialUEMessage:        "InitialUEMessage",
		S1APProcDownlinkNASTransport:    "DownlinkNASTransport",
		S1APProcUplinkNASTransport:      "UplinkNASTransport",
		S1APProcInitialContextSetup:     "InitialContextSetup",
		S1APProcUECapabilityInfoInd:     "UECapabilityInfoIndication",
		S1APProcUEContextRelease:        "UEContextRelease",
		S1APProcUEContextReleaseRequest: "UEContextReleaseRequest",
	}
	if n, ok := names[code]; ok {
		return n
	}
	return fmt.Sprintf("proc_%d", code)
}

func ngapProcName(code int) string {
	names := map[int]string{
		NGAPProcNGSetup:                 "NGSetup",
		NGAPProcInitialUEMessage:        "InitialUEMessage",
		NGAPProcDownlinkNASTransport:    "DownlinkNASTransport",
		NGAPProcUplinkNASTransport:      "UplinkNASTransport",
		NGAPProcInitialContextSetup:     "InitialContextSetup",
		NGAPProcUECapabilityInfoInd:     "UECapabilityInfoIndication",
		NGAPProcPDUSessionResourceSetup: "PDUSessionResourceSetup",
	}
	if n, ok := names[code]; ok {
		return n
	}
	return fmt.Sprintf("proc_%d", code)
}

func nasEMMName(msgType string) string {
	names := map[string]string{
		NASEMMAttachRequest:    "Attach Request",
		NASEMMAttachAccept:     "Attach Accept",
		NASEMMAttachComplete:   "Attach Complete",
		NASEMMIdentityRequest:  "Identity Request",
		NASEMMIdentityResponse: "Identity Response",
		NASEMMAuthRequest:      "Authentication Request",
		NASEMMAuthResponse:     "Authentication Response",
		NASEMMSecModeCommand:   "Security Mode Command",
		NASEMMSecModeComplete:  "Security Mode Complete",
		NASEMMInformation:      "EMM Information",
	}
	// Normalise to lowercase hex for map lookup.
	key := strings.ToLower(msgType)
	if n, ok := names[key]; ok {
		return n
	}
	return ""
}

func nasMMName(msgType string) string {
	names := map[string]string{
		NASMMRegistrationRequest:  "Registration Request",
		NASMMRegistrationAccept:   "Registration Accept",
		NASMMRegistrationComplete: "Registration Complete",
		NASMMAuthRequest:          "Authentication Request",
		NASMMAuthResponse:         "Authentication Response",
		NASMMSecModeCommand:       "Security Mode Command",
		NASMMSecModeComplete:      "Security Mode Complete",
	}
	key := strings.ToLower(msgType)
	if n, ok := names[key]; ok {
		return n
	}
	return ""
}

// --- Span attribute extraction helpers --------------------------------------
// These are used by emitTrace to enrich child span attributes.

// extractProtocol returns the protocol name from a span name.
// Span names follow the pattern "PROTOCOL:Procedure / NAS:Message" or
// "PROTOCOL:Procedure" for Phase 2+ protocols like "GTPv2:CreateSessionRequest".
func extractProtocol(spanName string) string {
	if idx := strings.Index(spanName, ":"); idx >= 0 {
		return spanName[:idx]
	}
	return ""
}

// extractNGAPProcedure returns the NGAP/S1AP procedure name from a span name.
// e.g. "NGAP:InitialUEMessage / NAS:Registration Request" → "InitialUEMessage"
// e.g. "S1AP:DownlinkNASTransport / NAS:Attach Accept" → "DownlinkNASTransport"
func extractNGAPProcedure(spanName string) string {
	// Format: "PROTOCOL:ProcedureName / NAS:Message" or "PROTOCOL:ProcedureName"
	after := spanName
	if idx := strings.Index(spanName, ":"); idx >= 0 {
		after = spanName[idx+1:]
	}
	// Strip everything from " /" onward
	if idx := strings.Index(after, " /"); idx >= 0 {
		after = after[:idx]
	}
	return strings.TrimSpace(after)
}

// extractNASMessage returns the NAS message name from a span name.
// e.g. "NGAP:InitialUEMessage / NAS:Registration Request" → "Registration Request"
// Returns "" if no NAS message is present in the span name.
func extractNASMessage(spanName string) string {
	const nasPrefix = "/ NAS:"
	idx := strings.Index(spanName, nasPrefix)
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(spanName[idx+len(nasPrefix):])
}

// messageDirection returns "request" or "response" based on the src IP
// relative to the known AMF/MME IP. For uplink messages (UE→core) the
// src is the gNB/eNB. For downlink (core→UE) the src is the AMF/MME.
// amfIP is the IP of the core-side NF (AMF for 5G, MME for 4G).
func messageDirection(srcIP, amfIP string) string {
	if srcIP == amfIP {
		return "response"
	}
	return "request"
}
