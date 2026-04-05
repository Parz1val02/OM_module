package correlator

import (
	"time"

	"github.com/Parz1val02/OM_module/internal/capture"
)

// ProcedureState represents where a UE procedure is in its lifecycle.
type ProcedureState int

const (
	StateIdle ProcedureState = iota

	// 4G attach states
	StateAttaching        // InitialUEMessage + Attach Request received
	StateIdentityPending  // Identity Request sent, waiting for response
	StateAuthenticating   // Identity Response received, IMSI now known
	StateAuthPending      // Authentication Request sent
	StateSecurityPending  // Authentication Response received
	StateSecModePending   // Security Mode Command sent
	StateContextPending   // Security Mode Complete received
	StateAttachCompleting // InitialContextSetup request (Attach Accept) sent
	StateAttachDone       // InitialContextSetup response received — DONE

	// 5G registration states
	StateRegistering        // InitialUEMessage + Registration Request received
	State5GAuthPending      // Authentication Request sent
	State5GSecurityPending  // Authentication Response received
	State5GSecModePending   // Security Mode Command sent
	State5GContextPending   // Security Mode Complete received (encrypted)
	State5GContextSetup     // InitialContextSetup request sent (Reg Accept inside)
	State5GRegistrationDone // InitialContextSetup response + final UplinkNAS — DONE
)

// ProcedureResult describes how the procedure ended.
type ProcedureResult string

const (
	ResultSuccess  ProcedureResult = "success"
	ResultTimeout  ProcedureResult = "timeout"
	ResultError    ProcedureResult = "error"
)

// SpanRecord holds the data for one span within a procedure.
// Each packet that advances the state machine becomes one span.
type SpanRecord struct {
	Name      string
	StartTime time.Time
	EndTime   time.Time // set when the next span starts
	SrcIP     string
	DstIP     string
}

// Procedure tracks the in-flight state for one UE procedure.
type Procedure struct {
	// Generation is "4g" or "5g".
	Generation string

	// ProcedureType is a human-readable name e.g. "4G EPC — UE Attach".
	ProcedureType string

	// IMSI is the subscriber identity. For 4G it is populated on Identity Response.
	// For 5G it is reconstructed from the SUCI MSIN on the first packet.
	IMSI string

	// State is the current position in the procedure state machine.
	State ProcedureState

	// Spans accumulates span records as the procedure progresses.
	Spans []SpanRecord

	// StartTime is the timestamp of the first packet.
	StartTime time.Time

	// LastSeen is updated on every packet. Used for timeout detection.
	LastSeen time.Time

	// ContextSetupCount tracks InitialContextSetup request/response pairs
	// needed to detect the terminal state in 5G.
	ContextSetupCount int

	// UplinkAfterContextSetup counts UplinkNASTransport messages received
	// after InitialContextSetup completes in 5G. The first one is
	// Registration Complete.
	UplinkAfterContextSetup int
}

// newProcedure creates a new Procedure seeded with the first packet.
func newProcedure(gen, procType, imsi string, pkt *capture.Packet) *Procedure {
	span := SpanRecord{
		Name:      spanNameFor(pkt),
		StartTime: pkt.Timestamp,
		SrcIP:     pkt.SrcIP,
		DstIP:     pkt.DstIP,
	}
	return &Procedure{
		Generation:    gen,
		ProcedureType: procType,
		IMSI:          imsi,
		State:         StateIdle,
		Spans:         []SpanRecord{span},
		StartTime:     pkt.Timestamp,
		LastSeen:      pkt.Timestamp,
	}
}

// addSpan appends a new span and closes the previous one's EndTime.
func (p *Procedure) addSpan(pkt *capture.Packet) {
	// Close the previous span.
	if len(p.Spans) > 0 {
		p.Spans[len(p.Spans)-1].EndTime = pkt.Timestamp
	}

	p.Spans = append(p.Spans, SpanRecord{
		Name:      spanNameFor(pkt),
		StartTime: pkt.Timestamp,
		SrcIP:     pkt.SrcIP,
		DstIP:     pkt.DstIP,
	})
	p.LastSeen = pkt.Timestamp
}

// advance4G processes one 4G packet through the attach state machine.
// Returns true if the procedure has reached a terminal state.
func (p *Procedure) advance4G(pkt *capture.Packet) (done bool) {
	p.addSpan(pkt)

	switch p.State {
	case StateAttaching:
		if pkt.S1APProcedureCode == S1APProcDownlinkNASTransport &&
			normaliseHex(pkt.NASEMMType) == NASEMMIdentityRequest {
			p.State = StateIdentityPending
		} else if pkt.S1APProcedureCode == S1APProcDownlinkNASTransport &&
			normaliseHex(pkt.NASEMMType) == NASEMMAuthRequest {
			// Some configurations skip Identity Request.
			p.State = StateAuthPending
		}

	case StateIdentityPending:
		if pkt.S1APProcedureCode == S1APProcUplinkNASTransport &&
			normaliseHex(pkt.NASEMMType) == NASEMMIdentityResponse {
			// IMSI is now available.
			if pkt.IMSI != "" {
				p.IMSI = pkt.IMSI
			}
			p.State = StateAuthenticating
		}

	case StateAuthenticating:
		if pkt.S1APProcedureCode == S1APProcDownlinkNASTransport &&
			normaliseHex(pkt.NASEMMType) == NASEMMAuthRequest {
			p.State = StateAuthPending
		}

	case StateAuthPending:
		if pkt.S1APProcedureCode == S1APProcUplinkNASTransport &&
			normaliseHex(pkt.NASEMMType) == NASEMMAuthResponse {
			p.State = StateSecurityPending
		}

	case StateSecurityPending:
		if pkt.S1APProcedureCode == S1APProcDownlinkNASTransport &&
			normaliseHex(pkt.NASEMMType) == NASEMMSecModeCommand {
			p.State = StateSecModePending
		}

	case StateSecModePending:
		if pkt.S1APProcedureCode == S1APProcUplinkNASTransport &&
			normaliseHex(pkt.NASEMMType) == NASEMMSecModeComplete {
			p.State = StateContextPending
		}

	case StateContextPending:
		if pkt.S1APProcedureCode == S1APProcInitialContextSetup &&
			normaliseHex(pkt.NASEMMType) == NASEMMAttachAccept {
			p.State = StateAttachCompleting
		}

	case StateAttachCompleting:
		// InitialContextSetup response (no NAS) — procedure complete.
		if pkt.S1APProcedureCode == S1APProcInitialContextSetup {
			// Close the last span with a tiny offset so it has non-zero duration.
			if len(p.Spans) > 0 {
				p.Spans[len(p.Spans)-1].EndTime = pkt.Timestamp.Add(time.Millisecond)
			}
			p.State = StateAttachDone
			return true
		}
	}

	return false
}

// advance5G processes one 5G packet through the registration state machine.
// Returns true if the procedure has reached a terminal state.
func (p *Procedure) advance5G(pkt *capture.Packet) (done bool) {
	p.addSpan(pkt)

	switch p.State {
	case StateRegistering:
		if pkt.NGAPProcedureCode == NGAPProcDownlinkNASTransport &&
			normaliseHex(pkt.NASMMType) == NASMMAuthRequest {
			p.State = State5GAuthPending
		}

	case State5GAuthPending:
		if pkt.NGAPProcedureCode == NGAPProcUplinkNASTransport &&
			normaliseHex(pkt.NASMMType) == NASMMAuthResponse {
			p.State = State5GSecurityPending
		}

	case State5GSecurityPending:
		if pkt.NGAPProcedureCode == NGAPProcDownlinkNASTransport &&
			normaliseHex(pkt.NASMMType) == NASMMSecModeCommand {
			p.State = State5GSecModePending
		}

	case State5GSecModePending:
		// Security Mode Complete is encrypted — NASMMType will be empty.
		// We detect it as any UplinkNASTransport after Security Mode Command.
		if pkt.NGAPProcedureCode == NGAPProcUplinkNASTransport {
			p.State = State5GContextPending
		}

	case State5GContextPending:
		if pkt.NGAPProcedureCode == NGAPProcInitialContextSetup {
			p.ContextSetupCount++
			if p.ContextSetupCount == 1 {
				p.State = State5GContextSetup
			}
		}

	case State5GContextSetup:
		// Second InitialContextSetup message (the response from gNB).
		if pkt.NGAPProcedureCode == NGAPProcInitialContextSetup {
			p.ContextSetupCount++
			if p.ContextSetupCount >= 2 {
				p.State = State5GRegistrationDone
			}
		}

	case State5GRegistrationDone:
		// The first UplinkNASTransport after InitialContextSetup completes
		// is the Registration Complete. This is the true terminal packet.
		if pkt.NGAPProcedureCode == NGAPProcUplinkNASTransport {
			p.UplinkAfterContextSetup++
			if p.UplinkAfterContextSetup >= 1 {
				if len(p.Spans) > 0 {
					p.Spans[len(p.Spans)-1].EndTime = pkt.Timestamp.Add(time.Millisecond)
				}
				return true
			}
		}
	}

	return false
}

// normaliseHex lowercases a hex string for consistent comparison.
// tshark outputs values like "0x41" or "0X41" — normalise to "0x41".
func normaliseHex(s string) string {
	if len(s) >= 2 && s[:2] == "0X" {
		return "0x" + s[2:]
	}
	return s
}
