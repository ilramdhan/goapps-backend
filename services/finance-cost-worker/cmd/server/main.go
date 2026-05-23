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
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/pkg/costcalc/metrics"
	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/config"
	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/infrastructure/financeclient"
	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/infrastructure/rmq"
	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/infrastructure/tracing"
	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/worker"
)

// scrapeDBPool periodically writes db.Stats().InUse into the gauge.
func scrapeDBPool(ctx context.Context, db *sql.DB, service string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics.DBPoolInUse.WithLabelValues(service).Set(float64(db.Stats().InUse))
		}
	}
}

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

	// OpenTelemetry tracing. No-op when disabled (the default).
	shutdownTracer, err := tracing.InitTracer(
		ctx,
		cfg.Tracing.Enabled,
		cfg.Tracing.ServiceName,
		cfg.App.Version,
		cfg.Tracing.Endpoint,
		cfg.Tracing.Insecure,
	)
	if err != nil {
		return fmt.Errorf("init tracer: %w", err)
	}
	defer func() {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		if shutErr := shutdownTracer(shutCtx); shutErr != nil {
			log.Warn().Err(shutErr).Msg("shutdown tracer")
		}
	}()
	if cfg.Tracing.Enabled {
		log.Info().Str("endpoint", cfg.Tracing.Endpoint).Msg("OpenTelemetry tracing enabled")
	}

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

	// PostgreSQL connection (used only for the cal_job_chunk lifecycle SQL
	// performed by the worker; finance owns calc state otherwise).
	db, err := sql.Open("postgres", cfg.Database.ConnectionString())
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close db")
		}
	}()
	db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.Database.ConnMaxIdleTime)
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := db.PingContext(pingCtx); err != nil {
		pingCancel()
		return fmt.Errorf("ping db: %w", err)
	}
	pingCancel()
	log.Info().Msg("PostgreSQL connected")

	// Finance gRPC client (used to call CostCalcService/ProcessChunkInternal
	// for each chunk consumed off the queue).
	fin, err := financeclient.New(cfg.Finance.GRPCHost, cfg.Finance.GRPCPort, cfg.Finance.ServiceAuthToken, cfg.Finance.CallTimeout)
	if err != nil {
		return fmt.Errorf("dial finance: %w", err)
	}
	defer func() {
		if closeErr := fin.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close finance client")
		}
	}()
	log.Info().Str("finance", fmt.Sprintf("%s:%d", cfg.Finance.GRPCHost, cfg.Finance.GRPCPort)).Msg("finance gRPC client ready")

	// Start worker loop.
	w := worker.New(cfg, workerID, db, rmqConn, fin)
	workerErrCh := make(chan error, 1)
	go func() { workerErrCh <- w.Run(ctx) }()

	// Background DB-pool gauge scraper.
	go scrapeDBPool(ctx, db, "worker")

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
