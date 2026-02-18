package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// ContainerInfo is the struct that will be serialized automatically by Wails
// in TypeScript via binding. All exported fields become TS properties.
type ContainerInfo struct {
	ID      string `json:"id"`
	ShortID string `json:"shortId"` // first 12 chars, more readable in the UI
	Name    string `json:"name"`
	Image   string `json:"image"`
	Status  string `json:"status"` // "running", "exited", "paused", etc.
	State   string `json:"state"`
	Created int64  `json:"created"` // Unix timestamp

	// Resource metrics (filled separately via Stats)
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryUsageMB float64 `json:"memoryUsageMB"`
	MemoryLimitMB float64 `json:"memoryLimitMB"`
}

// ListContainers returns all active containers on the local Docker.
// Called by Wails binding — the frontend calls this function directly via JS.
func (c *Client) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All: false, // only running containers
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]ContainerInfo, 0, len(containers))
	for _, ctr := range containers {
		name := "unknown"
		if len(ctr.Names) > 0 {
			// Docker prefixes names with "/", we remove it for the UI
			name = ctr.Names[0][1:]
		}

		info := ContainerInfo{
			ID:      ctr.ID,
			ShortID: ctr.ID[:12],
			Name:    name,
			Image:   ctr.Image,
			Status:  ctr.Status,
			State:   ctr.State,
			Created: ctr.Created,
		}

		result = append(result, info)
	}

	return result, nil
}

// GetContainerStats returns CPU and memory metrics of a specific container.
// Uses a single read (stream=false) to avoid keeping the connection open.
// For dashboard, we call this periodically from the frontend via polling or timer.
func (c *Client) GetContainerStats(ctx context.Context, containerID string) (*ContainerInfo, error) {
	resp, err := c.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats for container %s: %w", containerID, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read stats: %w", err)
	}

	var stats types.StatsJSON
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %w", err)
	}

	return &ContainerInfo{
		ID:            containerID,
		ShortID:       containerID[:12],
		CPUPercent:    calculateCPUPercent(&stats),
		MemoryUsageMB: bytesToMB(stats.MemoryStats.Usage),
		MemoryLimitMB: bytesToMB(stats.MemoryStats.Limit),
	}, nil
}

// StartContainer starts a stopped container.
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	if err := c.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container %s: %w", containerID, err)
	}
	return nil
}

// StopContainer stops a running container.
func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	if err := c.cli.ContainerStop(ctx, containerID, container.StopOptions{}); err != nil {
		return fmt.Errorf("failed to stop container %s: %w", containerID, err)
	}
	return nil
}

// RestartContainer restarts a container.
func (c *Client) RestartContainer(ctx context.Context, containerID string) error {
	if err := c.cli.ContainerRestart(ctx, containerID, container.StopOptions{}); err != nil {
		return fmt.Errorf("failed to restart container %s: %w", containerID, err)
	}
	return nil
}

// --- Helpers internos ---

// calculateCPUPercent calculates the CPU percentage based on the delta
// between two consecutive reads of stats (official Docker formula).
func calculateCPUPercent(stats *types.StatsJSON) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) -
		float64(stats.PreCPUStats.CPUUsage.TotalUsage)

	systemDelta := float64(stats.CPUStats.SystemUsage) -
		float64(stats.PreCPUStats.SystemUsage)

	numCPUs := float64(stats.CPUStats.OnlineCPUs)
	if numCPUs == 0 {
		numCPUs = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	}

	if systemDelta == 0 || cpuDelta < 0 {
		return 0.0
	}

	percent := (cpuDelta / systemDelta) * numCPUs * 100.0
	return math.Round(percent*100) / 100 // round to 2 decimal places
}

// bytesToMB converts bytes to megabytes with 2 decimal places.
func bytesToMB(bytes uint64) float64 {
	mb := float64(bytes) / (1024 * 1024)
	return math.Round(mb*100) / 100
}
