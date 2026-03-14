package config

import "time"

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Agent: AgentConfig{
			Name:           "uptimy-agent",
			WorkerPoolSize: 4,
		},
		Telemetry: TelemetryConfig{
			Enabled:     true,
			MetricsPort: 9090,
			BufferSize:  10000,
		},
		Storage: StorageConfig{
			Path: "./data/state.db",
		},
		Kubernetes: KubernetesConfig{
			Enabled: KubeAuto,
		},
		ControlPlane: ControlPlaneConfig{
			Endpoint: "grpc.coordinator.upti.my:443",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// DefaultCheckTimeout returns the default timeout for checks that don't specify one.
func DefaultCheckTimeout() time.Duration {
	return 10 * time.Second
}

// DefaultCheckInterval returns the default interval for checks that don't specify one.
func DefaultCheckInterval() time.Duration {
	return 30 * time.Second
}
