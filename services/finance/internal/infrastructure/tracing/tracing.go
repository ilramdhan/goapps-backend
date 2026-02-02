// Package tracing provides OpenTelemetry tracing setup.
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
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

// Provider wraps the trace provider.
type Provider struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
}

// NewProvider creates a new tracing provider.
func NewProvider(ctx context.Context, cfg *config.TracingConfig, serviceName, version string) (*Provider, error) {
	if !cfg.Enabled {
		return &Provider{}, nil
	}

	// Create OTLP exporter
	var opts []otlptracegrpc.Option
	opts = append(opts, otlptracegrpc.WithEndpoint(cfg.Endpoint))
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create resource with explicit schema to avoid version mismatch
	// Using NewSchemaless for custom attributes to prevent schema conflicts
	res, err := resource.New(ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
			attribute.String("environment", "development"),
		),
		resource.WithHost(),
		resource.WithProcess(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create trace provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Set global provider
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{
		provider: tp,
		tracer:   tp.Tracer(serviceName),
	}, nil
}

// Tracer returns the tracer.
func (p *Provider) Tracer() trace.Tracer {
	if p.tracer == nil {
		return otel.Tracer("finance-service")
	}
	return p.tracer
}

// Shutdown shuts down the trace provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.provider != nil {
		return p.provider.Shutdown(ctx)
	}
	return nil
}

// StartSpan starts a new span.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer("finance-service").Start(ctx, name, opts...)
}

// SpanFromContext returns the current span from context.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// AddEvent adds an event to the current span.
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetError marks the span as error.
func SetError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
}
