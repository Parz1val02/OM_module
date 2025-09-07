package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parz1val02/OM_module/discovery"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// ContainerStats represents container resource usage statistics
type ContainerStats struct {
	ContainerID    string  `json:"container_id"`
	Name           string  `json:"name"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryUsage    uint64  `json:"memory_usage"`
	MemoryLimit    uint64  `json:"memory_limit"`
	MemoryPercent  float64 `json:"memory_percent"`
	NetworkRxBytes uint64  `json:"network_rx_bytes"`
	NetworkTxBytes uint64  `json:"network_tx_bytes"`
	BlockRead      uint64  `json:"block_read"`
	BlockWrite     uint64  `json:"block_write"`
	PIDs           uint64  `json:"pids"`
	Timestamp      int64   `json:"timestamp"`
}

// cpuStats represents CPU statistics for delta calculation
type cpuStats struct {
	TotalUsage  uint64
	SystemUsage uint64
	OnlineCPUs  uint32
}

// ContainerMetricsCollector handles collection of container-level metrics
type ContainerMetricsCollector struct {
	dockerClient *client.Client
	topology     *discovery.NetworkTopology
	statsCache   map[string]*ContainerStats
	lastCPUStats map[string]cpuStats
	port         int
}

// NewContainerMetricsCollector creates a new container metrics collector
func NewContainerMetricsCollector(port int) (*ContainerMetricsCollector, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &ContainerMetricsCollector{
		dockerClient: cli,
		statsCache:   make(map[string]*ContainerStats),
		lastCPUStats: make(map[string]cpuStats),
		port:         port,
	}, nil
}

// Enhanced Start method with better shutdown messaging
func (cmc *ContainerMetricsCollector) Start(ctx context.Context, topology *discovery.NetworkTopology) error {
	cmc.topology = topology

	log.Printf("📊 Starting Container Metrics Collector on port %d", cmc.port)

	// Set up HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/container/metrics", cmc.handleMetricsRequest)
	mux.HandleFunc("/health", cmc.handleHealthCheck)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cmc.port),
		Handler: mux,
	}

	// Start server in goroutine

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("📊 Container metrics server listening on :%d", cmc.port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Start periodic collection
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("📊 Stopping Container Metrics Collector...")

			// Graceful shutdown with timeout
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := server.Shutdown(shutdownCtx); err != nil {
				log.Printf("⚠️  Container metrics server shutdown error: %v", err)
			} else {
				log.Printf("✅ Container metrics server stopped gracefully")
			}

			return ctx.Err()

		case err := <-serverErr:
			log.Printf("❌ Container metrics server error: %v", err)
			return err

		case <-ticker.C:
			// Only collect if context is still valid
			if ctx.Err() == nil {
				if err := cmc.collectAllContainerStats(ctx); err != nil && err != context.Canceled {

					log.Printf("⚠️  Failed to collect container stats: %v", err)
				}
			}
		}
	}
}

// collectMetricsPeriodically collects metrics from all containers periodically
func (cmc *ContainerMetricsCollector) collectMetricsPeriodically(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := cmc.collectAllContainerStats(ctx); err != nil {
				log.Printf("⚠️  Failed to collect container stats: %v", err)
			}
		}
	}
}

// collectAllContainerStats collects stats for all running containers in topology
func (cmc *ContainerMetricsCollector) collectAllContainerStats(ctx context.Context) error {
	if cmc.topology == nil {
		return fmt.Errorf("topology not set")
	}

	// Check if context is already cancelled to avoid unnecessary work
	select {
	case <-ctx.Done():

		log.Printf("📊 Container stats collection stopped (shutdown in progress)")
		return ctx.Err()
	default:
	}

	for name, component := range cmc.topology.Components {
		if !component.IsRunning {
			continue
		}

		// Check context before each container to enable faster shutdown
		select {
		case <-ctx.Done():
			log.Printf("📊 Container stats collection cancelled during processing")
			return ctx.Err()
		default:
		}

		// Get container by name with context check
		containers, err := cmc.dockerClient.ContainerList(ctx, container.ListOptions{})
		if err != nil {
			// Don't log as warning if it's due to context cancellation
			if ctx.Err() != nil {
				log.Printf("📊 Container stats collection stopped during shutdown")
				return ctx.Err()

			}
			return fmt.Errorf("failed to list containers: %w", err)
		}

		var targetContainer *container.Summary
		for _, c := range containers {

			containerName := strings.TrimPrefix(c.Names[0], "/")
			if containerName == name {
				targetContainer = &c

				break
			}
		}

		if targetContainer == nil {
			continue
		}

		// Collect stats for this container with context awareness

		stats, err := cmc.collectContainerStats(ctx, targetContainer.ID, name)
		if err != nil {
			// Don't log as warning if it's due to context cancellation
			if ctx.Err() != nil {
				log.Printf("📊 Container stats collection for %s stopped during shutdown", name)
				return ctx.Err()
			}
			log.Printf("⚠️  Failed to collect stats for %s: %v", name, err)
			continue
		}

		cmc.statsCache[name] = stats
	}

	return nil
}

// Enhanced collectContainerStats with better error handling
func (cmc *ContainerMetricsCollector) collectContainerStats(ctx context.Context, containerID, name string) (*ContainerStats, error) {
	// Check context before making Docker API call

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:

	}

	// Get container stats with context
	statsJSON, err := cmc.dockerClient.ContainerStats(ctx, containerID, false)
	if err != nil {
		// Don't treat context cancellation as an error
		if ctx.Err() != nil {

			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to get container stats: %w", err)
	}
	defer func() {
		if closeErr := statsJSON.Body.Close(); closeErr != nil {
			// Only log close errors if not during shutdown
			if ctx.Err() == nil {
				log.Printf("⚠️  Failed to close stats response: %v", closeErr)
			}
		}

	}()

	// Check context before parsing response
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Read and parse stats JSON
	statsData, err := io.ReadAll(statsJSON.Body)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()

		}
		return nil, fmt.Errorf("failed to read stats data: %w", err)
	}

	// Parse the raw JSON stats
	var rawStats map[string]any
	if err := json.Unmarshal(statsData, &rawStats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stats: %w", err)
	}

	// Create stats object
	stats := &ContainerStats{

		ContainerID: containerID,
		Name:        name,
		Timestamp:   time.Now().Unix(),
	}

	// Extract metrics with error handling
	if err := cmc.extractCPUStats(rawStats, stats, name); err != nil {
		log.Printf("⚠️  Failed to extract CPU stats for %s: %v", name, err)
	}

	if err := cmc.extractMemoryStats(rawStats, stats); err != nil {
		log.Printf("⚠️  Failed to extract memory stats for %s: %v", name, err)
	}

	if err := cmc.extractNetworkStats(rawStats, stats); err != nil {
		log.Printf("⚠️  Failed to extract network stats for %s: %v", name, err)
	}

	if err := cmc.extractBlockIOStats(rawStats, stats); err != nil {
		log.Printf("⚠️  Failed to extract block I/O stats for %s: %v", name, err)
	}

	if err := cmc.extractPIDStats(rawStats, stats); err != nil {
		log.Printf("⚠️  Failed to extract PID stats for %s: %v", name, err)
	}

	return stats, nil
}

// extractCPUStats safely extracts CPU statistics
func (cmc *ContainerMetricsCollector) extractCPUStats(rawStats map[string]any, stats *ContainerStats, name string) error {
	cpuStatsRaw, ok := rawStats["cpu_stats"].(map[string]any)
	if !ok {
		return fmt.Errorf("cpu_stats not found or wrong type")
	}

	// Extract current CPU usage
	var totalUsage, systemUsage uint64
	var onlineCPUs uint32

	if cpuUsage, ok := cpuStatsRaw["cpu_usage"].(map[string]any); ok {
		if total, ok := cpuUsage["total_usage"].(float64); ok {
			totalUsage = uint64(total)
		}
	}

	if system, ok := cpuStatsRaw["system_cpu_usage"].(float64); ok {
		systemUsage = uint64(system)
	}

	if cpus, ok := cpuStatsRaw["online_cpus"].(float64); ok {
		onlineCPUs = uint32(cpus)
	}

	// Calculate CPU percentage using our delta method
	previousCPU, exists := cmc.lastCPUStats[name]
	if exists && systemUsage > previousCPU.SystemUsage && totalUsage > previousCPU.TotalUsage {
		cpuDelta := float64(totalUsage - previousCPU.TotalUsage)
		systemDelta := float64(systemUsage - previousCPU.SystemUsage)

		if systemDelta > 0 {
			numCPUs := float64(onlineCPUs)
			if numCPUs == 0 {
				numCPUs = 1 // fallback
			}
			stats.CPUPercent = (cpuDelta / systemDelta) * numCPUs * 100.0
		}
	}

	// Store current stats for next calculation
	cmc.lastCPUStats[name] = cpuStats{
		TotalUsage:  totalUsage,
		SystemUsage: systemUsage,
		OnlineCPUs:  onlineCPUs,
	}

	return nil
}

// extractMemoryStats safely extracts memory statistics
func (cmc *ContainerMetricsCollector) extractMemoryStats(rawStats map[string]any, stats *ContainerStats) error {
	memStatsRaw, ok := rawStats["memory_stats"].(map[string]any)
	if !ok {
		return fmt.Errorf("memory_stats not found or wrong type")
	}

	if usage, ok := memStatsRaw["usage"].(float64); ok {
		stats.MemoryUsage = uint64(usage)
	}

	if limit, ok := memStatsRaw["limit"].(float64); ok {
		stats.MemoryLimit = uint64(limit)
		if stats.MemoryLimit > 0 {
			stats.MemoryPercent = float64(stats.MemoryUsage) / float64(stats.MemoryLimit) * 100
		}
	}

	return nil
}

// extractNetworkStats safely extracts network statistics
func (cmc *ContainerMetricsCollector) extractNetworkStats(rawStats map[string]any, stats *ContainerStats) error {
	networksRaw, ok := rawStats["networks"].(map[string]any)
	if !ok {
		return fmt.Errorf("networks not found or wrong type")
	}

	for _, networkInterface := range networksRaw {
		if netStats, ok := networkInterface.(map[string]any); ok {
			if rxBytes, ok := netStats["rx_bytes"].(float64); ok {
				stats.NetworkRxBytes += uint64(rxBytes)
			}
			if txBytes, ok := netStats["tx_bytes"].(float64); ok {
				stats.NetworkTxBytes += uint64(txBytes)
			}
		}
	}

	return nil
}

// extractBlockIOStats safely extracts block I/O statistics
func (cmc *ContainerMetricsCollector) extractBlockIOStats(rawStats map[string]any, stats *ContainerStats) error {
	// Try different possible field names for block I/O stats
	blockIOFieldNames := []string{"blokio_stats", "blockio_stats", "blkio_stats"}

	var blkioStatsRaw map[string]any
	var found bool

	for _, fieldName := range blockIOFieldNames {
		if blkioRaw, ok := rawStats[fieldName].(map[string]any); ok {
			blkioStatsRaw = blkioRaw
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("block I/O stats not found")
	}

	// Extract I/O service bytes
	if ioServiceBytes, ok := blkioStatsRaw["io_service_bytes_recursive"].([]any); ok {
		for _, ioEntry := range ioServiceBytes {
			if ioMap, ok := ioEntry.(map[string]any); ok {
				if op, ok := ioMap["op"].(string); ok {
					if value, ok := ioMap["value"].(float64); ok {
						switch strings.ToLower(op) {
						case "read":
							stats.BlockRead += uint64(value)
						case "write":
							stats.BlockWrite += uint64(value)
						}
					}
				}
			}
		}
	}

	return nil
}

// extractPIDStats safely extracts PID statistics
func (cmc *ContainerMetricsCollector) extractPIDStats(rawStats map[string]any, stats *ContainerStats) error {
	pidsStatsRaw, ok := rawStats["pids_stats"].(map[string]any)
	if !ok {
		return fmt.Errorf("pids_stats not found or wrong type")
	}

	if current, ok := pidsStatsRaw["current"].(float64); ok {
		stats.PIDs = uint64(current)
	}

	return nil
}

// handleMetricsRequest handles Prometheus metrics endpoint requests
func (cmc *ContainerMetricsCollector) handleMetricsRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	// Generate Prometheus metrics format
	metrics := cmc.generatePrometheusMetrics()

	if _, err := w.Write([]byte(metrics)); err != nil {
		log.Printf("❌ Failed to write metrics response: %v", err)
	}
}

// handleHealthCheck handles health check requests
func (cmc *ContainerMetricsCollector) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]any{
		"status":               "healthy",
		"timestamp":            time.Now().Unix(),
		"containers_monitored": len(cmc.statsCache),
		"docker_version":       "v28.3.3+incompatible",
	}

	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Printf("❌ Failed to decode metrics response: %v", err)
	}
}

// generatePrometheusMetrics generates Prometheus-formatted metrics
func (cmc *ContainerMetricsCollector) generatePrometheusMetrics() string {
	var metrics strings.Builder

	// Add metadata
	metrics.WriteString("# HELP container_cpu_usage_percent Container CPU usage percentage\n")
	metrics.WriteString("# TYPE container_cpu_usage_percent gauge\n")

	metrics.WriteString("# HELP container_memory_usage_bytes Container memory usage in bytes\n")
	metrics.WriteString("# TYPE container_memory_usage_bytes gauge\n")

	metrics.WriteString("# HELP container_memory_usage_percent Container memory usage percentage\n")
	metrics.WriteString("# TYPE container_memory_usage_percent gauge\n")

	metrics.WriteString("# HELP container_network_rx_bytes Container network received bytes\n")
	metrics.WriteString("# TYPE container_network_rx_bytes counter\n")

	metrics.WriteString("# HELP container_network_tx_bytes Container network transmitted bytes\n")
	metrics.WriteString("# TYPE container_network_tx_bytes counter\n")

	metrics.WriteString("# HELP container_block_read_bytes Container block device read bytes\n")
	metrics.WriteString("# TYPE container_block_read_bytes counter\n")

	metrics.WriteString("# HELP container_block_write_bytes Container block device write bytes\n")
	metrics.WriteString("# TYPE container_block_write_bytes counter\n")

	metrics.WriteString("# HELP container_pids Container process count\n")
	metrics.WriteString("# TYPE container_pids gauge\n")

	// Add metrics for each container
	for name, stats := range cmc.statsCache {
		labels := cmc.generateLabels(name)

		metrics.WriteString(fmt.Sprintf("container_cpu_usage_percent{%s} %s\n",
			labels, formatFloat(stats.CPUPercent)))

		metrics.WriteString(fmt.Sprintf("container_memory_usage_bytes{%s} %d\n",
			labels, stats.MemoryUsage))

		metrics.WriteString(fmt.Sprintf("container_memory_usage_percent{%s} %s\n",
			labels, formatFloat(stats.MemoryPercent)))

		metrics.WriteString(fmt.Sprintf("container_network_rx_bytes{%s} %d\n",
			labels, stats.NetworkRxBytes))

		metrics.WriteString(fmt.Sprintf("container_network_tx_bytes{%s} %d\n",
			labels, stats.NetworkTxBytes))

		metrics.WriteString(fmt.Sprintf("container_block_read_bytes{%s} %d\n",
			labels, stats.BlockRead))

		metrics.WriteString(fmt.Sprintf("container_block_write_bytes{%s} %d\n",
			labels, stats.BlockWrite))

		metrics.WriteString(fmt.Sprintf("container_pids{%s} %d\n",
			labels, stats.PIDs))
	}

	return metrics.String()
}

// generateLabels generates Prometheus labels for a container
func (cmc *ContainerMetricsCollector) generateLabels(name string) string {
	labels := []string{
		fmt.Sprintf(`container_name="%s"`, name),
		fmt.Sprintf(`container_id="%s"`, name),
	}

	// Add component-specific labels if topology is available
	if cmc.topology != nil {
		if component, exists := cmc.topology.Components[name]; exists {
			labels = append(labels,
				fmt.Sprintf(`component_type="%s"`, component.Type),
				fmt.Sprintf(`deployment_type="%s"`, cmc.topology.Type),
				fmt.Sprintf(`component_ip="%s"`, component.IP),
			)
		}
	}

	return strings.Join(labels, ",")
}

// formatFloat formats float64 to string with reasonable precision
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}

// UpdateTopology updates the topology reference
func (cmc *ContainerMetricsCollector) UpdateTopology(topology *discovery.NetworkTopology) {
	cmc.topology = topology
}

// GetStats returns current container stats
func (cmc *ContainerMetricsCollector) GetStats() map[string]*ContainerStats {
	return maps.Clone(cmc.statsCache)
}

// Close closes the Docker client
func (cmc *ContainerMetricsCollector) Close() error {
	if cmc.dockerClient != nil {
		return cmc.dockerClient.Close()
	}
	return nil
}
