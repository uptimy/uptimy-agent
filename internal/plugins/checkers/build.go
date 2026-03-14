package checkers

// Package checkers provides built-in health check implementations ("plugins")
// for the Uptimy Agent check engine.
//
// To add a new check type:
//  1. Create a new file in this package (e.g. dns.go).
//  2. Implement the checks.Check interface (Name + Run).
//  3. Add a case to BuildFromConfig below to construct it from config.
//  4. Document the check type in configs/default.yaml.

import (
	"github.com/uptimy/uptimy-agent/internal/checks"
	"github.com/uptimy/uptimy-agent/internal/config"
	"go.uber.org/zap"
)

// BuildFromConfig creates Check instances from check configuration entries,
// registers them in the registry, and schedules them for execution.
// Unknown check types are logged and skipped.
func BuildFromConfig(
	checkConfigs []config.CheckConfig,
	registry *checks.Registry,
	scheduler *checks.Scheduler,
	logger *zap.SugaredLogger,
) error {
	for i := range checkConfigs {
		cc := &checkConfigs[i]
		var c checks.Check
		timeout := cc.Timeout
		if timeout == 0 {
			timeout = config.DefaultCheckTimeout()
		}

		switch cc.Type {
		case "http":
			expectedStatus := cc.ExpectedStatus
			if expectedStatus == 0 {
				expectedStatus = 200
			}
			method := cc.Method
			if method == "" {
				method = "GET"
			}
			c = NewHTTPCheck(cc.Name, cc.Service, cc.URL, method, expectedStatus, timeout, cc.Headers)

		case "tcp":
			c = NewTCPCheck(cc.Name, cc.Service, cc.Address, timeout)

		case "cpu":
			threshold := cc.Threshold
			if threshold <= 0 {
				threshold = 90.0 // Default 90%
			}
			c = NewCPUCheck(cc.Name, cc.Service, threshold, timeout)

		case "memory":
			threshold := cc.Threshold
			if threshold <= 0 {
				threshold = 90.0 // Default 90%
			}
			c = NewMemoryCheck(cc.Name, cc.Service, threshold, timeout)

		case "disk":
			threshold := cc.Threshold
			if threshold <= 0 {
				threshold = 90.0 // Default 90%
			}
			path := cc.Path
			if path == "" {
				path = "/" // Default root filesystem
			}
			c = NewDiskCheck(cc.Name, cc.Service, path, threshold, timeout)

		case "process":
			c = NewProcessCheck(cc.Name, cc.Service, cc.ProcessName, cc.ServiceName, timeout)

		case "certificate":
			daysBeforeExpiry := cc.DaysBeforeExpiry
			if daysBeforeExpiry <= 0 {
				daysBeforeExpiry = 30 // Default 30 days
			}
			c = NewCertificateCheck(cc.Name, cc.Service, cc.CertPath, cc.CertURL, daysBeforeExpiry, timeout)

		case "docker_container":
			c = NewDockerContainerCheck(cc.Name, cc.Service, cc.ContainerName, timeout)

		case "docker_swarm":
			c = NewDockerSwarmCheck(cc.Name, cc.Service, timeout)

		default:
			logger.Warnw("unknown check type, skipping", "type", cc.Type, "name", cc.Name)
			continue
		}

		if err := registry.Register(c); err != nil {
			return err
		}

		interval := cc.Interval
		if interval == 0 {
			interval = config.DefaultCheckInterval()
		}
		scheduler.AddCheck(c, interval)
		logger.Infow("registered check", "name", cc.Name, "type", cc.Type, "interval", interval)
	}
	return nil
}
