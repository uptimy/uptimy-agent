# Uptimy Agent

A self-healing infrastructure watchdog that monitors, detects, and automatically repairs infrastructure failures using deterministic workflows.

## Overview

The Uptimy Agent runs on infrastructure nodes and continuously monitors the health of services. When failures are detected, it creates incidents and executes predefined repair workflows — no human intervention required for known failure modes.

**Core loop:** `monitor → detect → repair → verify`

## Features

- **Health Checks** — HTTP, TCP, CPU, memory, disk, process, and certificate checks
- **Incident Management** — Automatic deduplication, lifecycle tracking, auto-resolution
- **Deterministic Repairs** — Multi-step recipes with retries, branching, and verification
- **Safety Guardrails** — Rate limiting, cooldowns, allowed/forbidden action enforcement
- **Kubernetes Integration** — Watch cluster events (CrashLoopBackOff, PodFailed, NodeNotReady)
- **Telemetry** — Prometheus metrics, structured JSON logs, control plane streaming (optional)
- **Offline Capable** — Operates fully standalone; control plane connection is optional
- **Env-Var Expansion** — Use `${VAR}` in YAML config for secrets injection

## Quick Start

```bash
# Install (Linux/macOS)
curl -sSfL https://raw.githubusercontent.com/uptimy/uptimy-agent/main/scripts/install.sh | sudo bash

# Edit your config
sudo vi /etc/uptimy/config.yaml

# Start the agent
sudo systemctl enable --now uptimy-agent   # Linux with systemd
uptimy-agent run --config /etc/uptimy/config.yaml  # Manual / macOS
```

## Installation

### Option 1: One-Line Installer (Recommended)

The install script downloads the correct binary, creates a config, and sets up a systemd service:

```bash
curl -sSfL https://raw.githubusercontent.com/uptimy/uptimy-agent/main/scripts/install.sh | sudo bash
```

**Customise with environment variables:**

| Variable | Default | Description |
|---|---|---|
| `UPTIMY_VERSION` | `latest` | Release version to install |
| `UPTIMY_INSTALL` | `/usr/local/bin` | Binary install directory |
| `UPTIMY_CONFIG` | `/etc/uptimy` | Configuration directory |
| `UPTIMY_DATA` | `/var/lib/uptimy` | Data/state directory |
| `UPTIMY_USER` | `uptimy` | systemd service user |
| `UPTIMY_NO_SERVICE` | `0` | Set to `1` to skip systemd setup |

```bash
# Example: install a specific version to a custom path
UPTIMY_VERSION=0.3.0 UPTIMY_INSTALL=/opt/bin sudo -E bash scripts/install.sh
```

### Option 2: Docker

```bash
docker run -d \
  --name uptimy-agent \
  -v /path/to/config.yaml:/etc/uptimy/config.yaml:ro \
  -p 9090:9090 \
  uptimy/agent:latest
```

Or build locally:

```bash
make docker-build
docker run -d uptimy/agent:dev
```

### Option 3: Kubernetes (DaemonSet)

Deploy across all nodes with the included manifest:

```bash
# Apply the DaemonSet (creates namespace, RBAC, ConfigMap, DaemonSet)
kubectl apply -f deploy/kubernetes/daemonset.yaml

# Edit the ConfigMap with your checks and repair recipes
kubectl -n uptimy edit configmap uptimy-agent-config

# Restart to pick up config changes
kubectl -n uptimy rollout restart daemonset/uptimy-agent
```

The manifest includes:
- `uptimy` namespace
- ServiceAccount + ClusterRole with least-privilege RBAC
- ConfigMap with a starter config
- DaemonSet with health probes, resource limits, and security hardening

### Option 4: Build from Source

```bash
git clone https://github.com/uptimy/uptimy-agent.git
cd uptimy-agent
make build
./bin/uptimy-agent version
```

### Running as a systemd Service (Linux)

If you installed from source or a manual download, set up the service:

```bash
# Copy the binary and config
sudo install -m 0755 bin/uptimy-agent /usr/local/bin/uptimy-agent
sudo mkdir -p /etc/uptimy /var/lib/uptimy
sudo cp configs/default.yaml /etc/uptimy/config.yaml

# Create a service user
sudo useradd --system --no-create-home --shell /usr/sbin/nologin uptimy
sudo chown -R uptimy:uptimy /etc/uptimy /var/lib/uptimy

# Install and start the systemd unit
sudo cp deploy/systemd/uptimy-agent.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now uptimy-agent

# Check status and logs
sudo systemctl status uptimy-agent
sudo journalctl -fu uptimy-agent
```

### Uninstalling

```bash
sudo bash scripts/uninstall.sh
```

Set `UPTIMY_KEEP_DATA=1` to preserve state data during uninstall.

## Configuration

```yaml
checks:
  - type: http
    name: payment-api
    url: https://api.service.com/health
    interval: 30s
    timeout: 5s

repairs:
  - rule: payment-api-down
    check: payment-api
    recipe: restart_then_rollback
    max_repairs_per_hour: 3

recipes:
  - name: restart_then_rollback
    steps:
      - action: restart_pod
        retries: 2
      - action: wait
        duration: 30s
      - action: healthcheck
        check: payment-api
      - action: rollback_deployment
        on_failure_only: true
```

## Architecture

See [docs/uptimy_agent_architecture.md](docs/uptimy_agent_architecture.md) for the detailed architecture and [docs/uptimy_agent_implementation.md](docs/uptimy_agent_implementation.md) for implementation guidance.

## Examples

See the [`examples/`](examples/) directory for ready-to-use configurations:

| Example | Description |
|---------|-------------|
| [Web Service Monitoring](examples/web-service-monitoring.yaml) | HTTP health checks for APIs and Nginx with auto-restart |
| [Database Monitoring](examples/database-monitoring.yaml) | TCP checks for Postgres, Redis, MySQL |
| [Disk & Resource Monitoring](examples/disk-and-resource-monitoring.yaml) | Disk, CPU, memory checks with auto-cleanup |
| [Kubernetes Self-Healing](examples/kubernetes-self-healing.yaml) | Pod restart, rollback, and scaling |
| [Certificate Monitoring](examples/certificate-monitoring.yaml) | TLS cert expiry alerts via webhook |
| [Full-Stack Monitoring](examples/full-stack-monitoring.yaml) | Complete production config combining all check types |

Copy any example as your starting point:

```bash
sudo cp examples/web-service-monitoring.yaml /etc/uptimy/config.yaml
sudo vi /etc/uptimy/config.yaml
sudo systemctl restart uptimy-agent
```

## Project Structure

```
cmd/agent/            — CLI entrypoint
internal/
  config/             — Configuration loading and validation
  checks/             — Check engine, scheduler, registry
  incidents/          — Incident manager, lifecycle, deduplication
  repair/             — Repair engine, action registry, guardrails
  plugins/            — Extensible check and repair plugins
    checkers/         — Check implementations (HTTP, TCP, CPU, disk, …)
    actions/          — Repair action implementations (restart, rollback, …)
  telemetry/          — Metrics, telemetry buffer, exporter
  kubernetes/         — Kubernetes event watcher
  storage/            — Local state storage (BoltDB)
  runtime/            — Runtime supervisor, lifecycle management
  logging/            — Structured logging (zap)
  version/            — Build version info
  metrics/            — Prometheus metrics helpers
pkg/
  client/             — gRPC control plane client
    proto/            — Protobuf service definitions
configs/              — Default configuration files
examples/             — Example configs for common scenarios
deploy/
  systemd/            — systemd unit file
  kubernetes/         — DaemonSet manifest with RBAC
docs/                 — Architecture and implementation docs
scripts/
  install.sh          — One-line installer script
  uninstall.sh        — Uninstaller script
```

## Development

```bash
make build      # Build binary to bin/
make test       # Run tests with race detector
make lint       # Run golangci-lint
make fmt        # Format code
make coverage   # Generate coverage report
make dist       # Build release tarballs for all platforms
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).
