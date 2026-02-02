// Package postgres provides PostgreSQL database connection and utilities.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver with SCRAM-SHA-256 support
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

// DB wraps the SQL database connection.
type DB struct {
	*sql.DB
}

// NewConnection creates a new PostgreSQL connection pool.
func NewConnection(cfg *config.DatabaseConfig) (*DB, error) {
	db, err := sql.Open("pgx", cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Str("database", cfg.Name).
		Int("max_open_conns", cfg.MaxOpenConns).
		Int("max_idle_conns", cfg.MaxIdleConns).
		Msg("Database connection established")

	return &DB{db}, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	if err := db.DB.Close(); err != nil {
		return fmt.Errorf("failed to close database connection: %w", err)
	}
	log.Info().Msg("Database connection closed")
	return nil
}

// Health checks if the database is healthy.
func (db *DB) Health(ctx context.Context) error {
	return db.PingContext(ctx)
}

// Transaction executes a function within a database transaction.
func (db *DB) Transaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			// Use errors.Join to properly wrap both errors (Go 1.20+)
			return errors.Join(
				fmt.Errorf("rollback failed: %w", rbErr),
				fmt.Errorf("transaction error: %w", err),
			)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
