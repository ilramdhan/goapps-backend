// Package grpc provides gRPC server implementation for IAM service.
package grpc

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

var (
	// Request metrics.
	grpcRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_requests_total",
			Help: "Total number of gRPC requests.",
		},
		[]string{"method", "code"},
	)

	grpcRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_request_duration_seconds",
			Help:    "Duration of gRPC requests in seconds.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method"},
	)

	grpcRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "grpc_requests_in_flight",
			Help: "Current number of gRPC requests being processed.",
		},
	)

	// Business metrics.
	iamAuthOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iam_auth_operations_total",
			Help: "Total number of IAM authentication operations.",
		},
		[]string{"operation", "status"},
	)

	iamUserOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iam_user_operations_total",
			Help: "Total number of IAM user operations.",
		},
		[]string{"operation", "status"},
	)

	iamRoleOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iam_role_operations_total",
			Help: "Total number of IAM role operations.",
		},
		[]string{"operation", "status"},
	)

	cacheHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits.",
		},
		[]string{"cache"},
	)

	cacheMissesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses.",
		},
		[]string{"cache"},
	)
)

// MetricsInterceptor creates a Prometheus metrics interceptor.
func MetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		grpcRequestsInFlight.Inc()
		defer grpcRequestsInFlight.Dec()

		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start).Seconds()
		code := status.Code(err).String()

		grpcRequestsTotal.WithLabelValues(info.FullMethod, code).Inc()
		grpcRequestDuration.WithLabelValues(info.FullMethod).Observe(duration)

		return resp, err
	}
}

const (
	metricStatusSuccess = "success"
	metricStatusFailure = "failure"
)

// RecordAuthOperation records an authentication operation metric.
func RecordAuthOperation(operation string, success bool) {
	s := metricStatusSuccess
	if !success {
		s = metricStatusFailure
	}
	iamAuthOperationsTotal.WithLabelValues(operation, s).Inc()
}

// RecordUserOperation records a user operation metric.
func RecordUserOperation(operation string, success bool) {
	s := metricStatusSuccess
	if !success {
		s = metricStatusFailure
	}
	iamUserOperationsTotal.WithLabelValues(operation, s).Inc()
}

// RecordRoleOperation records a role operation metric.
func RecordRoleOperation(operation string, success bool) {
	s := metricStatusSuccess
	if !success {
		s = metricStatusFailure
	}
	iamRoleOperationsTotal.WithLabelValues(operation, s).Inc()
}

// RecordCacheHit records a cache hit.
func RecordCacheHit(cache string) {
	cacheHitsTotal.WithLabelValues(cache).Inc()
}

// RecordCacheMiss records a cache miss.
func RecordCacheMiss(cache string) {
	cacheMissesTotal.WithLabelValues(cache).Inc()
}
