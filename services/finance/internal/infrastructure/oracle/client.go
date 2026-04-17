// Package oracle provides Oracle database connectivity using go-ora (pure Go driver).
package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	go_ora "github.com/sijms/go-ora/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

// Client wraps an Oracle database connection pool.
type Client struct {
	db     *sql.DB
	config config.OracleConfig
	logger zerolog.Logger
}

// NewClient creates a new Oracle database client.
func NewClient(cfg config.OracleConfig, logger zerolog.Logger) (*Client, error) {
	connStr := go_ora.BuildUrl(cfg.Host, cfg.Port, cfg.Service, cfg.User, cfg.Password, nil)

	db, err := sql.Open("oracle", connStr)
	if err != nil {
		return nil, fmt.Errorf("oracle open: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetMaxIdleConns(2)

	// Verify connectivity.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			logger.Warn().Err(closeErr).Msg("failed to close oracle db after ping failure")
		}
		return nil, fmt.Errorf("oracle ping: %w", err)
	}

	logger.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Str("service", cfg.Service).
		Str("user", cfg.User).
		Msg("Oracle database connected")

	return &Client{db: db, config: cfg, logger: logger}, nil
}

// Close closes the Oracle database connection pool.
func (c *Client) Close() error {
	if c.db == nil {
		return nil
	}
	return c.db.Close()
}

// DB returns the underlying sql.DB for direct queries.
func (c *Client) DB() *sql.DB {
	return c.db
}

// Ping checks if the Oracle database is reachable.
func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// ExecuteProcedure runs an Oracle PL/SQL stored procedure.
// This is a blocking call that returns only after the procedure completes.
// For long-running procedures (10-20 min), set an appropriate context timeout.
func (c *Client) ExecuteProcedure(ctx context.Context, schema, procedure string) error {
	plsql := fmt.Sprintf("BEGIN %s.%s; END;", schema, procedure)

	c.logger.Info().
		Str("schema", schema).
		Str("procedure", procedure).
		Msg("Executing Oracle procedure")

	start := time.Now()

	_, err := c.db.ExecContext(ctx, plsql)
	if err != nil {
		return fmt.Errorf("execute procedure %s.%s: %w", schema, procedure, err)
	}

	c.logger.Info().
		Str("schema", schema).
		Str("procedure", procedure).
		Dur("duration", time.Since(start)).
		Msg("Oracle procedure completed")

	return nil
}

// ExecuteProcedureWithParam runs an Oracle PL/SQL stored procedure with a single string parameter.
func (c *Client) ExecuteProcedureWithParam(ctx context.Context, schema, procedure, param string) error {
	plsql := fmt.Sprintf("BEGIN %s.%s(:1); END;", schema, procedure)

	c.logger.Info().
		Str("schema", schema).
		Str("procedure", procedure).
		Str("param", param).
		Msg("Executing Oracle procedure with parameter")

	start := time.Now()

	_, err := c.db.ExecContext(ctx, plsql, param)
	if err != nil {
		return fmt.Errorf("execute procedure %s.%s(%s): %w", schema, procedure, param, err)
	}

	c.logger.Info().
		Str("schema", schema).
		Str("procedure", procedure).
		Dur("duration", time.Since(start)).
		Msg("Oracle procedure completed")

	return nil
}
