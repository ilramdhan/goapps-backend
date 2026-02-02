// Package grpc provides gRPC server implementation.
package grpc

import (
	"fmt"
	"net"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip" // Register gzip compressor
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// Server represents the gRPC server.
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	config     *config.ServerConfig
	db         *postgres.DB
}

// NewServer creates a new gRPC server with all interceptors.
func NewServer(cfg *config.ServerConfig, db *postgres.DB) (*Server, error) {
	// Create rate limiter
	rateLimiter := NewRateLimiter(100) // 100 requests per second default

	// Chain interceptors in order
	// Note: Validation is now handled at handler level to return proper BaseResponse
	unaryChain := grpc.ChainUnaryInterceptor(
		RecoveryInterceptor(),              // 1. Recover from panics first
		RequestIDInterceptor(),             // 2. Add request ID
		TracingInterceptor(),               // 3. Add tracing span
		MetricsInterceptor(),               // 4. Record metrics
		RateLimitInterceptor(rateLimiter),  // 5. Rate limiting
		LoggingInterceptor(),               // 6. Log request
		TimeoutInterceptor(30*time.Second), // 7. Enforce timeout
		// ValidationInterceptor removed - now handled in handler for proper BaseResponse format
	)

	// Server options
	opts := []grpc.ServerOption{
		unaryChain,
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Minute,
			MaxConnectionAge:      30 * time.Minute,
			MaxConnectionAgeGrace: 5 * time.Minute,
			Time:                  5 * time.Minute,
			Timeout:               1 * time.Minute,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             1 * time.Minute,
			PermitWithoutStream: true,
		}),
		grpc.MaxRecvMsgSize(10 * 1024 * 1024), // 10MB for file uploads
		grpc.MaxSendMsgSize(10 * 1024 * 1024), // 10MB for file downloads
	}

	// Create server
	grpcServer := grpc.NewServer(opts...)

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("finance.v1.UOMService", grpc_health_v1.HealthCheckResponse_SERVING)

	// Enable reflection for development
	reflection.Register(grpcServer)

	return &Server{
		grpcServer: grpcServer,
		config:     cfg,
		db:         db,
	}, nil
}

// GRPCServer returns the underlying gRPC server for service registration.
func (s *Server) GRPCServer() *grpc.Server {
	return s.grpcServer
}

// Start starts the gRPC server.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.config.GRPCPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	s.listener = listener

	log.Info().
		Int("port", s.config.GRPCPort).
		Str("address", addr).
		Msg("gRPC server starting")

	return s.grpcServer.Serve(listener)
}

// Stop stops the gRPC server gracefully.
func (s *Server) Stop() {
	log.Info().Msg("gRPC server stopping...")
	s.grpcServer.GracefulStop()
	log.Info().Msg("gRPC server stopped")
}
