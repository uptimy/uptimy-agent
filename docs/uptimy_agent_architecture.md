# Uptimy Agent --- Detailed Architecture

This document defines the **internal architecture of the Uptimy
Agent**.\
It is intended to guide implementation and help contributors understand
how the system is structured.

The agent is a **self‑healing infrastructure watchdog** responsible for:

monitor → detect → diagnose → repair → verify

It runs locally on infrastructure nodes while optionally connecting to
the Uptimy control plane.

------------------------------------------------------------------------

# Architectural Principles

The agent must follow several core principles:

Deterministic first\
Repairs must be rule‑based and predictable.

Standalone capable\
The agent must work even if the control plane is unreachable.

Safety first\
Repairs must have guardrails and never allow arbitrary system execution.

Lightweight footprint\
The agent should run with minimal resource usage.

Extensible design\
Checks and repairs must support plugins.

Observable system\
All components must expose metrics and structured logs.

------------------------------------------------------------------------

# High-Level System Diagram

Uptimy Control Plane

• configuration bundles\
• telemetry ingestion\
• analytics\
• dashboards

⬇ gRPC control stream

Uptimy Agent (Data Plane)

• runtime supervisor\
• check scheduler\
• incident manager\
• repair engine\
• Kubernetes watcher\
• telemetry client\
• local state store\
• metrics exporter

------------------------------------------------------------------------

# Runtime Lifecycle

Agent lifecycle:

1.  start process
2.  load configuration
3.  start check scheduler
4.  connect to control plane (optional)
5.  watch infrastructure events
6.  detect incidents
7.  run repair recipes
8.  export telemetry

------------------------------------------------------------------------

# Core Modules

The agent is organized into modules.

internal/

checks/\
repair/\
incidents/\
telemetry/\
config/\
runtime/\
kubernetes/\
storage/\
metrics/

------------------------------------------------------------------------

# Runtime Supervisor

The runtime supervisor coordinates all internal modules.

Responsibilities:

• initialize components\
• manage worker goroutines\
• manage graceful shutdown\
• health monitoring of internal services

Example structure:

``` go
type Runtime struct {
    CheckEngine *CheckEngine
    IncidentManager *IncidentManager
    RepairEngine *RepairEngine
    Telemetry *TelemetryClient
}
```

------------------------------------------------------------------------

# Check Scheduler

The check scheduler executes checks on configured intervals.

Responsibilities:

• schedule check execution\
• collect check results\
• forward failures to incident manager

Execution model:

goroutine worker pool.

Example flow:

check scheduled → worker executes → result sent to incident manager

Example interface:

``` go
type Check interface {
    Name() string
    Run(ctx context.Context) CheckResult
}
```

------------------------------------------------------------------------

# Incident Manager

The incident manager converts failures into tracked incidents.

Responsibilities:

• deduplicate failures\
• create incidents\
• maintain incident lifecycle\
• trigger repair workflows

Incident states:

open\
repairing\
verifying\
resolved\
failed

Example structure:

``` go
type Incident struct {
    ID string
    Service string
    Status string
    FailureCount int
    StartedAt time.Time
}
```

------------------------------------------------------------------------

# Repair Workflow Engine

Repairs are executed as deterministic workflows.

The workflow engine is essentially a **state machine**.

Workflow states:

pending\
running\
success\
failed\
skipped

Example step structure:

``` go
type RepairStep struct {
    Name string
    Action string
    Retries int
}
```

Example workflow:

restart pod\
wait\
health check\
rollback deployment\
verify

------------------------------------------------------------------------

# Repair Action Registry

Repair actions are registered in a central registry.

This allows plugins to extend the system.

Example:

``` go
type RepairAction interface {
    Name() string
    Execute(ctx context.Context) error
}
```

Registry example:

``` go
actions.Register("restart_pod", RestartPodAction)
```

------------------------------------------------------------------------

# Kubernetes Watcher

When running inside Kubernetes the agent activates a watcher.

Responsibilities:

• monitor Kubernetes events\
• detect cluster failures\
• feed incidents to manager

Important events:

CrashLoopBackOff\
PodFailed\
NodeNotReady\
FailedScheduling\
DeploymentFailure

Deployment recommendation:

DaemonSet.

------------------------------------------------------------------------

# Configuration System

Configuration defines:

checks\
repair rules\
telemetry settings\
resource limits

Configuration sources:

local file\
control plane policy bundle

Example configuration:

``` yaml
checks:
  - type: http
    name: payment-api
    url: https://api.service
    interval: 30s

repairs:
  - rule: payment-api-down
    recipe: restart_then_rollback
```

------------------------------------------------------------------------

# Control Plane Integration

The agent connects to the control plane using **gRPC bidirectional streaming**.

Connection flow:

agent start\
authenticate\
open stream\
send telemetry\
receive config updates

Example proto service:

``` proto
service AgentControlPlane {
  rpc Connect(stream AgentMessage)
      returns (stream ServerMessage);
}
```

------------------------------------------------------------------------

# Telemetry System

Telemetry includes:

metrics\
events\
repair records\
incidents

Telemetry client responsibilities:

• batch telemetry\
• send over stream\
• buffer during disconnections

Local metrics endpoint:

/metrics

Prometheus format.

------------------------------------------------------------------------

# Local State Storage

The agent stores minimal persistent state.

Examples:

incident history\
repair history\
configuration cache\
last applied bundle

Recommended storage engines:

BoltDB\
SQLite\
BadgerDB

------------------------------------------------------------------------

# Telemetry Buffer

To survive network outages the agent buffers telemetry locally.

Example:

ring buffer capacity: 10k events

Behavior:

store events when disconnected\
retry sending periodically\
drop oldest entries if full

------------------------------------------------------------------------

# Concurrency Model

The agent heavily uses goroutines.

Major concurrent subsystems:

check workers\
repair workflows\
telemetry streaming\
Kubernetes watcher

Communication between modules should use channels.

Example:

``` go
checkResults chan CheckResult
incidentEvents chan IncidentEvent
```

------------------------------------------------------------------------

# Safety Guardrails

The repair engine must enforce allowed actions.

Allowed:

restart_pod\
restart_container\
restart_service\
rollback_deployment\
scale_replicas

Forbidden:

shell execution\
file deletion\
secret modification

Additionally enforce:

repair rate limits\
cooldown windows

------------------------------------------------------------------------

# Resource Limits

Target runtime footprint:

CPU \< 50 millicores\
RAM \< 100MB

Optimization strategies:

batch telemetry\
shared check workers\
adaptive check intervals

------------------------------------------------------------------------

# Observability

Agent must expose internal metrics.

Examples:

agent_uptime_seconds\
checks_total\
checks_failed_total\
repairs_attempted_total\
repairs_success_total\
incident_count\
repair_duration_seconds

Logs must be structured JSON.

------------------------------------------------------------------------

# Recommended Project Structure

uptimy-agent/

cmd/agent/

internal/

runtime/ checks/ repair/ plugins/ plugins/checkers/ plugins/actions/ incidents/ telemetry/ kubernetes/ config/
storage/ logging/ version/ metrics/

pkg/

client/ client/proto/

configs/

examples/

------------------------------------------------------------------------

# Deployment Models

The agent supports multiple environments.

Standalone server

Docker container

Kubernetes DaemonSet

Example Docker:

docker run uptimy/agent

Example Kubernetes:

helm install uptimy-agent uptimy/agent

------------------------------------------------------------------------

# Failure Handling

The agent must gracefully handle failures.

Examples:

control plane unavailable → operate offline

repair failure → escalate incident

check crash → restart worker

------------------------------------------------------------------------

# Long-Term Vision

The Uptimy Agent should evolve into a distributed **autonomous
infrastructure recovery system**.

Traditional monitoring tools:

observe → alert

Uptimy:

observe → detect → repair → verify

The agent becomes the **execution engine** for self‑healing
infrastructure.
