package docker

import (
	"math"
	"testing"

	"github.com/docker/docker/api/types/container"
)

// ─── calculateCPUPercent ──────────────────────────────────────────────────────

func TestCalculateCPUPercent(t *testing.T) {
	tests := []struct {
		name     string
		stats    container.StatsResponse
		expected float64
	}{
		{
			name: "normal usage with 2 CPUs",
			stats: container.StatsResponse{
				Stats: container.Stats{
					CPUStats: container.CPUStats{
						CPUUsage:    container.CPUUsage{TotalUsage: 200_000_000},
						SystemUsage: 2_000_000_000,
						OnlineCPUs:  2,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage:    container.CPUUsage{TotalUsage: 100_000_000},
						SystemUsage: 1_000_000_000,
					},
				},
			},
			expected: 20.00,
		},
		{
			name: "zero usage",
			stats: container.StatsResponse{
				Stats: container.Stats{
					CPUStats: container.CPUStats{
						CPUUsage:    container.CPUUsage{TotalUsage: 100_000_000},
						SystemUsage: 1_000_000_000,
						OnlineCPUs:  1,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage:    container.CPUUsage{TotalUsage: 100_000_000},
						SystemUsage: 1_000_000_000,
					},
				},
			},
			expected: 0.00,
		},
		{
			name: "zero system delta returns 0",
			stats: container.StatsResponse{
				Stats: container.Stats{
					CPUStats: container.CPUStats{
						CPUUsage:    container.CPUUsage{TotalUsage: 200_000_000},
						SystemUsage: 1_000_000_000,
						OnlineCPUs:  1,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage:    container.CPUUsage{TotalUsage: 100_000_000},
						SystemUsage: 1_000_000_000,
					},
				},
			},
			expected: 0.00,
		},
		{
			name: "falls back to percpu count when OnlineCPUs is 0",
			stats: container.StatsResponse{
				Stats: container.Stats{
					CPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage:  200_000_000,
							PercpuUsage: []uint64{100_000_000, 100_000_000},
						},
						SystemUsage: 2_000_000_000,
						OnlineCPUs:  0,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage:    container.CPUUsage{TotalUsage: 100_000_000},
						SystemUsage: 1_000_000_000,
					},
				},
			},
			expected: 20.00,
		},
		{
			name: "high CPU usage",
			stats: container.StatsResponse{
				Stats: container.Stats{
					CPUStats: container.CPUStats{
						CPUUsage:    container.CPUUsage{TotalUsage: 900_000_000},
						SystemUsage: 1_000_000_000,
						OnlineCPUs:  1,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage:    container.CPUUsage{TotalUsage: 0},
						SystemUsage: 0,
					},
				},
			},
			expected: 90.00,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateCPUPercent(&tt.stats)
			if math.Abs(got-tt.expected) > 0.01 {
				t.Errorf("calculateCPUPercent() = %.2f, want %.2f", got, tt.expected)
			}
		})
	}
}

// ─── bytesToMB ────────────────────────────────────────────────────────────────

func TestBytesToMB(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected float64
	}{
		{"zero bytes", 0, 0.00},
		{"1 MB exactly", 1024 * 1024, 1.00},
		{"512 MB", 512 * 1024 * 1024, 512.00},
		{"1 GB", 1024 * 1024 * 1024, 1024.00},
		{"partial MB rounds to 2 decimals", 1_572_864, 1.50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bytesToMB(tt.input)
			if math.Abs(got-tt.expected) > 0.01 {
				t.Errorf("bytesToMB(%d) = %.2f, want %.2f", tt.input, got, tt.expected)
			}
		})
	}
}
