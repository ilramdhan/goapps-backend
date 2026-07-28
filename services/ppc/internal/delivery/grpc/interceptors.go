// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ContextKey for context values.
type ContextKey string

const (
	// RequestIDKey is the context key for request ID.
	RequestIDKey ContextKey = "request_id"
	// UserIDKey is the context key for user ID.
	UserIDKey ContextKey = "user_id"
)

// RequestIDInterceptor adds a unique request ID to each request.
func RequestIDInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		var requestID string
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if ids := md.Get("x-request-id"); len(ids) > 0 {
				requestID = ids[0]
			}
		}

		if requestID == "" {
			requestID = uuid.New().String()
		}

		ctx = context.WithValue(ctx, RequestIDKey, requestID)

		header := metadata.Pairs("x-request-id", requestID)
		if err := grpc.SetHeader(ctx, header); err != nil {
			log.Debug().Err(err).Msg("Failed to set request ID header")
		}

		return handler(ctx, req)
	}
}

// TimeoutInterceptor enforces a request timeout when the caller did not set a
// deadline of its own.
func TimeoutInterceptor(timeout time.Duration) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		return handler(ctx, req)
	}
}

// TraceContextInterceptor extracts the W3C trace context propagated in the
// incoming gRPC metadata and injects it into the request context so this
// server's span becomes a child of the caller's span. No-op when tracing is
// disabled or no trace headers are present.
func TraceContextInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.TextMapCarrier(metadataCarrier(md)))
		}
		return handler(ctx, req)
	}
}

// metadataCarrier adapts gRPC metadata.MD to propagation.TextMapCarrier.
type metadataCarrier metadata.MD

// Get returns the first value for key, or "" if absent.
func (c metadataCarrier) Get(key string) string {
	vals := metadata.MD(c).Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// Set overwrites key with value.
func (c metadataCarrier) Set(key, value string) {
	metadata.MD(c).Set(key, value)
}

// Keys returns all metadata keys.
func (c metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// TracingInterceptor adds OpenTelemetry tracing. The tracer is fetched lazily
// per request so the interceptor is robust to construction ordering relative to
// tracing provider setup.
func TracingInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		tracer := otel.Tracer("ppc-service")

		methodParts := strings.Split(info.FullMethod, "/")
		methodName := info.FullMethod
		if len(methodParts) > 0 {
			methodName = methodParts[len(methodParts)-1]
		}

		ctx, span := tracer.Start(ctx, methodName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
			),
		)
		defer span.End()

		if reqID, ok := ctx.Value(RequestIDKey).(string); ok {
			span.SetAttributes(attribute.String("request.id", reqID))
		}

		resp, err := handler(ctx, req)

		if err != nil {
			span.RecordError(err)
			span.SetAttributes(attribute.String("rpc.grpc.status_code", status.Code(err).String()))
		}

		return resp, err
	}
}

// LoggingInterceptor creates a unary interceptor for request logging.
func LoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		requestID := ""
		if id, ok := ctx.Value(RequestIDKey).(string); ok {
			requestID = id
		}

		log.Info().
			Str("method", info.FullMethod).
			Str("request_id", requestID).
			Msg("gRPC request started")

		resp, err := handler(ctx, req)

		duration := time.Since(start)

		if err != nil {
			log.Error().
				Str("method", info.FullMethod).
				Str("request_id", requestID).
				Dur("duration", duration).
				Err(err).
				Msg("gRPC request failed")
		} else {
			log.Info().
				Str("method", info.FullMethod).
				Str("request_id", requestID).
				Dur("duration", duration).
				Msg("gRPC request completed")
		}

		return resp, err
	}
}

// RecoveryInterceptor creates a unary interceptor for panic recovery.
func RecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				requestID := ""
				if id, ok := ctx.Value(RequestIDKey).(string); ok {
					requestID = id
				}

				log.Error().
					Str("method", info.FullMethod).
					Str("request_id", requestID).
					Interface("panic", r).
					Msg("Panic recovered in gRPC handler")

				err = status.Error(codes.Internal, "internal server error")
			}
		}()

		return handler(ctx, req)
	}
}
