// Package grpc provides gRPC server implementation for IAM service.
package grpc

import (
	"context"
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

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/jwt"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
	redisinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/redis"
)

// Server represents the gRPC server.
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	config     *config.ServerConfig
	db         *postgres.DB
}

// NewServer creates a new gRPC server with all interceptors.
func NewServer(cfg *config.ServerConfig, db *postgres.DB, jwtService *jwt.Service, sessionCache *redisinfra.SessionCache, sessionRepo session.Repository) (*Server, error) {
	// Create rate limiter
	rateLimiter := NewRateLimiter(100) // 100 requests per second default

	// Chain interceptors in order (outermost first)
	unaryChain := grpc.ChainUnaryInterceptor(
		StructuredErrorInterceptor(),                           // 0. Wrap gRPC errors into BaseResponse
		RecoveryInterceptor(),                                  // 1. Recover from panics first
		RequestIDInterceptor(),                                 // 2. Add request ID
		TracingInterceptor(),                                   // 3. Add tracing span
		MetricsInterceptor(),                                   // 4. Record metrics
		RateLimitInterceptor(rateLimiter),                      // 5. Rate limiting
		AuthInterceptor(jwtService, sessionCache, sessionRepo), // 6. JWT authentication + activity tracking
		PermissionInterceptor(),                                // 7. RBAC permission check
		LoggingInterceptor(),                                   // 8. Log request
		TimeoutInterceptor(30*time.Second),                     // 9. Enforce timeout
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
	healthServer.SetServingStatus("iam.v1.AuthService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("iam.v1.UserService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("iam.v1.RoleService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("iam.v1.PermissionService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("iam.v1.MenuService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("iam.v1.SessionService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("iam.v1.AuditService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("iam.v1.CompanyService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("iam.v1.DivisionService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("iam.v1.DepartmentService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("iam.v1.SectionService", grpc_health_v1.HealthCheckResponse_SERVING)

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

	// Use ListenConfig for context-aware listening
	lc := net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp", addr)
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
