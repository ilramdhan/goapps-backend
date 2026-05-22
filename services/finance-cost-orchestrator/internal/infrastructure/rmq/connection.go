// Package rmq provides RabbitMQ connection helpers for the orchestrator.
//
// The bootstrap connection is intentionally minimal: a single AMQP connection
// plus one channel. Reconnect and consumer/publisher logic land in S8c.4
// (publisher) and S8c.7 (consumer). The full topology declaration (exchanges,
// queues, DLX) already lives in services/finance/internal/infrastructure/rabbitmq
// and will be reused or extracted to a shared package when this service starts
// consuming/producing real messages.
package rmq

import (
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Connection wraps an AMQP connection and a single channel.
type Connection struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

// Connect opens a connection + channel to RabbitMQ. Returns an error if either
// step fails (the connection is closed on channel failure to avoid a leak).
func Connect(url string) (*Connection, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("open channel: %w (and close: %w)", err, closeErr)
		}
		return nil, fmt.Errorf("open channel: %w", err)
	}
	return &Connection{conn: conn, ch: ch}, nil
}

// ConnectWithRetry retries connection up to maxAttempts with the given delay.
// Useful for local dev so the service exits in bounded time when RMQ is down.
func ConnectWithRetry(url string, maxAttempts int, delay time.Duration) (*Connection, error) {
	var last error
	for i := range maxAttempts {
		c, err := Connect(url)
		if err == nil {
			return c, nil
		}
		last = err
		if i < maxAttempts-1 {
			time.Sleep(delay)
		}
	}
	return nil, last
}

// Channel returns the active channel. Callers MUST NOT close it directly.
func (c *Connection) Channel() *amqp.Channel { return c.ch }

// Close shuts down both the channel and the connection.
func (c *Connection) Close() error {
	if c.ch != nil {
		_ = c.ch.Close()
	}
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("close rabbitmq connection: %w", err)
		}
	}
	return nil
}
