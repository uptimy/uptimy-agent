package checkers

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/uptimy/uptimy-agent/internal/checks"
)

// CPUCheck monitors CPU usage against a threshold.
type CPUCheck struct {
	name      string
	service   string
	threshold float64
	timeout   time.Duration
}

// NewCPUCheck creates a new CPU usage health check.
func NewCPUCheck(name, service string, threshold float64, timeout time.Duration) *CPUCheck {
	return &CPUCheck{
		name:      name,
		service:   service,
		threshold: threshold,
		timeout:   timeout,
	}
}

// Name returns the check's unique identifier.
func (c *CPUCheck) Name() string { return c.name }

// Run executes the CPU check and returns the result.
func (c *CPUCheck) Run(ctx context.Context) checks.CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Get CPU usage over 1 second
	cpuPercent, err := cpu.PercentWithContext(ctx, time.Second, false)
	duration := time.Since(start)

	if err != nil || len(cpuPercent) == 0 {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("failed to get CPU stats: %w", err),
			Timestamp: time.Now(),
			Duration:  duration,
		}
	}

	usage := cpuPercent[0]
	metadata := map[string]string{
		"usage_percent": fmt.Sprintf("%.2f", usage),
		"threshold":     fmt.Sprintf("%.2f", c.threshold),
	}

	if usage > c.threshold {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("CPU usage %.2f%% exceeds threshold %.2f%%", usage, c.threshold),
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
