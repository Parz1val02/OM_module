package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// DeploymentType represents the type of deployment
type DeploymentType string

const (
	TYPE_4G    DeploymentType = "4G"
	TYPE_5G    DeploymentType = "5G"
	TYPE_MIXED DeploymentType = "MIXED"
)

// Component represents a network function component
type Component struct {
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	IP        string            `json:"ip"`
	Ports     []string          `json:"ports"`
	Status    string            `json:"status"`
	Image     string            `json:"image"`
	Labels    map[string]string `json:"labels"`
	IsRunning bool              `json:"is_running"`
}

// NetworkTopology represents the discovered network topology
type NetworkTopology struct {
	Type        DeploymentType       `json:"type"`
	Components  map[string]Component `json:"components"`
	Environment map[string]string    `json:"environment"`
}

// AutoDiscoveryService handles automatic discovery of network components
type AutoDiscoveryService struct {
	dockerClient *client.Client
	envFile      string
}

// NewAutoDiscoveryService creates a new auto-discovery service
func NewAutoDiscoveryService(envFile string) (*AutoDiscoveryService, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &AutoDiscoveryService{
		dockerClient: cli,
		envFile:      envFile,
	}, nil
}

// DiscoverTopology discovers the complete network topology
func (ads *AutoDiscoveryService) DiscoverTopology(ctx context.Context) (*NetworkTopology, error) {
	// 1. Parse environment configuration
	envConfig, err := ads.parseEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to parse environment: %w", err)
	}

	// 2. Discover running containers
	containers, err := ads.discoverContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to discover containers: %w", err)
	}

	// 3. Determine deployment type
	deploymentType := ads.determineDeploymentType(containers)

	// 4. Map containers to components with environment data
	components := ads.mapComponents(containers, envConfig)

	topology := &NetworkTopology{
		Type:        deploymentType,
		Components:  components,
		Environment: envConfig,
	}

	return topology, nil
}

// parseEnvironment parses the .env file
func (ads *AutoDiscoveryService) parseEnvironment() (map[string]string, error) {
	file, err := os.Open(ads.envFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open env file %s: %w", ads.envFile, err)
	}

	defer func() {
		err = file.Close()
	}()

	env := make(map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE pairs
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			env[key] = value
		}
	}

	return env, scanner.Err()
}

// discoverContainers discovers running containers
func (ads *AutoDiscoveryService) discoverContainers(ctx context.Context) ([]container.Summary, error) {
	containers, err := ads.dockerClient.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Filter containers related to open5gs
	var relevantContainers []container.Summary
	for _, c := range containers {
		if ads.isRelevantContainer(c) {
			relevantContainers = append(relevantContainers, c)
		}
	}

	return relevantContainers, nil
}

// isRelevantContainer checks if a container is relevant to our monitoring
func (ads *AutoDiscoveryService) isRelevantContainer(c container.Summary) bool {
	// Check if container is part of open5gs network
	for _, network := range c.NetworkSettings.Networks {
		if strings.Contains(network.NetworkID, "open5gs") {
			return true
		}
	}

	// Check container image
	relevantImages := []string{
		"docker_open5gs",
		"docker_srslte",
		"docker_srsran",
		"docker_metrics",
		"grafana",
		"mongo",
	}

	for _, image := range relevantImages {
		if strings.Contains(c.Image, image) {
			return true
		}
	}

	return false
}

// determineDeploymentType determines if this is 4G, 5G, or mixed deployment
func (ads *AutoDiscoveryService) determineDeploymentType(containers []container.Summary) DeploymentType {
	has4G := false
	has5G := false

	// 4G indicators
	fourGComponents := []string{"mme", "hss", "sgwc", "sgwu", "pcrf"}
	// 5G indicators
	fiveGComponents := []string{"amf", "nrf", "udm", "udr", "ausf", "nssf", "bsf", "pcf"}

	for _, c := range containers {
		containerName := ads.getContainerName(c)

		// Check for 4G components
		for _, component := range fourGComponents {
			if strings.Contains(containerName, component) {
				has4G = true
				break
			}
		}

		// Check for 5G components
		for _, component := range fiveGComponents {
			if strings.Contains(containerName, component) {
				has5G = true
				break
			}
		}
	}

	if has4G && has5G {
		return TYPE_MIXED
	} else if has5G {
		return TYPE_5G
	} else if has4G {
		return TYPE_4G
	}

	return TYPE_MIXED // Default fallback
}

// mapComponents maps containers to components with environment data
func (ads *AutoDiscoveryService) mapComponents(containers []container.Summary, env map[string]string) map[string]Component {
	components := make(map[string]Component)

	for _, c := range containers {
		name := ads.getContainerName(c)

		component := Component{
			Name:      name,
			Type:      ads.getComponentType(name),
			IP:        ads.getComponentIP(name, env),
			Ports:     ads.getContainerPorts(c),
			Status:    c.Status,
			Image:     c.Image,
			Labels:    c.Labels,
			IsRunning: c.State == "running",
		}

		components[name] = component
	}

	return components
}

// getContainerName extracts clean container name
func (ads *AutoDiscoveryService) getContainerName(c container.Summary) string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	return c.ID[:12]
}

// getComponentType determines the component type based on name
func (ads *AutoDiscoveryService) getComponentType(name string) string {
	componentTypes := map[string]string{
		// 4G Components
		"mme":  "4G-Control-Plane",
		"hss":  "4G-Database",
		"sgwc": "4G-Control-Plane",
		"sgwu": "4G-User-Plane",
		"pcrf": "4G-Policy",

		// 5G Components
		"amf":  "5G-Control-Plane",
		"smf":  "5G-Control-Plane",
		"upf":  "5G-User-Plane",
		"nrf":  "5G-Service-Discovery",
		"udm":  "5G-User-Management",
		"udr":  "5G-Database",
		"ausf": "5G-Authentication",
		"pcf":  "5G-Policy",
		"nssf": "5G-Slicing",
		"bsf":  "5G-Session-Management",
		"scp":  "5G-Service-Communication-Proxy",

		// Common
		"webui":   "Web-Interface",
		"mongo":   "Database",
		"grafana": "Monitoring",
		"metrics": "Metrics-Collection",
		"srsenb":  "4G-Radio-Access-Network",
		"srsgnb":  "5G-Radio-Access-Network",
		"srsue":   "User-Equipment",
	}

	for key, componentType := range componentTypes {
		if strings.Contains(name, key) {
			return componentType
		}
	}

	return "Unknown"
}

// getComponentIP gets the IP address from environment config
func (ads *AutoDiscoveryService) getComponentIP(name string, env map[string]string) string {
	// Map container names to environment variable names
	ipMappings := map[string]string{
		"mme":     "MME_IP",
		"hss":     "HSS_IP",
		"sgwc":    "SGWC_IP",
		"sgwu":    "SGWU_IP",
		"pcrf":    "PCRF_IP",
		"amf":     "AMF_IP",
		"smf":     "SMF_IP",
		"upf":     "UPF_IP",
		"nrf":     "NRF_IP",
		"udm":     "UDM_IP",
		"udr":     "UDR_IP",
		"ausf":    "AUSF_IP",
		"pcf":     "PCF_IP",
		"nssf":    "NSSF_IP",
		"bsf":     "BSF_IP",
		"scp":     "SCP_IP",
		"webui":   "WEBUI_IP",
		"mongo":   "MONGO_IP",
		"grafana": "GRAFANA_IP",
		"metrics": "METRICS_IP",
		"srsenb":  "SRS_ENB_IP",
		"srsgnb":  "SRS_GNB_IP",
		"srsue":   "SRS_UE_IP",
	}

	for key, envVar := range ipMappings {
		if strings.Contains(name, key) {
			if ip, exists := env[envVar]; exists {
				return ip
			}
		}
	}

	return "Unknown"
}

// getContainerPorts extracts port information from container
func (ads *AutoDiscoveryService) getContainerPorts(c container.Summary) []string {
	var ports []string
	for _, port := range c.Ports {
		if port.PublicPort != 0 {
			ports = append(ports, fmt.Sprintf("%d:%d/%s", port.PublicPort, port.PrivatePort, port.Type))
		} else {
			ports = append(ports, fmt.Sprintf("%d/%s", port.PrivatePort, port.Type))
		}
	}
	return ports
}

// GetHealthStatus performs health checks on discovered components
func (ads *AutoDiscoveryService) GetHealthStatus(ctx context.Context) (map[string]string, error) {
	containers, err := ads.discoverContainers(ctx)
	if err != nil {
		return nil, err
	}

	healthStatus := make(map[string]string)

	for _, c := range containers {
		name := ads.getContainerName(c)

		// Basic health check based on container state
		switch c.State {
		case "running":
			healthStatus[name] = "healthy"
		case "exited":
			healthStatus[name] = "failed"
		case "restarting":
			healthStatus[name] = "recovering"
		default:
			healthStatus[name] = "unknown"
		}
	}

	return healthStatus, nil
}

// GetComponentMetrics retrieves basic metrics for components
func (ads *AutoDiscoveryService) GetComponentMetrics(ctx context.Context, componentName string) (map[string]any, error) {
	// This would typically integrate with Docker stats API
	// For now, return placeholder metrics structure

	containers, err := ads.discoverContainers(ctx)
	if err != nil {
		return nil, err
	}

	for _, c := range containers {
		if ads.getContainerName(c) == componentName {
			// In a real implementation, you'd call:
			// stats, err := ads.dockerClient.ContainerStats(ctx, c.ID, false)

			metrics := map[string]any{
				"container_id": c.ID,
				"state":        c.State,
				"status":       c.Status,
				"image":        c.Image,
				"created":      c.Created,
				// Add actual resource metrics here when implementing Docker stats
			}

			return metrics, nil
		}
	}

	return nil, fmt.Errorf("component %s not found", componentName)
}

// GetNetworkTopologyJSON returns the topology as formatted JSON
func (ads *AutoDiscoveryService) GetNetworkTopologyJSON(ctx context.Context) (string, error) {
	topology, err := ads.DiscoverTopology(ctx)
	if err != nil {
		return "", err
	}

	// Convert to JSON with proper formatting
	jsonBytes, err := json.Marshal(topology)
	if err != nil {
		return "", fmt.Errorf("failed to marshal topology to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// ListActiveComponents returns a simple list of active component names
func (ads *AutoDiscoveryService) ListActiveComponents(ctx context.Context) ([]string, error) {
	containers, err := ads.discoverContainers(ctx)
	if err != nil {
		return nil, err
	}

	var activeComponents []string
	for _, c := range containers {
		if c.State == "running" {
			activeComponents = append(activeComponents, ads.getContainerName(c))
		}
	}

	return activeComponents, nil
}

// Close closes the docker client
func (ads *AutoDiscoveryService) Close() error {
	if ads.dockerClient != nil {
		return ads.dockerClient.Close()
	}
	return nil
}
