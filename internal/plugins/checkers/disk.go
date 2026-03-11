package checkers

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/uptimy/uptimy-agent/internal/checks"
)

// DiskCheck monitors disk usage against a threshold.
type DiskCheck struct {
	name      string
	service   string
	path      string
	threshold float64
	timeout   time.Duration
}

// NewDiskCheck creates a new disk usage health check.
func NewDiskCheck(name, service, path string, threshold float64, timeout time.Duration) *DiskCheck {
	return &DiskCheck{
		name:      name,
		service:   service,
		path:      path,
		threshold: threshold,
		timeout:   timeout,
	}
}

// Name returns the check's unique identifier.
func (c *DiskCheck) Name() string { return c.name }

// Run executes the disk check and returns the result.
func (c *DiskCheck) Run(ctx context.Context) checks.CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	usage, err := disk.UsageWithContext(ctx, c.path)
	duration := time.Since(start)

	if err != nil {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("failed to get disk usage for %s: %w", c.path, err),
			Timestamp: time.Now(),
			Duration:  duration,
			Metadata:  map[string]string{"path": c.path},
		}
	}

	usagePercent := usage.UsedPercent
	metadata := map[string]string{
		"path":          c.path,
		"usage_percent": fmt.Sprintf("%.2f", usagePercent),
		"threshold":     fmt.Sprintf("%.2f", c.threshold),
		"total_gb":      fmt.Sprintf("%.2f", float64(usage.Total)/1024/1024/1024),
		"used_gb":       fmt.Sprintf("%.2f", float64(usage.Used)/1024/1024/1024),
		"free_gb":       fmt.Sprintf("%.2f", float64(usage.Free)/1024/1024/1024),
	}

	if usagePercent > c.threshold {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("disk usage %.2f%% exceeds threshold %.2f%% for path %s", usagePercent, c.threshold, c.path),
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
