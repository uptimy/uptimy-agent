package checkers

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/uptimy/uptimy-agent/internal/checks"
)

// TCPCheck verifies that a TCP endpoint is accepting connections.
type TCPCheck struct {
	name    string
	service string
	address string
	timeout time.Duration
}

// NewTCPCheck creates a new TCP connectivity check.
func NewTCPCheck(name, service, address string, timeout time.Duration) *TCPCheck {
	return &TCPCheck{
		name:    name,
		service: service,
		address: address,
		timeout: timeout,
	}
}

// Name returns the check's unique identifier.
func (c *TCPCheck) Name() string { return c.name }

// Run executes the TCP check and returns the result.
func (c *TCPCheck) Run(ctx context.Context) checks.CheckResult {
	start := time.Now()

	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", c.address)
	duration := time.Since(start)

	if err != nil {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("tcp dial failed: %w", err),
			Timestamp: time.Now(),
			Duration:  duration,
			Metadata:  map[string]string{"address": c.address},
		}
	}
	conn.Close()

	return checks.CheckResult{
		Name:      c.name,
		Service:   c.service,
		Status:    checks.StatusHealthy,
		Timestamp: time.Now(),
		Duration:  duration,
		Metadata:  map[string]string{"address": c.address},
	}
}
