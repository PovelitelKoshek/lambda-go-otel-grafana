package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Event map[string]interface{}

type Response struct {
	Message         string  `json:"message"`
	Status          string  `json:"status"`
	DurationMS      float64 `json:"duration_ms"`
	SimulatedWorkMS int     `json:"simulated_work_ms"`
	ColdStart       bool    `json:"cold_start"`
}

var (
	tracer = otel.Tracer("lambda-4-go-tracer")
	meter  = otel.Meter("lambda-4-go-meter")

	requestCounter        otelmetric.Int64Counter
	errorCounter          otelmetric.Int64Counter
	coldStartCounter      otelmetric.Int64Counter
	durationHistogram     otelmetric.Float64Histogram
	workDurationHistogram otelmetric.Float64Histogram

	traceProvider  *sdktrace.TracerProvider
	metricProvider *sdkmetric.MeterProvider

	coldStart = true
)

func initOpenTelemetry(ctx context.Context) error {
	serviceName := getenv("OTEL_SERVICE_NAME", "lambda-4-go-otel-grafana")

	res, err := sdkresource.New(
		ctx,
		sdkresource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("service.namespace", "aws-lambda-grafana"),
			attribute.String("deployment.environment", "poc"),
			attribute.String("telemetry.source", "aws-lambda-go"),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	traceProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(traceProvider)

	metricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}

	metricProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(
				metricExporter,
				sdkmetric.WithInterval(5*time.Second),
			),
		),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(metricProvider)

	meter = otel.Meter("lambda-4-go-meter")
	tracer = otel.Tracer("lambda-4-go-tracer")

	requestCounter, err = meter.Int64Counter(
		"lambda4_requests_total",
		otelmetric.WithDescription("Total Lambda 4 requests"),
	)
	if err != nil {
		return err
	}

	errorCounter, err = meter.Int64Counter(
		"lambda4_errors_total",
		otelmetric.WithDescription("Total Lambda 4 errors"),
	)
	if err != nil {
		return err
	}

	coldStartCounter, err = meter.Int64Counter(
		"lambda4_cold_starts_total",
		otelmetric.WithDescription("Total Lambda 4 cold starts"),
	)
	if err != nil {
		return err
	}

	durationHistogram, err = meter.Float64Histogram(
		"lambda4_duration_ms",
		otelmetric.WithDescription("Lambda 4 handler duration in milliseconds"),
		otelmetric.WithUnit("ms"),
	)
	if err != nil {
		return err
	}

	workDurationHistogram, err = meter.Float64Histogram(
		"lambda4_simulated_work_ms",
		otelmetric.WithDescription("Simulated Lambda 4 work duration in milliseconds"),
		otelmetric.WithUnit("ms"),
	)
	if err != nil {
		return err
	}

	return nil
}

func handler(ctx context.Context, event Event) (Response, error) {
	start := time.Now()
	isColdStart := coldStart

	ctx, span := tracer.Start(ctx, "lambda4_go_demo_handler")
	defer span.End()

	baseMetricAttrs := []attribute.KeyValue{
		attribute.String("function", "lambda-4-go-otel-grafana"),
		attribute.String("runtime", "go"),
		attribute.String("project", "lambda-grafana-direct-otlp"),
	}

	span.SetAttributes(
		attribute.Bool("lambda.cold_start", isColdStart),
		attribute.String("lambda.runtime", "go"),
		attribute.String("project", "lambda-grafana-direct-otlp"),
	)

	eventJSON, _ := json.Marshal(event)
	log.Printf("Lambda 4 Go started. Event: %s", string(eventJSON))

	if isColdStart {
		coldStartCounter.Add(
			ctx,
			1,
			otelmetric.WithAttributes(baseMetricAttrs...),
		)
	}

	requestCounter.Add(
		ctx,
		1,
		otelmetric.WithAttributes(
			append(baseMetricAttrs, attribute.String("result", "started"))...,
		),
	)

	sleepMS := rand.Intn(700) + 100
	time.Sleep(time.Duration(sleepMS) * time.Millisecond)

	workDurationHistogram.Record(
		ctx,
		float64(sleepMS),
		otelmetric.WithAttributes(baseMetricAttrs...),
	)

	if failValue, ok := event["fail"].(bool); ok && failValue {
		err := fmt.Errorf("intentional test error from Lambda 4 Go")

		errorCounter.Add(
			ctx,
			1,
			otelmetric.WithAttributes(
				append(baseMetricAttrs, attribute.String("error_type", "intentional_test_error"))...,
			),
		)

		durationMS := float64(time.Since(start).Microseconds()) / 1000.0

		durationHistogram.Record(
			ctx,
			durationMS,
			otelmetric.WithAttributes(
				append(baseMetricAttrs, attribute.String("result", "error"))...,
			),
		)

		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(
			attribute.Bool("lambda.error", true),
			attribute.Float64("lambda.duration_ms", durationMS),
			attribute.Int("lambda.simulated_work_ms", sleepMS),
		)

		log.Printf("Lambda 4 Go finished with error. Duration: %.2f ms, Work: %d ms", durationMS, sleepMS)

		forceFlush(ctx)

		return Response{}, err
	}

	durationMS := float64(time.Since(start).Microseconds()) / 1000.0

	durationHistogram.Record(
		ctx,
		durationMS,
		otelmetric.WithAttributes(
			append(baseMetricAttrs, attribute.String("result", "success"))...,
		),
	)

	span.SetAttributes(
		attribute.Float64("lambda.duration_ms", durationMS),
		attribute.Int("lambda.simulated_work_ms", sleepMS),
		attribute.Bool("lambda.error", false),
	)

	coldStart = false

	log.Printf("Lambda 4 Go finished successfully. Duration: %.2f ms, Work: %d ms", durationMS, sleepMS)

	forceFlush(ctx)

	return Response{
		Message:         "Hello from Lambda 4 Go with OpenTelemetry metrics and traces",
		Status:          "success",
		DurationMS:      durationMS,
		SimulatedWorkMS: sleepMS,
		ColdStart:       isColdStart,
	}, nil
}

func forceFlush(ctx context.Context) {
	flushCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if traceProvider != nil {
		_ = traceProvider.ForceFlush(flushCtx)
	}

	if metricProvider != nil {
		_ = metricProvider.ForceFlush(flushCtx)
	}
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func main() {
	ctx := context.Background()

	if err := initOpenTelemetry(ctx); err != nil {
		log.Fatalf("failed to initialize OpenTelemetry: %v", err)
	}

	lambda.Start(handler)
}
