package checkers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/uptimy/uptimy-agent/internal/checks"
)

// HTTPCheck performs an HTTP request to verify endpoint health.
type HTTPCheck struct {
	name           string
	service        string
	url            string
	method         string
	expectedStatus int
	timeout        time.Duration
	headers        map[string]string
	client         *http.Client
}

// NewHTTPCheck creates a new HTTP health check.
// The timeout is enforced via context; the http.Client has no independent timeout
// to avoid confusing double-timeout error messages.
func NewHTTPCheck(name, service, url, method string, expectedStatus int, timeout time.Duration, headers map[string]string) *HTTPCheck {
	return &HTTPCheck{
		name:           name,
		service:        service,
		url:            url,
		method:         method,
		expectedStatus: expectedStatus,
		timeout:        timeout,
		headers:        headers,
		client:         &http.Client{},
	}
}

// Name returns the check's unique identifier.
func (c *HTTPCheck) Name() string { return c.name }

// Run executes the HTTP check and returns the result.
func (c *HTTPCheck) Run(ctx context.Context) checks.CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, c.method, c.url, nil)
	if err != nil {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("creating request: %w", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}
	}

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("executing request: %w", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	metadata := map[string]string{
		"status_code": fmt.Sprintf("%d", resp.StatusCode),
		"url":         c.url,
	}

	if resp.StatusCode != c.expectedStatus {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("unexpected status %d, expected %d", resp.StatusCode, c.expectedStatus),
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
