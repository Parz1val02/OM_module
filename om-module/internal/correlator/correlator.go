package correlator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Parz1val02/OM_module/internal/capture"
	"github.com/Parz1val02/OM_module/internal/reconstructor"
	"github.com/Parz1val02/OM_module/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Correlator consumes packets from the capture manager, maintains
// per-UE procedure state machines, and emits distributed traces to Tempo
// when procedures complete or time out.
type Correlator struct {
	mcc     string
	mnc     string
	timeout time.Duration
	recCfg  reconstructor.Config

	mu         sync.Mutex
	procedures map[string]*Procedure // key → *Procedure
	// keyAlias maps temporary keys (ENB-ID, RAN-ID) to stable IMSI keys
	// after the IMSI becomes known.
	keyAlias map[string]string
}

// New creates a Correlator.
//
//   - mcc, mnc: used to reconstruct 5G IMSI from SUCI MSIN
//   - timeout:  how long to wait before flushing an incomplete procedure as timeout
//   - recCfg:   reconstructor config used for automatic fallback on timeout
func New(mcc, mnc string, timeout time.Duration, recCfg reconstructor.Config) *Correlator {
	return &Correlator{
		mcc:        mcc,
		mnc:        mnc,
		timeout:    timeout,
		recCfg:     recCfg,
		procedures: make(map[string]*Procedure),
		keyAlias:   make(map[string]string),
	}
}

// Run reads packets from the capture manager and processes them.
// It also runs a background ticker to flush timed-out procedures.
// Blocks until ctx is cancelled.
func (c *Correlator) Run(ctx context.Context, pkts <-chan capture.Packet) {
	log.Printf("🔗 Correlator started (timeout=%s)", c.timeout)

	ticker := time.NewTicker(c.timeout / 2)
	defer ticker.Stop()

	for {
		select {
		case pkt, ok := <-pkts:
			if !ok {
				// Channel closed — capture manager restarting.
				return
			}
			c.handlePacket(ctx, pkt)

		case <-ticker.C:
			c.flushTimedOut(ctx)

		case <-ctx.Done():
			log.Printf("🔗 Correlator stopped")
			return
		}
	}
}

// ActiveProcedureCount returns the current number of in-flight procedures.
func (c *Correlator) ActiveProcedureCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.procedures)
}

// handlePacket routes a packet to the appropriate procedure state machine.
func (c *Correlator) handlePacket(ctx context.Context, pkt capture.Packet) {
	switch pkt.Generation {
	case capture.Generation4G:
		c.handle4G(ctx, pkt)
	case capture.Generation5G:
		c.handle5G(ctx, pkt)
	}
}

// handle4G processes a 4G S1AP packet.
func (c *Correlator) handle4G(ctx context.Context, pkt capture.Packet) {
	// Ignore infrastructure messages that are not UE procedures.
	if pkt.S1APProcedureCode == S1APProcS1Setup ||
		pkt.S1APProcedureCode == S1APProcUECapabilityInfoInd {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	rawKey := correlationKey4G(&pkt)
	if rawKey == "" {
		return
	}

	// Resolve alias: if this key has been upgraded to an IMSI key, use that.
	key := c.resolveAlias(rawKey)

	proc, exists := c.procedures[key]

	if !exists {
		// Only create a new procedure on InitialUEMessage + Attach Request.
		if pkt.S1APProcedureCode != S1APProcInitialUEMessage ||
			normaliseHex(pkt.NASEMMType) != NASEMMAttachRequest {
			return
		}

		proc = newProcedure(
			capture.Generation4G,
			"4G EPC — UE Attach & Default Bearer Establishment",
			"", // IMSI unknown yet
			&pkt,
		)
		proc.State = StateAttaching
		c.procedures[key] = proc
		log.Printf("🔗 4G procedure started (key=%s)", stripGenPrefix(key))
		return
	}

	// If IMSI is now available and procedure has no IMSI yet, upgrade the key.
	if pkt.IMSI != "" && proc.IMSI == "" {
		proc.IMSI = pkt.IMSI
		newKey := upgradeKey4G(pkt.IMSI)
		c.procedures[newKey] = proc
		delete(c.procedures, key)
		c.keyAlias[rawKey] = newKey
		key = newKey
		log.Printf("🔗 4G key upgraded to IMSI (imsi=%s)", pkt.IMSI)
	}

	done := proc.advance4G(&pkt)
	if done {
		delete(c.procedures, key)
		log.Printf("✅ 4G attach complete (imsi=%s spans=%d)", proc.IMSI, len(proc.Spans))
		go c.emitTrace(ctx, proc, ResultSuccess)
	}
}

// handle5G processes a 5G NGAP packet.
func (c *Correlator) handle5G(ctx context.Context, pkt capture.Packet) {
	// Ignore infrastructure messages.
	if pkt.NGAPProcedureCode == NGAPProcNGSetup ||
		pkt.NGAPProcedureCode == NGAPProcUECapabilityInfoInd {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := correlationKey5G(&pkt)
	if key == "" {
		return
	}

	proc, exists := c.procedures[key]

	if !exists {
		// Only create on InitialUEMessage + Registration Request.
		if pkt.NGAPProcedureCode != NGAPProcInitialUEMessage ||
			normaliseHex(pkt.NASMMType) != NASMMRegistrationRequest {
			return
		}

		// Reconstruct IMSI from SUCI MSIN immediately.
		imsi := ""
		if pkt.SUCIMsin != "" {
			imsi = reconstructIMSI(c.mcc, c.mnc, pkt.SUCIMsin)
		}

		proc = newProcedure(
			capture.Generation5G,
			"5G NR — UE Registration & PDU Session Establishment",
			imsi,
			&pkt,
		)
		proc.State = StateRegistering
		c.procedures[key] = proc
		log.Printf("🔗 5G procedure started (key=%s imsi=%s)", stripGenPrefix(key), imsi)
		return
	}

	done := proc.advance5G(&pkt)
	if done {
		delete(c.procedures, key)
		log.Printf("✅ 5G registration complete (imsi=%s spans=%d)", proc.IMSI, len(proc.Spans))
		go c.emitTrace(ctx, proc, ResultSuccess)
	}
}

// flushTimedOut finds procedures that have not received a packet within the
// timeout window and emits them as incomplete traces, then triggers the
// log-based reconstructor as a fallback.
func (c *Correlator) flushTimedOut(ctx context.Context) {
	now := time.Now()
	c.mu.Lock()
	var timedOut []*Procedure
	for key, proc := range c.procedures {
		if now.Sub(proc.LastSeen) > c.timeout {
			timedOut = append(timedOut, proc)
			delete(c.procedures, key)
		}
	}
	c.mu.Unlock()

	for _, proc := range timedOut {
		log.Printf("⏱  Procedure timeout (imsi=%s gen=%s state=%d) — flushing as incomplete",
			proc.IMSI, proc.Generation, proc.State)
		go c.emitTrace(ctx, proc, ResultTimeout)

		// Automatically trigger the log-based reconstructor as fallback
		// if we have an IMSI to query with.
		if proc.IMSI != "" {
			go c.triggerReconstructor(ctx, proc)
		}
	}
}

// triggerReconstructor calls the log-based reconstructor automatically when
// the packet-based correlator times out an incomplete procedure.
func (c *Correlator) triggerReconstructor(ctx context.Context, proc *Procedure) {
	log.Printf("🔄 Auto-triggering reconstructor for IMSI=%s gen=%s", proc.IMSI, proc.Generation)
	result, err := reconstructor.Reconstruct(ctx, c.recCfg, proc.IMSI, proc.Generation)
	if err != nil {
		log.Printf("⚠️  Reconstructor fallback failed (imsi=%s): %v", proc.IMSI, err)
		return
	}
	log.Printf("✅ Reconstructor fallback succeeded (imsi=%s traceID=%s)", proc.IMSI, result.TraceID)
}

// emitTrace builds and exports an OpenTelemetry trace from a completed or
// timed-out procedure. Each SpanRecord becomes a child span under a root span.
func (c *Correlator) emitTrace(ctx context.Context, proc *Procedure, result ProcedureResult) {
	if len(proc.Spans) == 0 {
		return
	}

	tracer := tracing.Tracer()
	traceStart := proc.StartTime
	traceEnd := proc.LastSeen
	if !traceEnd.After(traceStart) {
		traceEnd = traceStart.Add(time.Millisecond)
	}

	rootAttrs := []attribute.KeyValue{
		attribute.String("imsi", proc.IMSI),
		attribute.String("generation", proc.Generation),
		attribute.String("procedure", proc.ProcedureType),
		attribute.String("result", string(result)),
		attribute.String("source", "capture"),
		attribute.Int("span_count", len(proc.Spans)),
	}

	rootCtx, rootSpan := tracer.Start(ctx, proc.ProcedureType,
		oteltrace.WithTimestamp(traceStart),
		oteltrace.WithAttributes(rootAttrs...),
	)

	for _, sr := range proc.Spans {
		end := sr.EndTime
		if end.IsZero() || !end.After(sr.StartTime) {
			end = sr.StartTime.Add(time.Millisecond)
		}

		_, childSpan := tracer.Start(rootCtx, sr.Name,
			oteltrace.WithTimestamp(sr.StartTime),
			oteltrace.WithAttributes(
				attribute.String("imsi", proc.IMSI),
				attribute.String("generation", proc.Generation),
				attribute.String("source", "capture"),
				attribute.String("src_ip", sr.SrcIP),
				attribute.String("dst_ip", sr.DstIP),
			),
		)

		if result == ResultTimeout {
			childSpan.SetStatus(codes.Error, "procedure timed out")
		}

		childSpan.End(oteltrace.WithTimestamp(end))
	}

	if result != ResultSuccess {
		rootSpan.SetStatus(codes.Error, fmt.Sprintf("procedure %s", result))
	}
	rootSpan.End(oteltrace.WithTimestamp(traceEnd))

	log.Printf("📤 Trace emitted to Tempo (imsi=%s result=%s spans=%d traceID=%s)",
		proc.IMSI, result, len(proc.Spans)+1,
		rootSpan.SpanContext().TraceID().String())
}

// resolveAlias follows the alias chain to find the current canonical key.
func (c *Correlator) resolveAlias(key string) string {
	seen := make(map[string]bool)
	for {
		if seen[key] {
			break
		}
		seen[key] = true
		if alias, ok := c.keyAlias[key]; ok {
			key = alias
		} else {
			break
		}
	}
	return key
}

// ProcedureSummary is a lightweight snapshot of an in-flight procedure,
// used by the API status endpoint.
type ProcedureSummary struct {
	IMSI          string
	Generation    string
	ProcedureType string
	State         ProcedureState
	SpanCount     int
	AgeSeconds    float64
}

// ActiveProcedures returns summaries of all currently in-flight procedures.
func (c *Correlator) ActiveProcedures() []ProcedureSummary {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	summaries := make([]ProcedureSummary, 0, len(c.procedures))
	seen := make(map[*Procedure]bool)

	for _, proc := range c.procedures {
		if seen[proc] {
			continue
		}
		seen[proc] = true
		summaries = append(summaries, ProcedureSummary{
			IMSI:          proc.IMSI,
			Generation:    proc.Generation,
			ProcedureType: proc.ProcedureType,
			State:         proc.State,
			SpanCount:     len(proc.Spans),
			AgeSeconds:    now.Sub(proc.StartTime).Seconds(),
		})
	}
	return summaries
}

// stripGenPrefix removes the "4g:" or "5g:" prefix from a key for logging.
func stripGenPrefix(key string) string {
	if idx := strings.Index(key, ":"); idx >= 0 {
		return key[idx+1:]
	}
	return key
}
