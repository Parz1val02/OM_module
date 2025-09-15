package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
	"github.com/fsnotify/fsnotify"
)

// LogParser handles real-time log parsing and forwarding to Loki
type LogParser struct {
	topology   *discovery.NetworkTopology
	lokiURL    string
	watchers   map[string]*fsnotify.Watcher
	parsers    map[string]*ComponentLogParser
	httpClient *http.Client
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex

	// Educational context
	educationalMode bool
	protocolRegexes map[string]*regexp.Regexp
}

// ComponentLogParser handles parsing for specific network function types
type ComponentLogParser struct {
	ComponentName string
	ComponentType string
	LogFormat     string // "open5gs", "srsran", "srslte"
	LogPatterns   map[string]*regexp.Regexp
}

// StructuredLogEntry represents a parsed and enriched log entry
type StructuredLogEntry struct {
	Timestamp   time.Time           `json:"timestamp"`
	Level       string              `json:"level"`
	Component   string              `json:"component"`
	Message     string              `json:"message"`
	RawMessage  string              `json:"raw_message"`
	Labels      map[string]string   `json:"labels"`
	Educational *EducationalContext `json:"educational,omitempty"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
}

// EducationalContext provides learning context for students
type EducationalContext struct {
	ProtocolLayer string `json:"protocol_layer,omitempty"`
	Procedure     string `json:"procedure,omitempty"`
	Specification string `json:"specification,omitempty"`
	StudentNote   string `json:"student_note,omitempty"`
	FlowStep      int    `json:"flow_step,omitempty"`
}

// LokiStreamEntry represents a log entry in Loki format
type LokiStreamEntry struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// LokiPushRequest represents the request format for Loki push API
type LokiPushRequest struct {
	Streams []LokiStreamEntry `json:"streams"`
}

// NewLogParser creates a new log parser instance
func NewLogParser(topology *discovery.NetworkTopology, lokiURL string, educationalMode bool) *LogParser {
	ctx, cancel := context.WithCancel(context.Background())

	lp := &LogParser{
		topology:        topology,
		lokiURL:         lokiURL,
		watchers:        make(map[string]*fsnotify.Watcher),
		parsers:         make(map[string]*ComponentLogParser),
		educationalMode: educationalMode,
		ctx:             ctx,
		cancel:          cancel,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		protocolRegexes: initializeProtocolRegexes(),
	}

	return lp
}

// Start begins the log parsing service
func (lp *LogParser) Start() error {
	log.Printf("🚀 Starting Log Parser Service...")

	// Initialize component parsers based on topology
	if err := lp.initializeComponentParsers(); err != nil {
		return fmt.Errorf("failed to initialize component parsers: %w", err)
	}

	// Start HTTP server for log parser endpoints
	go lp.startHTTPServer()

	// Start file watchers for each component
	if err := lp.startFileWatchers(); err != nil {
		return fmt.Errorf("failed to start file watchers: %w", err)
	}

	log.Printf("✅ Log Parser Service started successfully")
	return nil
}

// initializeComponentParsers creates parsers for each discovered component
func (lp *LogParser) initializeComponentParsers() error {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	for name, component := range lp.topology.Components {
		parser := &ComponentLogParser{
			ComponentName: name,
			ComponentType: component.Type,
			LogFormat:     determineLogFormat(component.Type),
			LogPatterns:   createLogPatterns(component.Type),
		}

		lp.parsers[name] = parser
		log.Printf("📝 Initialized parser for %s (%s)", name, component.Type)
	}

	return nil
}

// startFileWatchers creates file system watchers for log directories
func (lp *LogParser) startFileWatchers() error {
	logDirectories := map[string]string{
		"open5gs": "/var/log/open5gs",
		"srslte":  "/var/log/srslte",
		"srsran":  "/var/log/srsran",
	}

	for source, dir := range logDirectories {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			log.Printf("⚠️ Log directory %s does not exist, skipping", dir)
			continue
		}

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("failed to create watcher for %s: %w", dir, err)
		}

		lp.watchers[source] = watcher

		// Watch the directory
		if err := watcher.Add(dir); err != nil {
			return fmt.Errorf("failed to watch directory %s: %w", dir, err)
		}

		// Start processing events for this watcher
		go lp.processWatcherEvents(source, watcher)

		log.Printf("👁️ Started watching %s", dir)
	}

	return nil
}

// processWatcherEvents handles file system events and processes log files
func (lp *LogParser) processWatcherEvents(source string, watcher *fsnotify.Watcher) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				go lp.processLogFile(source, event.Name)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("⚠️ Watcher error for %s: %v", source, err)

		case <-lp.ctx.Done():
			return
		}
	}
}

// processLogFile reads and parses a log file
func (lp *LogParser) processLogFile(source, filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("⚠️ Failed to open log file %s: %v", filePath, err)
		return
	}
	defer func() {
		err = file.Close()
	}()

	// Determine component name from file path
	componentName := extractComponentFromPath(filePath)
	if componentName == "" {
		log.Printf("⚠️ Could not determine component from path: %s", filePath)
		return
	}

	lp.mu.RLock()
	parser, exists := lp.parsers[componentName]
	lp.mu.RUnlock()

	if !exists {
		log.Printf("⚠️ No parser found for component: %s", componentName)
		return
	}

	// Read and parse log entries
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		entry := lp.parseLogLine(parser, line)
		if entry != nil {
			lp.forwardToLoki(entry)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("⚠️ Error reading log file %s: %v", filePath, err)
	}
}

// parseLogLine converts a raw log line into a structured entry
func (lp *LogParser) parseLogLine(parser *ComponentLogParser, line string) *StructuredLogEntry {
	entry := &StructuredLogEntry{
		Timestamp:  time.Now(),
		Component:  parser.ComponentName,
		RawMessage: line,
		Labels: map[string]string{
			"component":      parser.ComponentName,
			"component_type": parser.ComponentType,
			"log_format":     parser.LogFormat,
		},
		Metadata: make(map[string]any),
	}

	// Parse based on log format
	switch parser.LogFormat {
	case "open5gs":
		lp.parseOpen5GSLog(entry, line)
	case "srsran":
		lp.parseSRSRANLog(entry, line)
	case "srslte":
		lp.parseSRSLTELog(entry, line)
	default:
		entry.Message = line
		entry.Level = "unknown"
	}

	// Add educational context if enabled
	if lp.educationalMode {
		entry.Educational = lp.extractEducationalContext(entry)
	}

	return entry
}

// parseOpen5GSLog parses Open5GS log format
func (lp *LogParser) parseOpen5GSLog(entry *StructuredLogEntry, line string) {
	// Open5GS log format: YYYY/MM/DD HH:MM:SS.sss [LEVEL] component: message
	open5gsPattern := regexp.MustCompile(`(\d{4}/\d{2}/\d{2}\s\d{2}:\d{2}:\d{2}\.\d{3})\s\[(\w+)\]\s(\w+):\s(.+)`)

	matches := open5gsPattern.FindStringSubmatch(line)
	if len(matches) >= 5 {
		// Parse timestamp
		if ts, err := time.Parse("2006/01/02 15:04:05.000", matches[1]); err == nil {
			entry.Timestamp = ts
		}

		entry.Level = strings.ToLower(matches[2])
		entry.Message = matches[4]

		// Extract additional metadata
		lp.extractOpen5GSMetadata(entry, matches[4])
	} else {
		entry.Message = line
		entry.Level = "info"
	}
}

// parseSRSRANLog parses srsRAN log format
func (lp *LogParser) parseSRSRANLog(entry *StructuredLogEntry, line string) {
	// srsRAN format: timestamp [LEVEL] [LAYER] message
	srsranPattern := regexp.MustCompile(`(\d{2}:\d{2}:\d{2}\.\d{3})\s\[(\w+)\]\s\[(\w+)\]\s(.+)`)

	matches := srsranPattern.FindStringSubmatch(line)
	if len(matches) >= 5 {
		entry.Level = strings.ToLower(matches[2])
		entry.Labels["protocol_layer"] = matches[3]
		entry.Message = matches[4]

		// Extract protocol-specific metadata
		lp.extractSRSRANMetadata(entry, matches[3], matches[4])
	} else {
		entry.Message = line
		entry.Level = "info"
	}
}

// parseSRSLTELog parses srsLTE log format (similar to srsRAN)
func (lp *LogParser) parseSRSLTELog(entry *StructuredLogEntry, line string) {
	// Similar to srsRAN but might have slightly different format
	lp.parseSRSRANLog(entry, line) // Reuse srsRAN parser for now
}

// extractOpen5GSMetadata extracts metadata from Open5GS log messages
func (lp *LogParser) extractOpen5GSMetadata(entry *StructuredLogEntry, message string) {
	// Extract IMSI
	if imsiMatch := regexp.MustCompile(`imsi[:\s]*(\d{15})`).FindStringSubmatch(message); len(imsiMatch) > 1 {
		entry.Metadata["imsi"] = imsiMatch[1]
		entry.Labels["imsi"] = imsiMatch[1]
	}

	// Extract session information
	if sessionMatch := regexp.MustCompile(`session[:\s]*(\d+)`).FindStringSubmatch(message); len(sessionMatch) > 1 {
		entry.Metadata["session_id"] = sessionMatch[1]
	}

	// Extract procedure names
	procedures := []string{"attach", "detach", "handover", "paging", "authentication", "registration"}
	for _, proc := range procedures {
		if strings.Contains(strings.ToLower(message), proc) {
			entry.Labels["procedure"] = proc
			break
		}
	}
}

// extractSRSRANMetadata extracts metadata from srsRAN log messages
func (lp *LogParser) extractSRSRANMetadata(entry *StructuredLogEntry, layer, message string) {
	// Extract RSRP/RSRQ values
	if rsrpMatch := regexp.MustCompile(`RSRP=(-?\d+\.?\d*)`).FindStringSubmatch(message); len(rsrpMatch) > 1 {
		entry.Metadata["rsrp"] = rsrpMatch[1]
	}

	if rsrqMatch := regexp.MustCompile(`RSRQ=(-?\d+\.?\d*)`).FindStringSubmatch(message); len(rsrqMatch) > 1 {
		entry.Metadata["rsrq"] = rsrqMatch[1]
	}

	// Extract cell information
	if cellMatch := regexp.MustCompile(`cell[:\s]*(\d+)`).FindStringSubmatch(message); len(cellMatch) > 1 {
		entry.Metadata["cell_id"] = cellMatch[1]
	}
}

// extractEducationalContext adds educational metadata for student learning
func (lp *LogParser) extractEducationalContext(entry *StructuredLogEntry) *EducationalContext {
	ctx := &EducationalContext{}

	// Determine protocol layer
	if layer, exists := entry.Labels["protocol_layer"]; exists {
		ctx.ProtocolLayer = layer
		ctx.Specification = getSpecificationForLayer(layer)
	}

	// Determine procedure and add educational notes
	if proc, exists := entry.Labels["procedure"]; exists {
		ctx.Procedure = proc
		ctx.StudentNote = getEducationalNoteForProcedure(proc)
	}

	// Add protocol-specific context
	lp.addProtocolSpecificContext(ctx, entry)

	return ctx
}

// forwardToLoki sends the structured log entry to Loki
func (lp *LogParser) forwardToLoki(entry *StructuredLogEntry) {
	// Convert timestamp to nanoseconds
	timestampNano := fmt.Sprintf("%d", entry.Timestamp.UnixNano())

	// Convert entry to JSON
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		log.Printf("⚠️ Failed to marshal log entry: %v", err)
		return
	}

	// Create Loki stream
	stream := LokiStreamEntry{
		Stream: entry.Labels,
		Values: [][]string{
			{timestampNano, string(entryJSON)},
		},
	}

	// Create push request
	pushReq := LokiPushRequest{
		Streams: []LokiStreamEntry{stream},
	}

	// Send to Loki
	lp.sendToLoki(pushReq)
}

// sendToLoki sends the push request to Loki
func (lp *LogParser) sendToLoki(pushReq LokiPushRequest) {
	reqBody, err := json.Marshal(pushReq)
	if err != nil {
		log.Printf("⚠️ Failed to marshal Loki request: %v", err)
		return
	}

	url := fmt.Sprintf("%s/loki/api/v1/push", lp.lokiURL)
	resp, err := lp.httpClient.Post(url, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		log.Printf("⚠️ Failed to send to Loki: %v", err)
		return
	}
	defer func() {
		err = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusNoContent {
		log.Printf("⚠️ Loki returned status %d", resp.StatusCode)
	}
}

// startHTTPServer starts the HTTP server for log parser endpoints
func (lp *LogParser) startHTTPServer() {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "healthy"}); err != nil {
			log.Printf("❌ Log collector server error: %v", err)
		}
	})

	// Parser status
	mux.HandleFunc("/status", lp.handleParserStatus)

	// Manual log processing endpoint
	mux.HandleFunc("/parse", lp.handleManualParse)

	server := &http.Server{
		Addr:    ":8082",
		Handler: mux,
	}

	log.Printf("🌐 Log Parser HTTP server starting on :8082")
	if err := server.ListenAndServe(); err != nil {
		log.Printf("⚠️ HTTP server error: %v", err)
	}
}

// handleParserStatus returns the current status of all parsers
func (lp *LogParser) handleParserStatus(w http.ResponseWriter, r *http.Request) {
	lp.mu.RLock()
	defer lp.mu.RUnlock()

	status := map[string]any{
		"active_parsers":   len(lp.parsers),
		"active_watchers":  len(lp.watchers),
		"educational_mode": lp.educationalMode,
		"loki_url":         lp.lokiURL,
		"parsers":          make(map[string]map[string]string),
	}

	for name, parser := range lp.parsers {
		status["parsers"].(map[string]map[string]string)[name] = map[string]string{
			"type":   parser.ComponentType,
			"format": parser.LogFormat,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Printf("❌ Log collector server error: %v", err)
	}
}

// handleManualParse allows manual parsing of log lines via HTTP
func (lp *LogParser) handleManualParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Component string `json:"component"`
		LogLine   string `json:"log_line"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	lp.mu.RLock()
	parser, exists := lp.parsers[req.Component]
	lp.mu.RUnlock()

	if !exists {
		http.Error(w, "Component not found", http.StatusNotFound)
		return
	}

	entry := lp.parseLogLine(parser, req.LogLine)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(entry); err != nil {
		log.Printf("❌ Log collector server error: %v", err)
	}
}

// Stop gracefully stops the log parser service
func (lp *LogParser) Stop() {
	log.Printf("🛑 Stopping Log Parser Service...")

	lp.cancel()

	lp.mu.Lock()
	defer lp.mu.Unlock()

	for source, watcher := range lp.watchers {
		if err := watcher.Close(); err != nil {
			log.Printf("⚠️ Error closing watcher for %s: %v", source, err)
		}
	}

	log.Printf("✅ Log Parser Service stopped")
}

// Helper functions

func determineLogFormat(componentType string) string {
	switch {
	case strings.Contains(componentType, "ran") || strings.Contains(componentType, "enb") || strings.Contains(componentType, "gnb"):
		if strings.Contains(componentType, "srsran") {
			return "srsran"
		}
		return "srslte"
	default:
		return "open5gs"
	}
}

func createLogPatterns(componentType string) map[string]*regexp.Regexp {
	patterns := make(map[string]*regexp.Regexp)

	// Common patterns
	patterns["timestamp"] = regexp.MustCompile(`\d{4}/\d{2}/\d{2}\s\d{2}:\d{2}:\d{2}`)
	patterns["level"] = regexp.MustCompile(`\[(DEBUG|INFO|WARN|ERROR|FATAL)\]`)
	patterns["imsi"] = regexp.MustCompile(`imsi[:\s]*(\d{15})`)

	return patterns
}

func extractComponentFromPath(filePath string) string {
	// Extract component name from file path
	// e.g., /var/log/open5gs/amf.log -> amf
	base := filepath.Base(filePath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	return name
}

func initializeProtocolRegexes() map[string]*regexp.Regexp {
	regexes := make(map[string]*regexp.Regexp)

	regexes["nas_message"] = regexp.MustCompile(`NAS.*?(Attach|Detach|TAU|Registration)`)
	regexes["rrc_message"] = regexp.MustCompile(`RRC.*?(Setup|Release|Reconfiguration)`)
	regexes["s1ap_message"] = regexp.MustCompile(`S1AP.*?(InitialUEMessage|DownlinkNASTransport)`)
	regexes["ngap_message"] = regexp.MustCompile(`NGAP.*?(InitialUEMessage|DownlinkNASTransport)`)

	return regexes
}

func getSpecificationForLayer(layer string) string {
	specs := map[string]string{
		"NAS":  "3GPP TS 24.301 (4G) / TS 24.501 (5G)",
		"RRC":  "3GPP TS 36.331 (4G) / TS 38.331 (5G)",
		"S1AP": "3GPP TS 36.413",
		"NGAP": "3GPP TS 38.413",
		"PHY":  "3GPP TS 36.211 (4G) / TS 38.211 (5G)",
		"MAC":  "3GPP TS 36.321 (4G) / TS 38.321 (5G)",
	}

	if spec, exists := specs[layer]; exists {
		return spec
	}
	return ""
}

func getEducationalNoteForProcedure(procedure string) string {
	notes := map[string]string{
		"attach":         "Initial UE attachment to the network - establishes security context and default bearer",
		"detach":         "UE disconnection from network - releases all bearers and security contexts",
		"handover":       "UE mobility between cells - maintains session continuity",
		"authentication": "Network verifies UE identity using shared secret keys",
		"registration":   "5G equivalent of attach - registers UE with AMF",
		"paging":         "Network initiates contact with idle UE for incoming services",
	}

	if note, exists := notes[procedure]; exists {
		return note
	}
	return ""
}

func (lp *LogParser) addProtocolSpecificContext(ctx *EducationalContext, entry *StructuredLogEntry) {
	// Add flow step information for common procedures
	if proc := ctx.Procedure; proc != "" {
		switch proc {
		case "attach":
			if strings.Contains(entry.Message, "Initial") {
				ctx.FlowStep = 1
			} else if strings.Contains(entry.Message, "Authentication") {
				ctx.FlowStep = 2
			} else if strings.Contains(entry.Message, "Security") {
				ctx.FlowStep = 3
			} else if strings.Contains(entry.Message, "Complete") {
				ctx.FlowStep = 4
			}
		}
	}
}
