// Package main is the entry point for the finance-cost-orchestrator service.
//
// Bootstrap responsibilities:
//   - Load config (viper)
//   - Configure zerolog
//   - Establish RabbitMQ connection (with retry so failures surface quickly)
//   - Expose Prometheus /metrics + /healthz
//   - Start the Coordinator main loop
//   - Handle SIGINT/SIGTERM for graceful shutdown
//
// The actual planner / publisher / chunk-coordinator logic lands in S8c.2 - S8c.5.
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
	"github.com/mutugading/goapps-backend/services/finance-cost-orchestrator/internal/config"
	"github.com/mutugading/goapps-backend/services/finance-cost-orchestrator/internal/infrastructure/rmq"
	"github.com/mutugading/goapps-backend/services/finance-cost-orchestrator/internal/infrastructure/tracing"
	"github.com/mutugading/goapps-backend/services/finance-cost-orchestrator/internal/orchestrator"
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
		log.Fatal().Err(err).Msg("orchestrator failed")
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	setupLogger(cfg)

	log.Info().
		Str("service", cfg.App.Name).
		Str("version", cfg.App.Version).
		Str("environment", cfg.App.Env).
		Msg("Starting finance-cost-orchestrator")

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

	if err := rmq.DeclareTopology(rmqConn.Channel()); err != nil {
		return fmt.Errorf("declare rmq topology: %w", err)
	}
	log.Info().Msg("RabbitMQ topology declared")

	// DB connection for orchestrator repos.
	db, err := openDB(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close db")
		}
	}()
	log.Info().Msg("Database connected")

	// Start coordinator loop.
	coord := orchestrator.New(cfg, rmqConn, db)
	coordErrCh := make(chan error, 1)
	go func() { coordErrCh <- coord.Run(ctx) }()

	// Background DB-pool gauge scraper.
	go scrapeDBPool(ctx, db, "orchestrator")

	// Cron auto-trigger (S8e.6): monthly ALL-scope job on day 5 @ 02:00 WIB.
	cronExp := cfg.Orchestrator.CronSchedule
	if cronExp == "" {
		cronExp = "0 0 2 5 * *"
	}
	cronTZ := cfg.Orchestrator.CronTimezone
	if cronTZ == "" {
		cronTZ = "Asia/Jakarta"
	}
	cronPub := rmq.NewCronJobPublisher(rmqConn)
	sched, err := orchestrator.NewCronScheduler(db, cronPub, cronExp, cronTZ)
	if err != nil {
		return fmt.Errorf("cron scheduler init: %w", err)
	}
	nextFire, err := sched.Start()
	if err != nil {
		return fmt.Errorf("cron scheduler start: %w", err)
	}
	log.Info().Time("next_fire", nextFire).Str("expr", cronExp).Str("tz", cronTZ).Msg("cron scheduler started")
	defer sched.Stop()

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
	case err := <-coordErrCh:
		if err != nil {
			log.Error().Err(err).Msg("coordinator exited with error")
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

	log.Info().Msg("orchestrator stopped")
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

func openDB(ctx context.Context, cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.Database.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.Database.ConnMaxIdleTime)
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
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
