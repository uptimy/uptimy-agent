package checkers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uptimy/uptimy-agent/internal/checks"
	"github.com/uptimy/uptimy-agent/internal/plugins/checkers"
)

func TestHTTPCheck_Healthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	check := checkers.NewHTTPCheck("test-http", "test-service", server.URL, "GET", 200, 5*time.Second, nil)

	result := check.Run(context.Background())

	if result.Status != checks.StatusHealthy {
		t.Errorf("expected healthy, got %s", result.Status)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}
	if result.Name != "test-http" {
		t.Errorf("expected name test-http, got %s", result.Name)
	}
	if result.Service != "test-service" {
		t.Errorf("expected service test-service, got %s", result.Service)
	}
	if result.Metadata["status_code"] != "200" {
		t.Errorf("expected status_code 200, got %s", result.Metadata["status_code"])
	}
}

func TestHTTPCheck_Failed_WrongStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	check := checkers.NewHTTPCheck("test-http", "test-service", server.URL, "GET", 200, 5*time.Second, nil)

	result := check.Run(context.Background())

	if result.Status != checks.StatusFailed {
		t.Errorf("expected failed, got %s", result.Status)
	}
	if result.Error == nil {
		t.Error("expected error, got nil")
	}
}

func TestHTTPCheck_Failed_ConnectionRefused(t *testing.T) {
	check := checkers.NewHTTPCheck("test-http", "test-service", "http://127.0.0.1:1", "GET", 200, 2*time.Second, nil)

	result := check.Run(context.Background())

	if result.Status != checks.StatusFailed {
		t.Errorf("expected failed, got %s", result.Status)
	}
	if result.Error == nil {
		t.Error("expected error, got nil")
	}
}

func TestHTTPCheck_WithHeaders(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	headers := map[string]string{"Authorization": "Bearer test-token"}
	check := checkers.NewHTTPCheck("test-http", "test-service", server.URL, "GET", 200, 5*time.Second, headers)

	result := check.Run(context.Background())

	if result.Status != checks.StatusHealthy {
		t.Errorf("expected healthy, got %s", result.Status)
	}
	if receivedAuth != "Bearer test-token" {
		t.Errorf("expected auth header, got %q", receivedAuth)
	}
}
