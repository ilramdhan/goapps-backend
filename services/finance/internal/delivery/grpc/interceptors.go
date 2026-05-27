// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"errors"
	"strings"
	"time"

	"buf.build/go/protovalidate"
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
	"google.golang.org/protobuf/proto"
)

// ContextKey for context values.
type ContextKey string

const (
	// RequestIDKey is the context key for request ID.
	RequestIDKey ContextKey = "request_id"
	// UserIDKey is the context key for user ID (for future auth).
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
		// Check if request ID exists in metadata
		var requestID string
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if ids := md.Get("x-request-id"); len(ids) > 0 {
				requestID = ids[0]
			}
		}

		// Generate if not present
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add to context
		ctx = context.WithValue(ctx, RequestIDKey, requestID)

		// Add to response metadata
		header := metadata.Pairs("x-request-id", requestID)
		if err := grpc.SetHeader(ctx, header); err != nil {
			log.Debug().Err(err).Msg("Failed to set request ID header")
		}

		return handler(ctx, req)
	}
}

// TimeoutInterceptor enforces request timeout.
func TimeoutInterceptor(timeout time.Duration) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Check if context already has deadline
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		return handler(ctx, req)
	}
}

// TraceContextInterceptor extracts the W3C trace context propagated in the
// incoming gRPC metadata (e.g. by the finance-cost-worker) and injects it into
// the request context. Placed before TracingInterceptor so the span this server
// starts becomes a child of the caller's span, producing a single distributed
// trace across worker -> finance. No-op when the global propagator is the
// default no-op (tracing disabled) or when no trace headers are present.
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

// metadataCarrier adapts gRPC metadata.MD to propagation.TextMapCarrier so the
// W3C TraceContext propagator can read trace headers from incoming metadata.
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

// TracingInterceptor adds OpenTelemetry tracing.
//
// The tracer is fetched lazily on every request rather than captured at
// interceptor construction. This keeps the interceptor robust to call
// ordering: if NewServer runs before tracing.NewProvider sets the global
// TracerProvider, capturing the tracer up front would freeze a noop and
// silently drop every span. Looking it up per-request avoids that gotcha.
func TracingInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		tracer := otel.Tracer("finance-service")

		// Extract method name
		methodParts := strings.Split(info.FullMethod, "/")
		methodName := info.FullMethod
		if len(methodParts) > 0 {
			methodName = methodParts[len(methodParts)-1]
		}

		// Start span
		ctx, span := tracer.Start(ctx, methodName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
			),
		)
		defer span.End()

		// Add request ID to span
		if reqID, ok := ctx.Value(RequestIDKey).(string); ok {
			span.SetAttributes(attribute.String("request.id", reqID))
		}

		// Handle request
		resp, err := handler(ctx, req)

		// Record error if any
		if err != nil {
			span.RecordError(err)
			span.SetAttributes(attribute.String("rpc.grpc.status_code", status.Code(err).String()))
		}

		return resp, err
	}
}

// ValidationInterceptor creates a unary interceptor for proto validation.
func ValidationInterceptor(validator protovalidate.Validator) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Validate request
		if msg, ok := req.(proto.Message); ok {
			if err := validator.Validate(msg); err != nil {
				log.Debug().
					Str("method", info.FullMethod).
					Err(err).
					Msg("Validation failed")

				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
		}

		return handler(ctx, req)
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

// ErrorMapper maps domain errors to gRPC status codes.
func ErrorMapper(err error) error {
	if err == nil {
		return nil
	}

	// Check for common domain errors
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	default:
		// Check error message for common patterns
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found"):
			return status.Error(codes.NotFound, err.Error())
		case strings.Contains(errMsg, "already exists"):
			return status.Error(codes.AlreadyExists, err.Error())
		case strings.Contains(errMsg, "invalid"):
			return status.Error(codes.InvalidArgument, err.Error())
		default:
			return status.Error(codes.Internal, err.Error())
		}
	}
}
