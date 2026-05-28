# Architecture

## Goal

The goal of this proof-of-concept is to send AWS Lambda telemetry directly to Grafana Cloud without maintaining a separate telemetry server.

The project avoids:

- EC2 instance
- Virtual machine
- Self-hosted Grafana Alloy
- Self-hosted OpenTelemetry Collector
- CloudWatch as the main telemetry channel

## Current implementation

The Lambda function is written in Go. OpenTelemetry SDK is initialized directly inside the function code.

The function sends:

- traces to Grafana Cloud Traces / Tempo
- metrics to Grafana Cloud Metrics / Mimir

## Data flow

```text
AWS Lambda 4
    ↓
Go OpenTelemetry SDK
    ↓
Grafana Cloud OTLP endpoint
    ↓
Tempo + Metrics

Why OpenTelemetry SDK is inside the Lambda code

For traces and application-level metrics, the main application code has better context than an external extension.

The function code can create meaningful telemetry such as:

handler execution duration
business operation spans
success/error status
cold start attribute
custom counters
custom histograms

A Lambda Extension can observe platform-level events, but it cannot fully understand the internal business logic of the function.

Hybrid production approach

A realistic production architecture can combine both approaches:

Logs:
Lambda → Lambda Extension → Loki

Traces:
Lambda code → OpenTelemetry SDK → Grafana Cloud OTLP → Tempo

Metrics:
Lambda code → OpenTelemetry SDK → Grafana Cloud OTLP → Metrics/Mimir
Implemented metrics
lambda4_requests_total
lambda4_errors_total
lambda4_cold_starts_total
lambda4_duration_ms
lambda4_simulated_work_ms
Implemented trace span
lambda4_go_demo_handler
Grafana Cloud datasources

In Grafana Cloud, datasources may not be named exactly Tempo or Prometheus.

In this project, they were visible as:

grafanacloud-...-traces
grafanacloud-...-prom

The traces datasource is backed by Tempo, and the metrics datasource is Prometheus-compatible.

Summary

This proof-of-concept confirms that AWS Lambda can send traces and metrics directly to Grafana Cloud through the OTLP endpoint.

The final architecture keeps the project serverless and does not require EC2, VM, Alloy, or a separate Collector.
