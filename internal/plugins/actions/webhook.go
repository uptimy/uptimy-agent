package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// WebhookAction calls a webhook URL for external notifications or integrations.
type WebhookAction struct {
	logger *zap.SugaredLogger
	client *http.Client
}

func NewWebhookAction(logger *zap.SugaredLogger) *WebhookAction {
	return &WebhookAction{
		logger: logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

func (a *WebhookAction) Name() string { return "webhook" }

func (a *WebhookAction) Execute(ctx context.Context, params map[string]string) error {
	webhookURL := params["url"]
	if webhookURL == "" {
		return fmt.Errorf("webhook: 'url' parameter is required")
	}

	// Validate URL
	if err := a.validateURL(webhookURL); err != nil {
		return fmt.Errorf("webhook: %w", err)
	}

	method := params["method"]
	if method == "" {
		method = "POST"
	}

	// Parse timeout
	timeout := 30 * time.Second
	if v, ok := params["timeout"]; ok {
		if parsed, err := time.ParseDuration(v); err == nil {
			timeout = parsed
		}
	}

	// Build request body
	var body io.Reader
	if payload, ok := params["payload"]; ok && payload != "" {
		body = strings.NewReader(payload)
	} else {
		// Build JSON payload from params
		data := make(map[string]string)
		for k, v := range params {
			if k != "url" && k != "method" && k != "timeout" {
				data[k] = v
			}
		}
		if len(data) > 0 {
			jsonData, _ := json.Marshal(data)
			body = bytes.NewReader(jsonData)
		}
	}

	a.logger.Infow("calling webhook", "url", webhookURL, "method", method)

	// Create request with timeout context
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, webhookURL, body)
	if err != nil {
		return fmt.Errorf("webhook: failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "uptimy-agent/1.0")

	// Add custom headers from params
	if headers, ok := params["headers"]; ok {
		var headerMap map[string]string
		if err := json.Unmarshal([]byte(headers), &headerMap); err == nil {
			for k, v := range headerMap {
				req.Header.Set(k, v)
			}
		}
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook: received status %d: %s", resp.StatusCode, string(respBody))
	}

	a.logger.Infow("webhook called successfully",
		"url", webhookURL,
		"status", resp.StatusCode,
	)

	return nil
}

// validateURL ensures the URL is well-formed and uses https or http.
func (a *WebhookAction) validateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL must use http or https scheme")
	}

	if parsed.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}
