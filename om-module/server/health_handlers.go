package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

// handleLoggingHealth provides health check for logging components
func (s *HTTPServer) handleLoggingHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"status":    "healthy",
		"components": map[string]string{
			"loki":     "checking...",
			"parser":   "checking...",
			"promtail": "checking...",
		},
	}

	if s.loggingService != nil {
		status := s.loggingService.GetStatus()
		if running, ok := status["running"].(bool); ok && running {
			health["components"].(map[string]string)["logging_service"] = "healthy"
		} else {
			health["components"].(map[string]string)["logging_service"] = "stopped"
			health["status"] = "degraded"
		}
	} else {
		health["components"].(map[string]string)["logging_service"] = "not_initialized"
		health["status"] = "unhealthy"
	}

	// Check Loki connectivity
	go checkLokiHealth(health)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
		log.Printf("❌ Log health server error: %v", err)
	}
}

// checkLokiHealth checks if Loki is accessible
func checkLokiHealth(health map[string]any) {
	lokiURL := os.Getenv("LOKI_URL")
	if lokiURL == "" {
		lokiURL = "http://loki:3100"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(lokiURL + "/ready")
	if err != nil {
		health["components"].(map[string]string)["loki"] = "unreachable"
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		health["components"].(map[string]string)["loki"] = "healthy"
	} else {
		health["components"].(map[string]string)["loki"] = "unhealthy"
	}
}
