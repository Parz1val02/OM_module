// Package reconstructor builds synthetic distributed traces from Open5GS
// log lines stored in Loki. Supports both 5G (NR) and 4G (EPC) procedures.
package reconstructor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Parz1val02/OM_module/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ─── Configuration ────────────────────────────────────────────────────────────

type Config struct {
	LokiURL     string
	QueryWindow time.Duration
}

func DefaultConfig() Config {
	return Config{
		LokiURL:     "http://loki:3100",
		QueryWindow: 10 * time.Minute,
	}
}

// ─── Log event ────────────────────────────────────────────────────────────────

type logEvent struct {
	Timestamp  time.Time
	NF         string
	Generation string
	Level      string
	Message    string
	Step       string
	IsError    bool
}

// ─── Skip patterns ────────────────────────────────────────────────────────────
//
// Lines matching any of these are dropped before step-mapping.
// Covers SBI/GTP plumbing, NRF lifecycle noise, teardown lines, hex dumps,
// and indented detail lines that carry no procedure-level information.

var skipPatterns = []string{
	// SBI endpoint plumbing
	"Setup NF EndPoint",
	"UnRef NF EndPoint",
	// NRF lifecycle (heartbeat, registration, subscriptions)
	"NF de-registered",
	"No heartbeat",
	"NF registered [Heartbeat",
	"NRF-notify",
	"NRF-profile-get",
	"Subscription created",
	"NF registered\n",
	// SBI infrastructure
	"nghttp2_server",
	"NF Service [",
	"Stream error",
	"client_notify_cb",
	// PFCP / GTP plumbing
	"pfcp_server",
	"gtp_server",
	"gtp_connect",
	"s1ap_server",
	"PFCP associated",
	"PFCP[RSP]",
	"PFCP[REQ]",
	"PFCP de-associated",
	"No UPF available",
	"No Heartbeat from UPF",
	"No Reponse. Give up",
	// Startup / config
	"Open5GS daemon",
	"File Logging",
	"Configuration:",
	"initialize...done",
	"Polling freeDiameter",
	"MongoDB URI",
	"metrics_server",
	// Diameter
	"CONNECTED TO",
	// Timezone detail lines (indented)
	"Timezone",
	"LOCAL [",
	"UTC [",
	// 5G V-SMF discovery noise
	"V-SMF Instance",
	"V-SMF discovered",
	"nsmf_pdusession [",
	// NGAP/S1AP detail lines (indented)
	"ENB_UE_S1AP_ID",
	"RAN_UE_NGAP_ID",
	"UE Context Release",
	"SUCI[suci",
	// UPF hex dumps
	"0000: ",
	"0010: ",
	"0020: ",
	// eNB/gNB connection lines
	"eNB-S1 accepted",
	"eNB-S1[",
	"gNB-N2 accepted",
	"gNB-N2[",
	"max_num_of_ostreams",
	// gNB add (not procedure step — just RAN connection)
	"Number of gNBs is now 1",
	"Number of eNBs is now 1",
	// Teardown / [Removed] lines
	"[Removed]",
	"Number of eNBs is now 0",
	"Number of gNBs is now 0",
	"Number of eNB-UEs is now 0",
	"Number of gNB-UEs is now 0",
	"Number of MME-UEs is now 0",
	"Number of MME-Sessions is now 0",
	"Number of SMF-UEs is now 0",
	"Number of SMF-Sessions is now 0",
	"Number of SGWC-UEs is now 0",
	"Number of SGWC-Sessions is now 0",
	"Number of SGWU-Sessions is now 0",
	"Number of UPF-Sessions is now 0",
	"Number of UPF-sessions is now 0",
	// gNB disconnects (not a UE procedure step)
	"connection refused",
	// Indented detail lines — carry no new procedure information
	"emm-handler.c:492",   // "    IMSI[001011...] (emm-handler.c:492)"
	"emm-handler.c:296",   // "    IMSI[001011...] (emm-handler.c:296)"
	"s1ap-handler.c:2148", // "    IMSI[...] (s1ap-handler.c:2148)"
	"s1ap-handler.c:2143", // "    ENB_UE_S1AP_ID[...] (s1ap-handler.c:2143)"
	"emm-handler.c:256",   // "    GUTI[...] IMSI[...] (emm-handler.c:256)"
}

func shouldSkip(msg string) bool {
	for _, s := range skipPatterns {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// ─── Step patterns ────────────────────────────────────────────────────────────
//
// Derived from real Open5GS v2.7.x log output for 5G NR and 4G EPC.
// Order matters — first match wins.
// Patterns use source-file suffixes (e.g. "s5c-handler.c", "s11-handler.c")
// where necessary to disambiguate lines that share common substrings.

var stepPatterns = []struct {
	substr string
	step   string
	isErr  bool
}{
	// ════════════════════════════════════════════════════════
	// 5G NR — Registration & PDU Session
	// ════════════════════════════════════════════════════════

	// [amf] INFO: InitialUEMessage (ngap-handler.c:461)
	{"InitialUEMessage", "RAN → AMF/MME: Initial UE Message", false},

	// [amf] INFO: [Added] Number of gNB-UEs is now 1
	{"Number of gNB-UEs is now 1", "AMF: gNB-UE context created", false},

	// [amf] INFO: [suci-...] known UE by SUCI
	{"known UE by SUCI", "AMF: UE identified by SUCI (known UE)", false},

	// [amf] INFO: [suci-...] Unknown UE by SUCI
	{"Unknown UE by SUCI", "AMF: New UE — SUCI unknown (first registration)", false},

	// [gmm] INFO: Registration request
	{"Registration request", "AMF: NAS Registration Request received", false},

	// [gmm] INFO: [suci-...] SUCI  (gmm-handler.c:183)
	{"SUCI", "AMF: SUCI identity confirmed", false},

	// [ausf] INFO: Setup NF EndPoint [...] (nudm-handler.c) — skipped by Setup NF EndPoint
	// Caught via nausf-handler on AMF side instead:
	{"nausf-handler", "AMF → AUSF: Authentication request (Nausf)", false},

	// [amf] INFO: [...] (npcf-handler.c:143)
	{"npcf-handler.c:143)", "AMF → PCF: AM Policy Association request (N15)", false},

	// [amf] INFO: [Added] Number of AMF-UEs is now 1
	{"Number of AMF-UEs is now 1", "AMF: UE context created", false},

	// [amf] INFO: [Added] Number of AMF-Sessions is now 1
	{"Number of AMF-Sessions is now 1", "AMF: NAS Session context created", false},

	// [gmm] INFO: [imsi-...] Registration complete
	{"Registration complete", "AMF: Registration Complete ✓", false},

	// [amf] INFO: [imsi-...] Configuration update command
	{"Configuration update command", "AMF: Configuration Update Command sent", false},

	// [gmm] INFO: UE SUPI[imsi-...] DNN[internet] LBO[0] ... (gmm-handler.c:1416)
	{"gmm-handler.c:1416)", "AMF → SMF: PDU Session Establishment Request (N11)", false},

	// [gmm] INFO: [imsi-...] No GUTI allocated
	{"No GUTI allocated", "AMF: Registration finalised (no GUTI re-allocated)", false},

	// [amf] INFO: [imsi-...:1:11] /nsmf-pdusession/v1/sm-contexts/.../modify
	{"nsmf-pdusession/v1/sm-contexts", "AMF ↔ SMF: PDU Session context modify (N11)", false},

	// [amf] INFO: [imsi-...] Release SM context [204]
	{"Release SM context", "AMF: SM Context release initiated", false},

	// [amf] INFO: [imsi-...] Release SM Context [state:31]
	{"Release SM Context", "AMF: SM Context released", false},

	// ── 5G SMF ───────────────────────────────────────────
	// [smf] INFO: [Added] Number of SMF-UEs is now 1
	{"Number of SMF-UEs is now 1", "SMF: UE context created", false},

	// [smf] INFO: [Added] Number of SMF-Sessions is now 1
	{"Number of SMF-Sessions is now 1", "SMF: PDU/Bearer session context created", false},

	// [smf] INFO: [...] (nudm-handler.c:456)
	{"nudm-handler.c:456)", "SMF → UDM: Session subscription data lookup (N10)", false},

	// [smf] INFO: [...] (npcf-handler.c:373)
	{"npcf-handler.c:373)", "SMF → PCF: SM Policy Association request (N7)", false},

	// [smf] INFO: UE SUPI[imsi-...] DNN[internet] IPv4[...] (npcf-handler.c:594)
	// Use source file to distinguish from 4G "UE IMSI[...]"
	{"npcf-handler.c:594)", "SMF: PDU Session established — IP address allocated", false},

	// [smf/mme] INFO: Removed Session: UE IMSI:[imsi-...] — previous session lifecycle teardown
	{"Removed Session:", "SMF/MME: Previous session removed (lifecycle teardown)", false},

	// ── 5G PCF ───────────────────────────────────────────
	// [pcf] INFO: [...] (npcf-handler.c:114)
	{"npcf-handler.c:114)", "PCF: AM Policy Association response (N15)", false},

	// [pcf] INFO: [...] (npcf-handler.c:448)
	{"npcf-handler.c:448)", "PCF: SM Policy Association response (N7)", false},

	// ── 5G UPF (shared with 4G) ──────────────────────────
	// [upf] INFO: [Added] Number of UPF-Sessions is now 1
	{"Number of UPF-Sessions is now 1", "UPF: PFCP session context created", false},

	// [upf] INFO: UE F-SEID[UP:0x... CP:0x...] APN[internet] ...
	{"F-SEID", "UPF: F-SEID allocated — data plane ready", false},

	// ── 5G Idle ──────────────────────────────────────────
	// [gmm] WARNING: [imsi-...] Mobile Reachable Timer Expired
	{"Mobile Reachable Timer Expired", "AMF: UE entered idle mode (Mobile Reachable Timer expired)", false},

	// ════════════════════════════════════════════════════════
	// 4G EPC — Attach & Default Bearer Setup
	// ════════════════════════════════════════════════════════

	// [mme] INFO: S_TMSI[G:2,C:1,M_TMSI:0xc...] IMSI:[001011...] (s1ap-handler.c:597)
	// This is the first line where MME logs the IMSI — UE identified via S_TMSI lookup
	{"s1ap-handler.c:597)", "MME: UE identified — S_TMSI resolved to IMSI", false},

	// [mme] INFO: [Added] Number of eNB-UEs is now 1
	{"Number of eNB-UEs is now 1", "MME: eNB-UE context created", false},

	// [mme] INFO: Unknown UE by GUTI[...]
	{"Unknown UE by GUTI", "MME: UE unknown by GUTI — Identity Request triggered", false},

	// [mme] INFO: Unknown UE by S_TMSI[...]
	{"Unknown UE by S_TMSI", "MME: UE unknown by S_TMSI", false},

	// [mme] INFO: [Added] Number of MME-UEs is now 1
	{"Number of MME-UEs is now 1", "MME: UE context created", false},

	// [emm] INFO: [] Attach request  (emm-sm.c:469)
	{"Attach request", "MME: NAS Attach Request received (EMM)", false},

	// [emm] INFO: Identity response  (emm-sm.c:439)
	{"Identity response", "MME: Identity Response — IMSI confirmed", false},

	// ── 4G SGW-C ─────────────────────────────────────────
	// [sgwc] INFO: [Added] Number of SGWC-UEs is now 1
	{"Number of SGWC-UEs is now 1", "SGW-C: UE context created", false},

	// [sgwc] INFO: [Added] Number of SGWC-Sessions is now 1
	{"Number of SGWC-Sessions is now 1", "SGW-C: Bearer session context created", false},

	// [sgwc] INFO: UE IMSI[001011...] APN[internet] (s11-handler.c:268)
	// Use source file to distinguish sgwc from smf
	{"s11-handler.c", "SGW-C → SMF/PGW-C: Create Session Request (S11/GTPv2)", false},

	// ── 4G SMF/PGW-C ─────────────────────────────────────
	// [smf] INFO: UE IMSI[001011...] APN[internet] IPv4[...] (s5c-handler.c:311)
	// Use source file to distinguish from 5G "UE SUPI[...]"
	{"s5c-handler.c", "SMF/PGW-C: EPS Bearer created — IP address allocated", false},

	// ── 4G SGW-U ─────────────────────────────────────────
	// [sgwu] INFO: [Added] Number of SGWU-Sessions is now 1
	{"Number of SGWU-Sessions is now 1", "SGW-U: PFCP session context created", false},

	// ── 4G MME Session + Attach Complete ─────────────────
	// [mme] INFO: [Added] Number of MME-Sessions is now 1
	{"Number of MME-Sessions is now 1", "MME: Session context created (default bearer established)", false},

	// [emm] INFO: [001011...] Attach complete  (emm-sm.c:1573)
	{"Attach complete", "MME: EPS Attach Complete ✓", false},

	// ── 4G Idle ──────────────────────────────────────────
	// [mme] INFO: Mobile Reachable timer started for IMSI[...]
	{"Mobile Reachable timer started", "MME: UE released to idle — Mobile Reachable Timer started", false},

	// [emm] INFO: [001011...] Mobile Reachable timer expired
	{"Mobile Reachable timer expired", "MME: UE idle timeout — Mobile Reachable Timer expired", false},

	// [emm] INFO: [001011...] Detach request
	{"Detach request", "MME: UE Detach Request received", false},

	// ════════════════════════════════════════════════════════
	// Universal — Errors (always last)
	// ════════════════════════════════════════════════════════
	{"ERROR", "Error detected", true},
}

func mapStep(msg string) (step string, isErr bool) {
	for _, p := range stepPatterns {
		if strings.Contains(msg, p.substr) {
			return p.step, p.isErr
		}
	}
	return "", false
}

// ─── Loki query ───────────────────────────────────────────────────────────────

type lokiQueryResult struct {
	Data struct {
		Result []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func queryLoki(ctx context.Context, cfg Config, imsi string, start, end time.Time) ([]logEvent, error) {
	expr := fmt.Sprintf(`{domain=~"core|epc"} |= "%s"`, imsi)

	params := url.Values{}
	params.Set("query", expr)
	params.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	params.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	params.Set("limit", "500")
	params.Set("direction", "forward")

	reqURL := fmt.Sprintf("%s/loki/api/v1/query_range?%s", cfg.LokiURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("loki: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("loki: HTTP request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("⚠️  queryLoki: failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("loki: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result lokiQueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("loki: decode response: %w", err)
	}

	var events []logEvent
	for _, stream := range result.Data.Result {
		nf := stream.Stream["nf"]
		generation := stream.Stream["generation"]
		level := stream.Stream["level"]

		if generation == "" {
			generation = inferGeneration(nf)
		}

		for _, val := range stream.Values {
			if len(val) < 2 {
				continue
			}
			nsec, err := strconv.ParseInt(val[0], 10, 64)
			if err != nil {
				continue
			}
			ts := time.Unix(0, nsec)
			msg := val[1]

			if shouldSkip(msg) {
				continue
			}

			step, isErr := mapStep(msg)
			if step == "" {
				continue
			}

			events = append(events, logEvent{
				Timestamp:  ts,
				NF:         nf,
				Generation: generation,
				Level:      level,
				Message:    msg,
				Step:       step,
				IsError:    isErr,
			})
		}
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	events = dedup(events)

	return events, nil
}

// inferGeneration maps NF names to 4g/5g when the Loki label is absent.
func inferGeneration(nf string) string {
	switch nf {
	case "mme", "sgwc", "sgwu", "hss", "pcrf":
		return "4g"
	case "amf", "smf", "upf", "ausf", "udm", "udr", "pcf", "nrf", "scp", "bsf", "nssf":
		return "5g"
	default:
		return ""
	}
}

// dedup removes consecutive (NF, Step) duplicates within 2 seconds.
// Handles cases like UPF F-SEID being logged twice in the same attach.
func dedup(events []logEvent) []logEvent {
	if len(events) == 0 {
		return events
	}
	out := []logEvent{events[0]}
	for i := 1; i < len(events); i++ {
		prev := out[len(out)-1]
		cur := events[i]
		if cur.NF == prev.NF && cur.Step == prev.Step &&
			cur.Timestamp.Sub(prev.Timestamp) < 2*time.Second {
			continue
		}
		out = append(out, cur)
	}
	return out
}

// ─── Trace emission ───────────────────────────────────────────────────────────

type Result struct {
	TraceID    string         `json:"trace_id"`
	IMSI       string         `json:"imsi"`
	Generation string         `json:"generation"`
	Procedure  string         `json:"procedure"`
	Start      time.Time      `json:"start"`
	End        time.Time      `json:"end"`
	SpanCount  int            `json:"span_count"`
	Events     []EventSummary `json:"events"`
}

type EventSummary struct {
	NF        string    `json:"nf"`
	Step      string    `json:"step"`
	Timestamp time.Time `json:"timestamp"`
	IsError   bool      `json:"is_error"`
}

func Reconstruct(ctx context.Context, cfg Config, imsi, generation string) (*Result, error) {
	end := time.Now()
	start := end.Add(-cfg.QueryWindow)

	log.Printf("🔍 Reconstructing trace for IMSI=%s generation=%s window=[%s → %s]",
		imsi, generation, start.Format(time.RFC3339), end.Format(time.RFC3339))

	events, err := queryLoki(ctx, cfg, imsi, start, end)
	if err != nil {
		return nil, fmt.Errorf("reconstruct: query loki: %w", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("reconstruct: no log events found for IMSI %s in the last %s",
			imsi, cfg.QueryWindow)
	}

	if generation != "" {
		var filtered []logEvent
		for _, e := range events {
			if e.Generation == generation {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("reconstruct: no %s events found for IMSI %s", generation, imsi)
		}
		events = filtered
	}

	if generation == "" && len(events) > 0 {
		generation = events[0].Generation
	}

	procedure := procedureName(generation, events)

	traceStart := events[0].Timestamp
	traceEnd := events[len(events)-1].Timestamp
	if !traceEnd.After(traceStart) {
		traceEnd = traceStart.Add(time.Millisecond)
	}

	tracer := tracing.Tracer()

	rootCtx, rootSpan := tracer.Start(ctx, procedure,
		oteltrace.WithTimestamp(traceStart),
		oteltrace.WithAttributes(
			attribute.String("imsi", imsi),
			attribute.String("generation", generation),
			attribute.String("procedure", procedure),
			attribute.Int("log_events", len(events)),
		),
	)

	var summaries []EventSummary
	hasError := false

	for i, ev := range events {
		spanName := fmt.Sprintf("%s: %s", strings.ToUpper(ev.NF), ev.Step)

		spanDur := 5 * time.Millisecond
		if i+1 < len(events) {
			gap := events[i+1].Timestamp.Sub(ev.Timestamp)
			if gap > 0 && gap < 30*time.Second {
				spanDur = gap
			}
		}
		spanEnd := ev.Timestamp.Add(spanDur)

		_, childSpan := tracer.Start(rootCtx, spanName,
			oteltrace.WithTimestamp(ev.Timestamp),
			oteltrace.WithAttributes(
				attribute.String("nf", ev.NF),
				attribute.String("generation", ev.Generation),
				attribute.String("step", ev.Step),
				attribute.String("log.message", truncate(ev.Message, 256)),
				attribute.String("imsi", imsi),
				attribute.String("log.level", ev.Level),
			),
		)

		if ev.IsError {
			childSpan.SetStatus(codes.Error, ev.Message)
			hasError = true
		}

		childSpan.End(oteltrace.WithTimestamp(spanEnd))

		summaries = append(summaries, EventSummary{
			NF:        ev.NF,
			Step:      ev.Step,
			Timestamp: ev.Timestamp,
			IsError:   ev.IsError,
		})
	}

	if hasError {
		rootSpan.SetStatus(codes.Error, "procedure contained errors")
	}
	rootSpan.End(oteltrace.WithTimestamp(traceEnd))

	traceID := rootSpan.SpanContext().TraceID().String()
	log.Printf("✅ Trace emitted to Tempo: traceID=%s spans=%d procedure=%q",
		traceID, len(events)+1, procedure)

	return &Result{
		TraceID:    traceID,
		IMSI:       imsi,
		Generation: generation,
		Procedure:  procedure,
		Start:      traceStart,
		End:        traceEnd,
		SpanCount:  len(events) + 1,
		Events:     summaries,
	}, nil
}

func procedureName(generation string, events []logEvent) string {
	nfSet := map[string]bool{}
	for _, e := range events {
		nfSet[e.NF] = true
	}
	switch generation {
	case "5g":
		if nfSet["amf"] && nfSet["smf"] && nfSet["upf"] {
			return "5G NR — UE Registration & PDU Session Establishment"
		}
		if nfSet["amf"] && nfSet["smf"] {
			return "5G NR — UE Registration & SMF Session"
		}
		if nfSet["amf"] {
			return "5G NR — UE Registration"
		}
		return "5G NR Core Procedure"
	case "4g":
		if nfSet["mme"] && nfSet["smf"] && nfSet["upf"] {
			return "4G EPC — UE Attach & Default Bearer Establishment"
		}
		if nfSet["mme"] && nfSet["sgwc"] {
			return "4G EPC — UE Attach & Bearer Setup"
		}
		if nfSet["mme"] {
			return "4G EPC — UE Attach"
		}
		return "4G EPC Core Procedure"
	default:
		return "Core Network Procedure"
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
