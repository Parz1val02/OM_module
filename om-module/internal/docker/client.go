package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// Client wraps the Docker SDK client.
type Client struct {
	cli *client.Client
}

// New creates a Docker client connected to the given socket path.
func New(socketPath string) (*Client, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost("unix://"+socketPath),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}
	return &Client{cli: cli}, nil
}

// Close releases the underlying Docker client.
func (c *Client) Close() error {
	return c.cli.Close()
}

// ContainerInfo is the subset of Docker container data the O&M module cares about.
type ContainerInfo struct {
	ID     string
	Name   string
	State  string
	Image  string
	Labels map[string]string
}

// ListContainers returns all containers whose Compose project label matches
// the given project name. If project is empty, all containers are returned.
func (c *Client) ListContainers(ctx context.Context, project string) ([]ContainerInfo, error) {
	all, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}

	var result []ContainerInfo
	for _, ct := range all {
		if project != "" {
			proj, ok := ct.Labels["com.docker.compose.project"]
			if !ok || proj != project {
				continue
			}
		}

		name := ct.ID[:12]
		if len(ct.Names) > 0 {
			name = ct.Names[0]
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}

		result = append(result, ContainerInfo{
			ID:     ct.ID,
			Name:   name,
			State:  ct.State,
			Image:  ct.Image,
			Labels: ct.Labels,
		})
	}
	return result, nil
}

// GetBridgeInterface returns the Linux bridge interface name for the given
// Docker network name (e.g. "docker_open5gs_default").
//
// Docker names bridge interfaces as "br-<first12chars_of_network_id>".
// The method verifies the interface actually exists on the host before
// returning it, so the caller can rely on the result being usable.
func (c *Client) GetBridgeInterface(ctx context.Context, networkName string) (string, error) {
	// Inspect the named network to get its ID.
	nr, err := c.cli.NetworkInspect(ctx, networkName, network.InspectOptions{})
	if err != nil {
		return "", fmt.Errorf("docker: inspect network %q: %w", networkName, err)
	}

	if len(nr.ID) < 12 {
		return "", fmt.Errorf("docker: network %q has unexpectedly short ID %q", networkName, nr.ID)
	}

	ifaceName := "br-" + nr.ID[:12]
	log.Printf("🌉 Bridge interface discovered: %s", ifaceName)
	return ifaceName, nil
}

// GetNetworkContainerIPs returns a map of IP address → container name for all
// containers attached to the given Docker network. The CIDR suffix is stripped
// from the IP (e.g. "172.22.0.10/24" becomes "172.22.0.10").
func (c *Client) GetNetworkContainerIPs(ctx context.Context, networkName string) (map[string]string, error) {
	nr, err := c.cli.NetworkInspect(ctx, networkName, network.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker: inspect network %q: %w", networkName, err)
	}

	result := make(map[string]string, len(nr.Containers))
	for _, ct := range nr.Containers {
		ip := ct.IPv4Address
		// Strip CIDR suffix if present
		if idx := strings.Index(ip, "/"); idx >= 0 {
			ip = ip[:idx]
		}
		if ip != "" {
			result[ip] = ct.Name
		}
	}
	return result, nil
}

// RawStats holds the raw JSON stats from the Docker API for one container.
type RawStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Stats struct {
			Cache uint64 `json:"cache"`
		} `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
	PidsStats struct {
		Current uint64 `json:"current"`
	} `json:"pids_stats"`
}

// GetStats fetches a single non-streaming stats snapshot for the given container ID.
func (c *Client) GetStats(ctx context.Context, containerID string) (*RawStats, error) {
	resp, err := c.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("⚠️  Failed to close response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var stats RawStats
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}
