# Uptimy Agent --- Implementation Guide for Copilot

This document defines the architecture and implementation instructions
for building the Uptimy Agent, a self-healing infrastructure agent
written in Go.

The agent detects infrastructure failures, executes deterministic repair
workflows, and optionally connects to the Uptimy control plane for
telemetry, configuration, and analytics.

This document is intended to guide automated coding assistants (e.g.,
Copilot) when implementing the project.

------------------------------------------------------------------------

# Core Goals

The Uptimy Agent must:

-   Monitor infrastructure health
-   Detect incidents
-   Execute deterministic repair workflows
-   Operate standalone (offline capable)
-   Integrate with the control plane via gRPC
-   Be lightweight and safe to run on production nodes
-   Support Kubernetes and container environments
-   Expose metrics for observability

Primary concept:

monitor → detect → repair → verify

------------------------------------------------------------------------

# High-Level Architecture

Uptimy Control Plane - configuration bundles - telemetry
ingestion - analytics dashboards

gRPC stream

Uptimy Agent (Data Plane) - Check Engine - Repair Engine -
Incident Manager - Kubernetes Watcher - Telemetry
Client - Local State Store - Metrics Exporter

The agent runs as a single Go binary.

------------------------------------------------------------------------

# Check Engine

Responsible for running health checks on infrastructure.

Example checks:

-   HTTP endpoint
-   PostgreSQL connectivity
-   Redis connectivity
-   Docker container health
-   disk usage
-   CPU pressure

Example interface:

type Check interface { Name() string Run(ctx context.Context)
CheckResult }

Example result:

type CheckResult struct { Name string Status string Error error
Timestamp time.Time }

Statuses:

-   healthy
-   degraded
-   failed

------------------------------------------------------------------------

# Incident Manager

Failures create incidents instead of triggering repairs directly.

Lifecycle:

check failure → incident opened → repair recipe → verification →
resolved

Example structure:

type Incident struct { ID string Service string Status string StartedAt
time.Time ResolvedAt \*time.Time }

------------------------------------------------------------------------

# Repair Engine

Repairs use multi-step recipes.

Example recipe:

restart pod → wait 30 seconds → health check → rollback deployment →
verify

Example step structure:

type RepairStep struct { Name string Action string Retries int }

Example recipe:

recipe: restart_then_rollback

steps: - action: restart_pod - action: wait duration: 30s - action:
healthcheck check: payment-api - if_failed: action: rollback_deployment

Features:

-   retries
-   branching
-   verification
-   cooldowns

------------------------------------------------------------------------

# Safety Mechanisms

Repairs must include guardrails.

Examples:

max_repairs_per_service_per_hour = 3

restart_container cooldown: 5m

rollback_deployment cooldown: 30m

If repairs fail repeatedly:

stop automation and escalate.

------------------------------------------------------------------------

# Kubernetes Integration

Detect Kubernetes via:

KUBERNETES_SERVICE_HOST environment variable

Watch events:

-   CrashLoopBackOff
-   PodFailed
-   FailedScheduling
-   NodeNotReady
-   DeploymentFailure

Recommended deployment:

DaemonSet

------------------------------------------------------------------------

# Telemetry System

Telemetry types:

-   metrics
-   events
-   repair records

Example metrics:

uptimy_checks_total uptimy_checks_failed_total
uptimy_repairs_attempted_total uptimy_repairs_success_total
uptimy_incidents_total

Expose local metrics endpoint:

/metrics

Prometheus format.

------------------------------------------------------------------------

# Telemetry Streaming

Agents communicate with the control plane using gRPC bidirectional streaming.

Flow:

agent start → authenticate → open stream → send telemetry → receive
config updates

Example proto:

service AgentControlPlane { rpc Connect(stream AgentMessage) returns
(stream ServerMessage); }

Batch telemetry before sending.

------------------------------------------------------------------------

# Configuration System

Agents receive configuration via versioned bundles.

Example notification:

config_version: 42 bundle_url: https://cdn.uptimy.com/config/42
signature: abc123

Agent process:

receive notification → download bundle → verify signature → activate

Example configuration:

checks: - type: http url: https://api.service.com interval: 30s

repairs: - rule: payment-api-down recipe: restart_then_rollback

------------------------------------------------------------------------

# Local State Storage

Store:

-   incident history
-   repair history
-   configuration cache

Recommended:

-   BoltDB
-   SQLite
-   BadgerDB

------------------------------------------------------------------------

# Telemetry Buffer

If the control plane disconnects:

store telemetry locally retry later drop oldest when full

Example:

ring buffer size: 10k events

------------------------------------------------------------------------

# Resource Limits

Target:

CPU \< 50m RAM \< 100MB

Optimization:

-   batching
-   shared check runners
-   adaptive intervals

------------------------------------------------------------------------

# Security Model

Allowed repair actions:

restart_pod restart_container restart_service rollback_deployment
scale_replicas

Forbidden:

arbitrary shell execution delete files modify secrets

Agent should run with least privilege.

------------------------------------------------------------------------

# Observability

Metrics:

agent_uptime_seconds checks_total checks_failed_total
repairs_attempted_total repairs_success_total incident_count
repair_duration_seconds incident_resolution_time

Logs should be structured JSON.

------------------------------------------------------------------------

# CLI

Commands:

uptimy-agent run uptimy-agent init uptimy-agent
version

Example:

uptimy-agent run --config /etc/uptimy/config.yaml

------------------------------------------------------------------------

# Project Structure

uptimy-agent/

cmd/ agent/

internal/ checks/ incidents/ repair/ plugins/ plugins/checkers/ plugins/actions/ telemetry/ config/
kubernetes/ storage/ runtime/ logging/ version/ metrics/

pkg/ client/

client/proto/

configs/

------------------------------------------------------------------------

# Testing

Tests should cover:

-   unit tests for checks
-   recipe execution
-   Kubernetes simulations

Tools:

kind chaos-mesh docker-compose

Test scenarios:

-   container crash loops
-   database outages
-   node failures
-   network partitions

------------------------------------------------------------------------

# Final Vision

The Uptimy Agent should become an open-source self-healing
infrastructure watchdog.

Traditional monitoring:

monitor → alert

Uptimy:

monitor → detect → repair → verify
