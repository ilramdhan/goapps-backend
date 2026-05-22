// Package main is the entry point for the finance-cost-worker service.
//
// Bootstrap responsibilities:
//   - Load config (viper)
//   - Configure zerolog
//   - Generate worker_id (hostname-pid) if not provided
//   - Establish RabbitMQ connection (with bounded retry)
//   - Expose Prometheus /metrics + /healthz
//   - Start the Worker main loop
//   - Handle SIGINT/SIGTERM for graceful shutdown
//
// The actual chunk consumer + calc executor + result publisher lands in S8c.7.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/config"
	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/infrastructure/rmq"
	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/worker"
)

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("worker failed")
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	setupLogger(cfg)

	workerID := resolveWorkerID(cfg.Worker.WorkerID)

	log.Info().
		Str("service", cfg.App.Name).
		Str("version", cfg.App.Version).
		Str("environment", cfg.App.Env).
		Str("worker_id", workerID).
		Msg("Starting finance-cost-worker")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// RabbitMQ connection (with bounded retry so we exit fast when RMQ is down).
	rmqConn, err := rmq.ConnectWithRetry(cfg.RabbitMQ.URL, 3, cfg.RabbitMQ.ReconnectDelay)
	if err != nil {
		return fmt.Errorf("connect rabbitmq: %w", err)
	}
	defer func() {
		if closeErr := rmqConn.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close rabbitmq")
		}
	}()
	log.Info().Msg("RabbitMQ connected")

	// DB connection deferred to S8c.7.

	// Start worker loop.
	w := worker.New(cfg, rmqConn, workerID)
	workerErrCh := make(chan error, 1)
	go func() { workerErrCh <- w.Run(ctx) }()

	// HTTP server for /metrics + /healthz.
	srv := newHTTPServer(cfg.Server.MetricsPort)
	srvErrCh := make(chan error, 1)
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("metrics+health server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErrCh <- err
			return
		}
		srvErrCh <- nil
	}()

	// Wait for shutdown signal or fatal error.
	select {
	case <-ctx.Done():
		log.Info().Msg("shutdown signal received")
	case err := <-workerErrCh:
		if err != nil {
			log.Error().Err(err).Msg("worker exited with error")
			cancel()
			return err
		}
	case err := <-srvErrCh:
		if err != nil {
			log.Error().Err(err).Msg("metrics server exited with error")
			cancel()
			return err
		}
	}

	// Graceful shutdown.
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Warn().Err(err).Msg("metrics server shutdown")
	}

	log.Info().Str("worker_id", workerID).Msg("worker stopped")
	return nil
}

func setupLogger(cfg *config.Config) {
	zerolog.TimeFieldFormat = time.RFC3339
	switch cfg.Logger.Level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	if cfg.App.Env == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

// resolveWorkerID returns the configured worker id, falling back to
// "hostname-pid" so each pod/process is uniquely identifiable.
func resolveWorkerID(configured string) string {
	if configured != "" {
		return configured
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func newHTTPServer(port int) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
