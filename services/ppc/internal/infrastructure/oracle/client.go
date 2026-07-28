// Package oracle provides read-only Oracle connectivity for PPC ETL and the
// machine sync, using the pure-Go go-ora driver (no CGO / Instant Client).
package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	go_ora "github.com/sijms/go-ora/v2"

	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/config"
)

// Client wraps a read-only Oracle connection pool.
type Client struct {
	db *sql.DB
}

// New opens an Oracle connection pool from config. An empty host/user yields a
// nil client (degraded): callers treat a nil client as "Oracle unavailable" and
// proceed without it. Connectivity is verified with a short ping; a ping failure
// returns an error so the caller can decide to degrade.
func New(cfg config.OracleConfig) (*Client, error) {
	if cfg.Host == "" || cfg.User == "" {
		log.Warn().Msg("oracle: host/user not configured, running without Oracle (degraded)")
		return nil, nil //nolint:nilnil // absent config legitimately maps to no client
	}

	connStr := go_ora.BuildUrl(cfg.Host, cfg.Port, cfg.Service, cfg.User, cfg.Password, nil)
	db, err := sql.Open("oracle", connStr)
	if err != nil {
		return nil, fmt.Errorf("oracle open: %w", err)
	}

	maxConns := cfg.MaxOpenConns
	if maxConns <= 0 {
		maxConns = 5
	}
	db.SetMaxOpenConns(maxConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetMaxIdleConns(2)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("oracle: failed to close after ping failure")
		}
		return nil, fmt.Errorf("oracle ping: %w", err)
	}

	log.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Str("service", cfg.Service).
		Msg("Oracle connected")

	return &Client{db: db}, nil
}

// Close closes the Oracle connection pool.
func (c *Client) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

// DB returns the underlying *sql.DB for direct queries.
func (c *Client) DB() *sql.DB {
	return c.db
}
