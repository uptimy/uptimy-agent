package checkers

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/mem"
	"github.com/uptimy/uptimy-agent/internal/checks"
)

// MemoryCheck monitors memory usage against a threshold.
type MemoryCheck struct {
	name      string
	service   string
	threshold float64
	timeout   time.Duration
}

// NewMemoryCheck creates a new memory usage health check.
func NewMemoryCheck(name, service string, threshold float64, timeout time.Duration) *MemoryCheck {
	return &MemoryCheck{
		name:      name,
		service:   service,
		threshold: threshold,
		timeout:   timeout,
	}
}

// Name returns the check's unique identifier.
func (c *MemoryCheck) Name() string { return c.name }

// Run executes the memory check and returns the result.
func (c *MemoryCheck) Run(ctx context.Context) checks.CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	vmStat, err := mem.VirtualMemoryWithContext(ctx)
	duration := time.Since(start)

	if err != nil {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("failed to get memory stats: %w", err),
			Timestamp: time.Now(),
			Duration:  duration,
		}
	}

	usage := vmStat.UsedPercent
	metadata := map[string]string{
		"usage_percent": fmt.Sprintf("%.2f", usage),
		"threshold":     fmt.Sprintf("%.2f", c.threshold),
		"total_mb":      fmt.Sprintf("%d", vmStat.Total/1024/1024),
		"used_mb":       fmt.Sprintf("%d", vmStat.Used/1024/1024),
		"available_mb":  fmt.Sprintf("%d", vmStat.Available/1024/1024),
	}

	if usage > c.threshold {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("memory usage %.2f%% exceeds threshold %.2f%%", usage, c.threshold),
			Timestamp: time.Now(),
			Duration:  duration,
			Metadata:  metadata,
		}
	}

	return checks.CheckResult{
		Name:      c.name,
		Service:   c.service,
		Status:    checks.StatusHealthy,
		Timestamp: time.Now(),
		Duration:  duration,
		Metadata:  metadata,
	}
}
