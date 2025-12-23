package server

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/Parz1val02/OM_module/discovery"
	"github.com/Parz1val02/OM_module/logging"
)

// HTTPServer manages the logging HTTP server
type HTTPServer struct {
	server         *http.Server
	loggingService *logging.LoggingService
	topology       *discovery.NetworkTopology
}

// NewHTTPServer creates a new HTTP server instance
func NewHTTPServer(loggingService *logging.LoggingService) *HTTPServer {
	return &HTTPServer{
		loggingService: loggingService,
	}
}

// SetTopology updates the topology reference
func (s *HTTPServer) SetTopology(topology *discovery.NetworkTopology) {
	s.topology = topology
}

// Start starts the HTTP server on the specified port
func (s *HTTPServer) Start(port string) error {
	mux := http.NewServeMux()

	// Add logging endpoints
	s.addLoggingEndpoints(mux)

	// Create server
	s.server = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("🌐 Logging HTTP server starting on :%s", port)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("⚠️ Logging HTTP server error: %v", err)
		return err
	}
	return nil
}

// addLoggingEndpoints registers all HTTP endpoints
func (s *HTTPServer) addLoggingEndpoints(mux *http.ServeMux) {
	// Logging service endpoints
	mux.HandleFunc("/logging/status", s.handleLoggingStatus)
	mux.HandleFunc("/logging/configs", s.handlePromtailConfigs)
	mux.HandleFunc("/logging/health", s.handleLoggingHealth)
	mux.HandleFunc("/logging/dashboard", s.handleEducationalDashboard)

	// Educational endpoints
	mux.HandleFunc("/educational/insights", s.handleEducationalInsights)

	// Root endpoint for logging service
	mux.HandleFunc("/", s.handleRoot)
}

// handleRoot returns service information
func (s *HTTPServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	response := map[string]any{
		"service": "O&M Logging Service",
		"version": "1.0.0",
		"endpoints": []string{
			"/logging/status",
			"/logging/configs",
			"/logging/health",
			"/logging/dashboard",
			"/educational/insights",
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("❌ Log collector server error: %v", err)
	}
}

// getTopology returns the current topology
func (s *HTTPServer) getTopology() *discovery.NetworkTopology {
	return s.topology
}
