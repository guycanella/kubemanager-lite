package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

type ContainerInfo struct {
	ID      string `json:"id"`
	ShortID string `json:"shortId"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	State   string `json:"state"`
	Created int64  `json:"created"`

	CPUPercent    float64 `json:"cpuPercent"`
	MemoryUsageMB float64 `json:"memoryUsageMB"`
	MemoryLimitMB float64 `json:"memoryLimitMB"`
}

type StatsUpdate struct {
	ContainerID   string  `json:"containerId"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryUsageMB float64 `json:"memoryUsageMB"`
	MemoryLimitMB float64 `json:"memoryLimitMB"`
}

type StatsEmitter interface {
	EmitStats(update StatsUpdate)
}

type StatsStreamer struct {
	client  *Client
	emitter StatsEmitter
	streams map[string]context.CancelFunc
}

func NewStatsStreamer(client *Client, emitter StatsEmitter) *StatsStreamer {
	return &StatsStreamer{
		client:  client,
		emitter: emitter,
		streams: make(map[string]context.CancelFunc),
	}
}

func (ss *StatsStreamer) StartStream(containerID string) {
	if _, exists := ss.streams[containerID]; exists {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	ss.streams[containerID] = cancel

	go func() {
		defer delete(ss.streams, containerID)

		if err := ss.streamStats(ctx, containerID); err != nil {
			if ctx.Err() == nil {
				fmt.Printf("[StatsStreamer] Error streaming stats for %s: %v\n", containerID, err)
			}
		}
	}()
}

func (ss *StatsStreamer) StopStream(containerID string) {
	if cancel, exists := ss.streams[containerID]; exists {
		cancel()
		delete(ss.streams, containerID)
	}
}

func (ss *StatsStreamer) StopAll() {
	for id, cancel := range ss.streams {
		cancel()
		delete(ss.streams, id)
	}
}

func (ss *StatsStreamer) streamStats(ctx context.Context, containerID string) error {
	resp, err := ss.client.cli.ContainerStats(ctx, containerID, true) // stream=true
	if err != nil {
		return fmt.Errorf("failed to open stats stream: %w", err)
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		var stats types.StatsJSON
		if err := decoder.Decode(&stats); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("failed to decode stats: %w", err)
		}

		ss.emitter.EmitStats(StatsUpdate{
			ContainerID:   containerID,
			CPUPercent:    calculateCPUPercent(&stats),
			MemoryUsageMB: bytesToMB(stats.MemoryStats.Usage),
			MemoryLimitMB: bytesToMB(stats.MemoryStats.Limit),
		})
	}
}

func (c *Client) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]ContainerInfo, 0, len(containers))
	for _, ctr := range containers {
		name := "unknown"
		if len(ctr.Names) > 0 {
			name = ctr.Names[0][1:]
		}

		result = append(result, ContainerInfo{
			ID:      ctr.ID,
			ShortID: ctr.ID[:12],
			Name:    name,
			Image:   ctr.Image,
			Status:  ctr.Status,
			State:   ctr.State,
			Created: ctr.Created,
		})
	}

	return result, nil
}

func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	if err := c.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container %s: %w", containerID, err)
	}
	return nil
}

func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10 * time.Second
	if err := c.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: timeoutSeconds(timeout)}); err != nil {
		return fmt.Errorf("failed to stop container %s: %w", containerID, err)
	}
	return nil
}

func (c *Client) RestartContainer(ctx context.Context, containerID string) error {
	timeout := 10 * time.Second
	if err := c.cli.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: timeoutSeconds(timeout)}); err != nil {
		return fmt.Errorf("failed to restart container %s: %w", containerID, err)
	}
	return nil
}

// --- Internal helpers ---

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
	return math.Round(percent*100) / 100
}

func bytesToMB(bytes uint64) float64 {
	mb := float64(bytes) / (1024 * 1024)
	return math.Round(mb*100) / 100
}

func timeoutSeconds(d time.Duration) *int {
	s := int(d.Seconds())
	return &s
}
