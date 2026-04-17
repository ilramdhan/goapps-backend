// Package main is the entry point for the finance worker service.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/oraclesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("Worker failed")
	}
}

func run() error {
	setupLogger()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log.Info().
		Str("service", cfg.App.Name+"-worker").
		Str("version", cfg.App.Version).
		Str("environment", cfg.App.Env).
		Msg("Starting finance worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup PostgreSQL.
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return err
	}
	defer closeResource("database", db)

	log.Info().
		Str("host", cfg.Database.Host).
		Int("port", cfg.Database.Port).
		Msg("Database connected")

	// Setup Oracle.
	oracleClient, err := oracle.NewClient(cfg.Oracle, log.Logger)
	if err != nil {
		return err
	}
	defer closeResource("oracle", oracleClient)

	log.Info().
		Str("host", cfg.Oracle.Host).
		Int("port", cfg.Oracle.Port).
		Msg("Oracle connected")

	// Setup RabbitMQ.
	rmqConn, err := rabbitmq.NewConnection(cfg.RabbitMQ, log.Logger)
	if err != nil {
		return err
	}
	defer closeResource("rabbitmq", rmqConn)

	// Create repositories.
	jobRepo := postgres.NewJobRepository(db)
	oracleRepo := oracle.NewItemConsStockPORepository(oracleClient)
	syncDataRepo := postgres.NewSyncDataRepository(db)

	// Create sync handler.
	syncHandler := oraclesync.NewSyncHandler(jobRepo, oracleRepo, syncDataRepo, log.Logger)

	// Create message handler.
	handler := func(ctx context.Context, msg rabbitmq.JobMessage) error {
		jobID, parseErr := uuid.Parse(msg.JobID)
		if parseErr != nil {
			log.Error().Err(parseErr).Str("job_id", msg.JobID).Msg("Invalid job ID in message")
			return parseErr
		}
		return syncHandler.Execute(ctx, jobID)
	}

	// Start consumer.
	consumer := rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueOracleSync, handler, log.Logger)

	// Handle reconnection in a goroutine.
	go watchConnection(ctx, rmqConn)

	// Start consuming in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Start(ctx)
	}()

	// Wait for shutdown signal or consumer error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("Shutdown signal received")
		cancel()
	case err := <-errCh:
		if err != nil {
			log.Error().Err(err).Msg("Consumer error")
			cancel()
			return err
		}
	}

	// Give in-flight jobs time to finish.
	log.Info().Msg("Waiting for in-flight jobs to complete...")
	time.Sleep(5 * time.Second)

	log.Info().Msg("Worker shutdown complete")
	return nil
}

func setupLogger() {
	zerolog.TimeFieldFormat = time.RFC3339
	if os.Getenv("APP_ENV") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

type closer interface {
	Close() error
}

func closeResource(name string, c closer) {
	if err := c.Close(); err != nil {
		log.Warn().Err(err).Str("resource", name).Msg("Failed to close resource")
	}
}

func watchConnection(ctx context.Context, conn *rabbitmq.Connection) {
	closeCh := conn.NotifyClose()
	select {
	case <-ctx.Done():
		return
	case err := <-closeCh:
		if err != nil {
			log.Error().Err(err).Msg("RabbitMQ connection lost")
		}
	}
}
