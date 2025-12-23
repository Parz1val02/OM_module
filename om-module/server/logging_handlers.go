package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Parz1val02/OM_module/logging"
)

// handleLoggingStatus returns the status of the logging service
func (s *HTTPServer) handleLoggingStatus(w http.ResponseWriter, r *http.Request) {
	if s.loggingService == nil {
		http.Error(w, "Logging service not initialized", http.StatusServiceUnavailable)
		return
	}

	status := s.loggingService.GetStatus()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, "Failed to encode status", http.StatusInternalServerError)
		return
	}
}

// handlePromtailConfigs returns the generated Promtail configurations
func (s *HTTPServer) handlePromtailConfigs(w http.ResponseWriter, r *http.Request) {
	if s.loggingService == nil {
		http.Error(w, "Logging service not initialized", http.StatusServiceUnavailable)
		return
	}

	configs, err := s.loggingService.GetPromtailConfigs()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get configs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(configs); err != nil {
		http.Error(w, "Failed to encode configs", http.StatusInternalServerError)
		return
	}
}

// handleEducationalDashboard returns educational content for students
func (s *HTTPServer) handleEducationalDashboard(w http.ResponseWriter, r *http.Request) {
	if s.loggingService == nil {
		http.Error(w, "Logging service not initialized", http.StatusServiceUnavailable)
		return
	}

	topology := s.getTopology()

	dashboard := logging.GenerateEducationalDashboard(topology)
	insights := logging.GetEducationalInsights(topology)

	response := map[string]any{
		"dashboard": dashboard,
		"insights":  insights,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode dashboard", http.StatusInternalServerError)
		return
	}
}

// handleEducationalInsights returns educational insights
func (s *HTTPServer) handleEducationalInsights(w http.ResponseWriter, r *http.Request) {
	topology := s.getTopology()
	insights := logging.GetEducationalInsights(topology)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(insights); err != nil {
		http.Error(w, "Failed to encode insights", http.StatusInternalServerError)
		return
	}
}
