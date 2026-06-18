// Package grpc provides gRPC server implementation for finance service.
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
	// Request metrics
	grpcRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_requests_total",
			Help: "Total number of gRPC requests",
		},
		[]string{"method", "code"},
	)

	grpcRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_request_duration_seconds",
			Help:    "Duration of gRPC requests in seconds",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method"},
	)

	grpcRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "grpc_requests_in_flight",
			Help: "Current number of gRPC requests being processed",
		},
	)

	// Business metrics
	uomOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "uom_operations_total",
			Help: "Total number of UOM operations",
		},
		[]string{"operation", "status"},
	)

	rmCategoryOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rm_category_operations_total",
			Help: "Total number of RM Category operations",
		},
		[]string{"operation", "status"},
	)

	parameterOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "parameter_operations_total",
			Help: "Total number of Parameter operations",
		},
		[]string{"operation", "status"},
	)

	formulaOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "formula_operations_total",
			Help: "Total number of Formula operations",
		},
		[]string{"operation", "status"},
	)

	uomCategoryOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "uom_category_operations_total",
			Help: "Total number of UOM Category operations",
		},
		[]string{"operation", "status"},
	)

	rmGroupOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rm_group_operations_total",
			Help: "Total number of RM Group operations",
		},
		[]string{"operation", "status"},
	)

	rmCostOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rm_cost_operations_total",
			Help: "Total number of RM Cost operations",
		},
		[]string{"operation", "status"},
	)

	mbHeadOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mb_head_operations_total",
			Help: "Total number of MB Head operations",
		},
		[]string{"operation", "status"},
	)

	mbSpinOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mb_spin_operations_total",
			Help: "Total number of MB Spin operations.",
		},
		[]string{"operation", "status"},
	)

	machineOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "machine_operations_total",
			Help: "Total number of Machine operations.",
		},
		[]string{"operation", "status"},
	)

	boxBobbinCostOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "box_bobbin_cost_operations_total",
			Help: "Total number of Box Bobbin Cost operations.",
		},
		[]string{"operation", "status"},
	)

	interminglingOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "intermingling_operations_total",
			Help: "Total number of Intermingling operations.",
		},
		[]string{"operation", "status"},
	)

	productGradeOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "product_grade_operations_total",
			Help: "Total number of Product Grade operations.",
		},
		[]string{"operation", "status"},
	)

	cacheHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"cache"},
	)

	cacheMissesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses",
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

func metricStatus(success bool) string {
	if success {
		return metricStatusSuccess
	}
	return metricStatusFailure
}

// RecordUOMOperation records a UOM operation metric.
func RecordUOMOperation(operation string, success bool) {
	uomOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}

// RecordCacheHit records a cache hit.
func RecordCacheHit(cache string) {
	cacheHitsTotal.WithLabelValues(cache).Inc()
}

// RecordCacheMiss records a cache miss.
func RecordCacheMiss(cache string) {
	cacheMissesTotal.WithLabelValues(cache).Inc()
}

// RecordMachineOperation records a Machine operation metric.
func RecordMachineOperation(operation string, success bool) {
	machineOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}

// RecordInterminglingOperation records an Intermingling operation metric.
func RecordInterminglingOperation(operation string, success bool) {
	interminglingOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}

// RecordProductGradeOperation records a Product Grade operation metric.
func RecordProductGradeOperation(operation string, success bool) {
	productGradeOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}

// RecordMBHeadOperation records an MB Head operation metric.
func RecordMBHeadOperation(operation string, success bool) {
	mbHeadOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}

// RecordMBSpinOperation records an MB Spin operation metric.
func RecordMBSpinOperation(operation string, success bool) {
	mbSpinOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}

// RecordBoxBobbinCostOperation records a Box Bobbin Cost operation metric.
func RecordBoxBobbinCostOperation(operation string, success bool) {
	boxBobbinCostOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}
