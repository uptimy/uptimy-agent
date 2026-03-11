package checkers

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/uptimy/uptimy-agent/internal/checks"
)

// ProcessCheck verifies that a process or systemd service is running.
type ProcessCheck struct {
	name        string
	service     string
	processName string // Process name to check (by process list)
	serviceName string // Systemd service name to check (via systemctl)
	timeout     time.Duration
}

// NewProcessCheck creates a new process health check.
// Use processName for process list lookup, or serviceName for systemctl.
func NewProcessCheck(name, service, processName, serviceName string, timeout time.Duration) *ProcessCheck {
	return &ProcessCheck{
		name:        name,
		service:     service,
		processName: processName,
		serviceName: serviceName,
		timeout:     timeout,
	}
}

// Name returns the check's unique identifier.
func (c *ProcessCheck) Name() string { return c.name }

// Run executes the process check and returns the result.
func (c *ProcessCheck) Run(ctx context.Context) checks.CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var status checks.CheckStatus
	var err error
	var metadata map[string]string

	if c.serviceName != "" {
		status, err, metadata = c.checkSystemdService(ctx)
	} else if c.processName != "" {
		status, err, metadata = c.checkProcessByName(ctx)
	} else {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("either processName or serviceName must be specified"),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}
	}

	return checks.CheckResult{
		Name:      c.name,
		Service:   c.service,
		Status:    status,
		Error:     err,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Metadata:  metadata,
	}
}

// checkSystemdService checks if a systemd service is active.
func (c *ProcessCheck) checkSystemdService(ctx context.Context) (checks.CheckStatus, error, map[string]string) {
	metadata := map[string]string{"service_name": c.serviceName}

	// Validate service name to prevent command injection
	if err := validateServiceName(c.serviceName); err != nil {
		return checks.StatusFailed, fmt.Errorf("invalid service name: %w", err), metadata
	}

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", c.serviceName)
	output, err := cmd.Output()

	status := strings.TrimSpace(string(output))
	metadata["systemctl_status"] = status

	if err != nil {
		return checks.StatusFailed, fmt.Errorf("service %s is not active: %s", c.serviceName, status), metadata
	}

	if status == "active" {
		return checks.StatusHealthy, nil, metadata
	}

	return checks.StatusFailed, fmt.Errorf("service %s is %s", c.serviceName, status), metadata
}

// checkProcessByName checks if a process with the given name is running.
func (c *ProcessCheck) checkProcessByName(ctx context.Context) (checks.CheckStatus, error, map[string]string) {
	metadata := map[string]string{"process_name": c.processName}

	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return checks.StatusFailed, fmt.Errorf("failed to list processes: %w", err), metadata
	}

	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(name), strings.ToLower(c.processName)) {
			metadata["pid"] = fmt.Sprintf("%d", p.Pid)
			return checks.StatusHealthy, nil, metadata
		}
	}

	return checks.StatusFailed, fmt.Errorf("process %s not found", c.processName), metadata
}

// validateServiceName rejects suspicious service names.
// systemd unit names may contain [a-zA-Z0-9:._@-].
func validateServiceName(name string) error {
	for _, r := range name {
		if !isValidServiceRune(r) {
			return fmt.Errorf("invalid character %q in service name", r)
		}
	}
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid service name %q", name)
	}
	return nil
}

func isValidServiceRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '-' || r == '.' || r == ':' || r == '@'
}
