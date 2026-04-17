// Package rabbitmq provides RabbitMQ messaging infrastructure.
package rabbitmq

import (
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

const (
	// ExchangeName is the topic exchange for all finance jobs.
	ExchangeName = "finance.jobs"
	// QueueOracleSync is the queue for oracle sync jobs.
	QueueOracleSync = "finance.jobs.oracle_sync"
	// RoutingKeyOracleSync is the routing key for oracle sync messages.
	RoutingKeyOracleSync = "oracle_sync"
	// DeadLetterExchange is the dead letter exchange for failed messages.
	DeadLetterExchange = "finance.jobs.dlx"
	// DeadLetterQueue is the dead letter queue.
	DeadLetterQueue = "finance.jobs.dlq"
)

// Connection wraps an AMQP connection and channel with reconnect support.
type Connection struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	config  config.RabbitMQConfig
	logger  zerolog.Logger
}

// NewConnection creates a new RabbitMQ connection and declares topology.
func NewConnection(cfg config.RabbitMQConfig, logger zerolog.Logger) (*Connection, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("connect to rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			logger.Warn().Err(closeErr).Msg("close connection after channel failure")
		}
		return nil, fmt.Errorf("open channel: %w", err)
	}

	if err := ch.Qos(cfg.PrefetchCount, 0, false); err != nil {
		if closeErr := ch.Close(); closeErr != nil {
			logger.Warn().Err(closeErr).Msg("close channel after qos failure")
		}
		if closeErr := conn.Close(); closeErr != nil {
			logger.Warn().Err(closeErr).Msg("close connection after qos failure")
		}
		return nil, fmt.Errorf("set qos: %w", err)
	}

	c := &Connection{
		conn:    conn,
		channel: ch,
		config:  cfg,
		logger:  logger,
	}

	if err := c.declareTopology(); err != nil {
		if closeErr := ch.Close(); closeErr != nil {
			logger.Warn().Err(closeErr).Msg("close channel after topology failure")
		}
		if closeErr := conn.Close(); closeErr != nil {
			logger.Warn().Err(closeErr).Msg("close connection after topology failure")
		}
		return nil, fmt.Errorf("declare topology: %w", err)
	}

	logger.Info().
		Str("url", sanitizeURL(cfg.URL)).
		Msg("RabbitMQ connected and topology declared")

	return c, nil
}

// Channel returns the underlying AMQP channel.
func (c *Connection) Channel() *amqp.Channel {
	return c.channel
}

// Close closes the channel and connection.
func (c *Connection) Close() error {
	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			c.logger.Warn().Err(err).Msg("close rabbitmq channel")
		}
	}
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("close rabbitmq connection: %w", err)
		}
	}
	c.logger.Info().Msg("RabbitMQ connection closed")
	return nil
}

// NotifyClose returns a channel that signals connection closure.
func (c *Connection) NotifyClose() chan *amqp.Error {
	return c.conn.NotifyClose(make(chan *amqp.Error, 1))
}

// ReconnectDelay returns the configured reconnect delay.
func (c *Connection) ReconnectDelay() time.Duration {
	if c.config.ReconnectDelay > 0 {
		return c.config.ReconnectDelay
	}
	return 5 * time.Second
}

func (c *Connection) declareTopology() error {
	// Dead letter exchange + queue.
	if err := c.channel.ExchangeDeclare(DeadLetterExchange, "fanout", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare DLX: %w", err)
	}
	if _, err := c.channel.QueueDeclare(DeadLetterQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare DLQ: %w", err)
	}
	if err := c.channel.QueueBind(DeadLetterQueue, "", DeadLetterExchange, false, nil); err != nil {
		return fmt.Errorf("bind DLQ: %w", err)
	}

	// Main topic exchange.
	if err := c.channel.ExchangeDeclare(ExchangeName, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}

	// Oracle sync queue with dead-letter routing.
	args := amqp.Table{
		"x-dead-letter-exchange": DeadLetterExchange,
	}
	if _, err := c.channel.QueueDeclare(QueueOracleSync, true, false, false, false, args); err != nil {
		return fmt.Errorf("declare oracle sync queue: %w", err)
	}
	if err := c.channel.QueueBind(QueueOracleSync, RoutingKeyOracleSync, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind oracle sync queue: %w", err)
	}

	return nil
}

// sanitizeURL removes credentials from the AMQP URL for logging.
func sanitizeURL(url string) string {
	// Simple: just show host portion.
	if len(url) > 20 {
		return url[:7] + "***@" + url[len(url)-15:]
	}
	return "amqp://***"
}
