# AWS Lambda Go OpenTelemetry to Grafana Cloud

This project demonstrates a proof-of-concept for sending AWS Lambda telemetry directly to Grafana Cloud without using EC2, VM, Grafana Alloy, or a custom collector server.

The Lambda function is written in Go and uses the OpenTelemetry SDK to export traces and metrics directly to the Grafana Cloud OTLP endpoint.

## Architecture

AWS Lambda 4 на Go
        ↓
OpenTelemetry SDK
        ↓
Grafana Cloud OTLP endpoint
        ↓
Grafana Cloud Traces
Grafana Cloud Metrics
