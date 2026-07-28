// Package grpc provides gRPC server implementation for the PPC service.
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
	grpcRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ppc_grpc_requests_total",
			Help: "Total number of gRPC requests.",
		},
		[]string{"method", "code"},
	)

	grpcRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ppc_grpc_request_duration_seconds",
			Help:    "Duration of gRPC requests in seconds.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method"},
	)

	grpcRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ppc_grpc_requests_in_flight",
			Help: "Current number of gRPC requests being processed.",
		},
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
