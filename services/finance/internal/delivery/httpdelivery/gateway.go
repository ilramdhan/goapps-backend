// Package httpdelivery provides HTTP server for gateway and Swagger.
package httpdelivery

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

//go:embed swagger.json
var swaggerJSON []byte

// Server represents the HTTP server.
type Server struct {
	server         *http.Server
	grpcTarget     string
	config         *config.ServerConfig
	allowedOrigins []string
	corsMaxAge     int
}

// NewServer creates a new HTTP server.
func NewServer(cfg *config.ServerConfig, opts ...Option) *Server {
	s := &Server{
		config:         cfg,
		grpcTarget:     fmt.Sprintf("localhost:%d", cfg.GRPCPort),
		allowedOrigins: []string{"http://localhost:3000"},
		corsMaxAge:     300,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Option configures the HTTP server.
type Option func(*Server)

// WithCORS sets CORS allowed origins and max age.
func WithCORS(origins []string, maxAge int) Option {
	return func(s *Server) {
		if len(origins) > 0 {
			s.allowedOrigins = origins
		}
		if maxAge > 0 {
			s.corsMaxAge = maxAge
		}
	}
}

// Start starts the HTTP server.
func (s *Server) Start(ctx context.Context) error {
	// Create gRPC-Gateway mux
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				UseProtoNames:   true,
				EmitUnpopulated: true,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}),
		runtime.WithErrorHandler(baseResponseErrorHandler),
	)

	// Connect to gRPC server
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	if err := financev1.RegisterUOMServiceHandlerFromEndpoint(ctx, gwMux, s.grpcTarget, opts); err != nil {
		return fmt.Errorf("failed to register UOM gateway: %w", err)
	}

	if err := financev1.RegisterRMCategoryServiceHandlerFromEndpoint(ctx, gwMux, s.grpcTarget, opts); err != nil {
		return fmt.Errorf("failed to register RMCategory gateway: %w", err)
	}

	// Create main mux
	mux := http.NewServeMux()

	// API routes (gRPC-Gateway)
	mux.Handle("/api/", gwMux)

	// Health check endpoints
	mux.HandleFunc("/healthz", s.healthHandler)
	mux.HandleFunc("/readyz", s.readyHandler)
	mux.HandleFunc("/livez", s.liveHandler)

	// Metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Swagger UI
	mux.HandleFunc("/swagger/", s.swaggerHandler)
	mux.HandleFunc("/swagger.json", s.swaggerJSONHandler)

	// CORS middleware (configurable for SSO multi-app support)
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   s.allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-Requested-With"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           s.corsMaxAge,
	}).Handler(mux)

	// Create server
	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.HTTPPort),
		Handler:      corsHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Info().
		Int("port", s.config.HTTPPort).
		Msg("HTTP server starting")

	return s.server.ListenAndServe()
}

// Stop stops the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// Health handlers
func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"healthy"}`)); err != nil {
		log.Warn().Err(err).Msg("Failed to write health response")
	}
}

func (s *Server) readyHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"ready"}`)); err != nil {
		log.Warn().Err(err).Msg("Failed to write ready response")
	}
}

func (s *Server) liveHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"live"}`)); err != nil {
		log.Warn().Err(err).Msg("Failed to write live response")
	}
}

// Swagger handlers
func (s *Server) swaggerHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(swaggerUIHTML)); err != nil {
		log.Warn().Err(err).Msg("Failed to write swagger UI response")
	}
}

func (s *Server) swaggerJSONHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(swaggerJSON); err != nil {
		log.Warn().Err(err).Msg("Failed to write swagger JSON response")
	}
}

// grpcCodeToHTTP maps gRPC status codes to HTTP status codes.
func grpcCodeToHTTP(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// baseResponseErrorHandler wraps gRPC errors into the standard BaseResponse JSON format.
func baseResponseErrorHandler(_ context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, _ *http.Request, err error) {
	st := status.Convert(err)
	httpCode := grpcCodeToHTTP(st.Code())

	resp := map[string]any{
		"base": map[string]any{
			"is_success":        false,
			"status_code":       fmt.Sprintf("%d", httpCode),
			"message":           st.Message(),
			"validation_errors": []any{},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		log.Warn().Err(encErr).Msg("Failed to write error response")
	}
}

const swaggerUIHTML = `<!DOCTYPE html>
<html>
<head>
    <title>Finance Service API</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" />
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: "/swagger.json",
                dom_id: '#swagger-ui',
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIBundle.SwaggerUIStandalonePreset
                ],
                layout: "BaseLayout",
                persistAuthorization: true
            });
        };
    </script>
</body>
</html>`
