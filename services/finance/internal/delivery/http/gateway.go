// Package http provides HTTP server for gateway and Swagger.
package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

// Server represents the HTTP server.
type Server struct {
	server     *http.Server
	grpcTarget string
	config     *config.ServerConfig
}

// NewServer creates a new HTTP server.
func NewServer(cfg *config.ServerConfig) *Server {
	return &Server{
		config:     cfg,
		grpcTarget: fmt.Sprintf("localhost:%d", cfg.GRPCPort),
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
	)

	// Connect to gRPC server
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	if err := financev1.RegisterUOMServiceHandlerFromEndpoint(ctx, gwMux, s.grpcTarget, opts); err != nil {
		return fmt.Errorf("failed to register gateway: %w", err)
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

	// CORS middleware
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
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
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"healthy"}`))
}

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

func (s *Server) liveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"live"}`))
}

// Swagger handlers
func (s *Server) swaggerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(swaggerUIHTML))
}

func (s *Server) swaggerJSONHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	http.ServeFile(w, r, "gen/openapi/finance/v1/uom.swagger.json")
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
                layout: "BaseLayout"
            });
        };
    </script>
</body>
</html>`
