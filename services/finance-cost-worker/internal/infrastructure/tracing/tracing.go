// Package tracing provides OpenTelemetry tracing setup for the worker.
//
// When tracing is disabled (the default) InitTracer installs nothing: the
// global TracerProvider stays the SDK's built-in no-op, so span creation in the
// hot path costs nothing. Enabling it wires an OTLP/gRPC exporter to Jaeger and
// registers the W3C TraceContext propagator so trace context flows across the
// RMQ + gRPC boundaries.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// ShutdownFunc flushes and stops the trace provider. It is always safe to call
// (a no-op when tracing is disabled).
type ShutdownFunc func(ctx context.Context) error

// InitTracer configures the global OpenTelemetry tracer provider + propagator.
//
// When enabled is false it returns a no-op shutdown func and leaves the global
// provider untouched (the SDK default is already a no-op tracer), guaranteeing
// zero overhead in development.
func InitTracer(ctx context.Context, enabled bool, serviceName, version, endpoint string, insecure bool) (ShutdownFunc, error) {
	if !enabled {
		return func(context.Context) error { return nil }, nil
	}

	var opts []otlptracegrpc.Option
	opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	// Pin the schema URL to 1.24.0 to match the finance service. Do NOT add
	// resource.WithHost()/WithProcess() — those detectors reference a newer
	// semconv schema and emit a startup warning.
	res, err := resource.New(ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
			attribute.String("deployment.environment", "development"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
