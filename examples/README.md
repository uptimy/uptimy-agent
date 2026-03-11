# Uptimy Agent — Example Configurations

Ready-to-use configurations for common self-healing scenarios. Copy one, edit it for your environment, and start the agent.

## Quick Start

```bash
# Install the agent
curl -sSfL https://raw.githubusercontent.com/uptimy/uptimy-agent/main/scripts/install.sh | sudo bash

# Copy an example as your starting config
sudo cp examples/web-service-monitoring.yaml /etc/uptimy/config.yaml

# Edit for your environment
sudo vi /etc/uptimy/config.yaml

# Start the agent
sudo systemctl enable --now uptimy-agent
```

## Examples

| File | Use Case | Check Types | Actions |
|------|----------|-------------|---------|
| [web-service-monitoring.yaml](web-service-monitoring.yaml) | Web API + Nginx monitoring | HTTP, Process | restart_container, restart_service, webhook |
| [database-monitoring.yaml](database-monitoring.yaml) | Postgres, Redis, MySQL | TCP | restart_service, restart_container, webhook |
| [disk-and-resource-monitoring.yaml](disk-and-resource-monitoring.yaml) | Disk, CPU, memory management | Disk, CPU, Memory | clear_temp, rotate_logs, webhook |
| [kubernetes-self-healing.yaml](kubernetes-self-healing.yaml) | K8s microservices healing | HTTP, TCP | restart_pod, rollback_deployment, scale_replicas |
| [certificate-monitoring.yaml](certificate-monitoring.yaml) | TLS/SSL cert expiry alerts | Certificate | webhook |
| [full-stack-monitoring.yaml](full-stack-monitoring.yaml) | Complete production setup | All types | All actions |

## Available Check Types

| Type | Description | Key Parameters |
|------|-------------|----------------|
| `http` | HTTP/HTTPS endpoint check | `url`, `method`, `expected_status`, `headers` |
| `tcp` | TCP port connectivity | `address` (host:port) |
| `process` | System process alive check | `service_name` |
| `cpu` | CPU usage threshold | `threshold` (percent) |
| `memory` | Memory usage threshold | `threshold` (percent) |
| `disk` | Disk usage threshold | `path`, `threshold` (percent) |
| `certificate` | TLS certificate expiry | `cert_url` or `cert_path`, `days_before_expiry` |

## Available Repair Actions

| Action | Description | Params |
|--------|-------------|--------|
| `restart_pod` | Delete and restart a Kubernetes pod | `pod`, `namespace`, `grace_period` |
| `restart_container` | Restart a Docker container | `container` |
| `restart_service` | Restart a systemd service | `service` |
| `start_service` | Start a stopped systemd service | `service` |
| `stop_service` | Stop a systemd service | `service` |
| `rollback_deployment` | Roll back K8s deployment to previous revision | `deployment`, `namespace` |
| `scale_replicas` | Scale K8s deployment replicas | `deployment`, `namespace`, `replicas` |
| `clear_temp` | Remove old files from a directory | `path`, `age` |
| `rotate_logs` | Rotate/truncate log files | `path`, `max_size` |
| `webhook` | Send HTTP webhook notification | `url`, `method` |
| `wait` | Pause between steps | `duration` |
| `healthcheck` | Re-run a check to verify repair | `check` |

## Tips

- **Use `${ENV_VAR}` in YAML** for secrets like webhook URLs and tokens
- **Set `max_repairs_per_hour`** to prevent repair loops — start low (2-3), increase as you gain confidence
- **Use `on_failure_only: true`** on steps that should only run if a previous step failed (e.g. rollback, escalation alerts)
- **Combine recipes:** start with a lightweight fix (restart), then escalate (rollback/scale), then alert (webhook)
- **Test first:** run the agent with `uptimy-agent run --config your-config.yaml` in the foreground to verify checks pass before deploying as a service
