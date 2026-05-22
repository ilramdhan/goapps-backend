// Package rmq provides RabbitMQ connection helpers for the worker.
//
// The bootstrap connection is intentionally minimal: a single AMQP connection
// plus one channel. Consumer / publisher logic (chunk-consume, chunk-done
// publish) lands in S8c.7.
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

// Connect opens a connection + channel to RabbitMQ.
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
func ConnectWithRetry(url string, maxAttempts int, delay time.Duration) (*Connection, error) {
	var last error
	for i := 0; i < maxAttempts; i++ {
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
