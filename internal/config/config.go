// Package config handles loading, validating, and providing
// agent configuration from YAML files and defaults.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level agent configuration.
type Config struct {
	Agent        AgentConfig        `yaml:"agent"`
	Checks       []CheckConfig      `yaml:"checks"`
	Repairs      []RepairRuleConfig `yaml:"repairs"`
	Recipes      []RecipeConfig     `yaml:"recipes"`
	Telemetry    TelemetryConfig    `yaml:"telemetry"`
	Storage      StorageConfig      `yaml:"storage"`
	Kubernetes   KubernetesConfig   `yaml:"kubernetes"`
	ControlPlane ControlPlaneConfig `yaml:"control_plane"`
	Logging      LoggingConfig      `yaml:"logging"`
}

// AgentConfig holds general agent settings.
type AgentConfig struct {
	Name           string `yaml:"name"`
	WorkerPoolSize int    `yaml:"worker_pool_size"`
}

// CheckConfig defines a health check from configuration.
type CheckConfig struct {
	Name           string            `yaml:"name"`
	Type           string            `yaml:"type"`
	Service        string            `yaml:"service"`
	URL            string            `yaml:"url,omitempty"`
	Method         string            `yaml:"method,omitempty"`
	ExpectedStatus int               `yaml:"expected_status,omitempty"`
	Address        string            `yaml:"address,omitempty"`
	Headers        map[string]string `yaml:"headers,omitempty"`
	Interval       time.Duration     `yaml:"interval"`
	Timeout        time.Duration     `yaml:"timeout"`

	// CPU/Memory/Disk check parameters
	Threshold float64 `yaml:"threshold,omitempty"` // Percentage threshold (0-100)
	Path      string  `yaml:"path,omitempty"`      // Disk path or certificate file path

	// Process check parameters
	ProcessName string `yaml:"process_name,omitempty"` // Process name to check
	ServiceName string `yaml:"service_name,omitempty"` // Systemd service name to check

	// Certificate check parameters
	CertURL          string `yaml:"cert_url,omitempty"`           // URL to check certificate from
	CertPath         string `yaml:"cert_path,omitempty"`          // Path to certificate file
	DaysBeforeExpiry int    `yaml:"days_before_expiry,omitempty"` // Alert if expiring within this many days
}

// RepairRuleConfig maps a check to a repair recipe.
type RepairRuleConfig struct {
	Rule              string `yaml:"rule"`
	Check             string `yaml:"check"`
	Recipe            string `yaml:"recipe"`
	MaxRepairsPerHour int    `yaml:"max_repairs_per_hour"`
}

// RecipeConfig defines a repair recipe.
type RecipeConfig struct {
	Name  string             `yaml:"name"`
	Steps []RecipeStepConfig `yaml:"steps"`
}

// RecipeStepConfig is a single step in a recipe.
type RecipeStepConfig struct {
	Action        string            `yaml:"action"`
	Retries       int               `yaml:"retries,omitempty"`
	Timeout       time.Duration     `yaml:"timeout,omitempty"`
	OnFailureOnly bool              `yaml:"on_failure_only,omitempty"`
	Params        map[string]string `yaml:"params,omitempty"`
	// Convenience fields for common actions.
	Duration time.Duration `yaml:"duration,omitempty"`
	Check    string        `yaml:"check,omitempty"`
}

// TelemetryConfig holds telemetry/metrics settings.
type TelemetryConfig struct {
	Enabled     bool `yaml:"enabled"`
	MetricsPort int  `yaml:"metrics_port"`
	BufferSize  int  `yaml:"buffer_size"`
}

// StorageConfig holds local storage settings.
type StorageConfig struct {
	Path string `yaml:"path"`
}

// KubernetesEnabled represents the tri-state for Kubernetes integration.
type KubernetesEnabled string

// KubernetesEnabled values.
const (
	KubeAuto     KubernetesEnabled = "auto"
	KubeEnabled  KubernetesEnabled = "true"
	KubeDisabled KubernetesEnabled = "false"
)

// KubernetesConfig holds K8s integration settings.
type KubernetesConfig struct {
	Enabled   KubernetesEnabled `yaml:"enabled"` // "auto", "true", "false"
	Namespace string            `yaml:"namespace,omitempty"`
}

// ControlPlaneConfig holds control plane connection settings.
type ControlPlaneConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint,omitempty"`
	Token    string `yaml:"token,omitempty"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// LoadFromFile reads a YAML config file, merges with defaults, and validates.
// Environment variables in the form ${VAR} or $VAR are expanded in the raw
// YAML before parsing. This is useful for injecting secrets (e.g. control_plane.token).
func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Expand environment variables before unmarshaling.
	expanded := os.ExpandEnv(string(data))

	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks the configuration for logical errors.
func (c *Config) Validate() error {
	if c.Agent.Name == "" {
		return fmt.Errorf("agent.name must not be empty")
	}
	if c.Agent.WorkerPoolSize < 1 {
		return fmt.Errorf("agent.worker_pool_size must be >= 1")
	}

	// Storage validation.
	if c.Storage.Path == "" {
		return fmt.Errorf("storage.path must not be empty")
	}

	// Index check names for cross-reference validation.
	checkNames := make(map[string]bool, len(c.Checks))
	for i := range c.Checks {
		ch := &c.Checks[i]
		if ch.Name == "" {
			return fmt.Errorf("check name must not be empty")
		}
		if ch.Type == "" {
			return fmt.Errorf("check %q: type must not be empty", ch.Name)
		}
		if ch.Interval <= 0 {
			return fmt.Errorf("check %q: interval must be greater than zero", ch.Name)
		}
		if ch.Timeout < 0 {
			return fmt.Errorf("check %q: timeout must not be negative", ch.Name)
		}
		checkNames[ch.Name] = true
	}

	// Index recipe names and validate steps.
	recipeNames := make(map[string]bool, len(c.Recipes))
	for _, r := range c.Recipes {
		if r.Name == "" {
			return fmt.Errorf("recipe name must not be empty")
		}
		for i, step := range r.Steps {
			if step.Action == "" {
				return fmt.Errorf("recipe %q step %d: action must not be empty", r.Name, i)
			}
		}
		recipeNames[r.Name] = true
	}

	// Validate repair rules cross-references.
	for _, rule := range c.Repairs {
		if rule.Rule == "" {
			return fmt.Errorf("repair rule name must not be empty")
		}
		if rule.Check == "" {
			return fmt.Errorf("repair rule %q: check must not be empty", rule.Rule)
		}
		if rule.Recipe == "" {
			return fmt.Errorf("repair rule %q: recipe must not be empty", rule.Rule)
		}
		if len(c.Checks) > 0 && !checkNames[rule.Check] {
			return fmt.Errorf("repair rule %q references unknown check %q", rule.Rule, rule.Check)
		}
		if len(c.Recipes) > 0 && !recipeNames[rule.Recipe] {
			return fmt.Errorf("repair rule %q references unknown recipe %q", rule.Rule, rule.Recipe)
		}
	}

	// Validate telemetry.
	if c.Telemetry.Enabled && c.Telemetry.MetricsPort < 1 {
		return fmt.Errorf("telemetry.metrics_port must be a valid port when telemetry is enabled")
	}
	if c.Telemetry.BufferSize < 1 {
		c.Telemetry.BufferSize = 1000
	}

	// Validate Kubernetes enabled value.
	switch c.Kubernetes.Enabled {
	case KubeAuto, KubeEnabled, KubeDisabled:
		// ok
	default:
		return fmt.Errorf("kubernetes.enabled must be one of: auto, true, false (got %q)", c.Kubernetes.Enabled)
	}

	// Validate control plane config.
	if c.ControlPlane.Enabled && c.ControlPlane.Endpoint == "" {
		return fmt.Errorf("control_plane.endpoint must not be empty when control_plane is enabled")
	}

	return nil
}
