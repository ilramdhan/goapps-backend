// Package rabbitmq provides RabbitMQ messaging infrastructure.
package rabbitmq

import (
	"context"
	"fmt"
	"net/url"
	"sync"
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
	// QueueRMCostCalc is the queue for RM landed-cost calculation jobs.
	QueueRMCostCalc = "finance.jobs.rm_cost_calc"
	// RoutingKeyRMCostCalc is the routing key for RM cost calculation messages.
	RoutingKeyRMCostCalc = "rm_cost_calculation"
	// QueueRMCostExport is the queue for RM cost async export jobs.
	QueueRMCostExport = "finance.jobs.rm_cost_export"
	// RoutingKeyRMCostExport is the routing key for RM cost export messages.
	RoutingKeyRMCostExport = "rm_cost_export"
	// QueueProductCostSheetExport is the queue for product cost sheet async export jobs.
	QueueProductCostSheetExport = "finance.jobs.product_cost_sheet_export"
	// RoutingKeyProductCostSheetExport is the routing key for product cost sheet export messages.
	RoutingKeyProductCostSheetExport = "product_cost_sheet_export"
	// QueueImportJob is the queue name for costing data import jobs.
	QueueImportJob = "finance.costing.import"
	// RoutingKeyImportJob is the routing key for costing data import jobs.
	RoutingKeyImportJob = "costing_import"
	// DeadLetterExchange is the dead letter exchange for failed messages.
	DeadLetterExchange = "finance.jobs.dlx"
	// DeadLetterQueue is the dead letter queue.
	DeadLetterQueue = "finance.jobs.dlq"
)

// Connection wraps an AMQP connection and channel.
//
// The conn/channel pair is guarded by mu so that Supervise can swap in a fresh
// pair after a broker outage while publishers concurrently read it. Publishers
// resolve the channel per publish (see Publisher.PublishJob and
// CostJobPublisher.PublishJobTriggered), so a swap heals them with no
// publisher-side changes.
type Connection struct {
	mu      sync.RWMutex
	conn    *amqp.Connection
	channel *amqp.Channel
	// closed records a deliberate Close() so Supervise can distinguish an
	// intentional shutdown from an unexpected broker-side close.
	closed bool
	// onReconnect holds topology hooks re-run after every successful redial.
	onReconnect []func(*amqp.Channel) error

	config config.RabbitMQConfig
	logger zerolog.Logger
}

// NewConnection creates a new RabbitMQ connection and declares topology.
func NewConnection(cfg config.RabbitMQConfig, logger zerolog.Logger) (*Connection, error) {
	return dial(cfg, logger)
}

// NewConnectionWithRetry creates a connection, retrying up to maxAttempts times
// with the configured reconnect delay between attempts. Use this at startup so
// a service that boots before RabbitMQ is reachable still ends up with working
// publishers instead of permanently nil ones. Returns the last dial error when
// every attempt fails.
func NewConnectionWithRetry(cfg config.RabbitMQConfig, logger zerolog.Logger, maxAttempts int) (*Connection, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	delay := reconnectDelay(cfg)
	var last error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		c, err := dial(cfg, logger)
		if err == nil {
			return c, nil
		}
		last = err
		if attempt < maxAttempts {
			logger.Warn().
				Err(err).
				Int("attempt", attempt).
				Int("max_attempts", maxAttempts).
				Dur("retry_in", delay).
				Str("url", SanitizeURL(cfg.URL)).
				Msg("RabbitMQ connect failed, retrying")
			time.Sleep(delay)
		}
	}
	return nil, last
}

// dial performs a single connect + QoS + topology-declare cycle.
func dial(cfg config.RabbitMQConfig, logger zerolog.Logger) (*Connection, error) {
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
		closeBoth(conn, ch, logger, "qos failure")
		return nil, fmt.Errorf("set qos: %w", err)
	}

	c := &Connection{
		conn:    conn,
		channel: ch,
		config:  cfg,
		logger:  logger,
	}

	if err := c.declareTopology(); err != nil {
		closeBoth(conn, ch, logger, "topology failure")
		return nil, fmt.Errorf("declare topology: %w", err)
	}

	logger.Info().
		Str("url", SanitizeURL(cfg.URL)).
		Msg("RabbitMQ connected and topology declared")

	return c, nil
}

// closeBoth tears down a half-built connection, logging (not returning) errors.
func closeBoth(conn *amqp.Connection, ch *amqp.Channel, logger zerolog.Logger, reason string) {
	if closeErr := ch.Close(); closeErr != nil {
		logger.Warn().Err(closeErr).Msgf("close channel after %s", reason)
	}
	if closeErr := conn.Close(); closeErr != nil {
		logger.Warn().Err(closeErr).Msgf("close connection after %s", reason)
	}
}

// Channel returns the underlying shared AMQP channel. All consumers created
// with concurrency 1 (the default) share this channel and therefore its
// single QoS(prefetch_count) setting.
//
// After a Supervise-driven reconnect this returns the new channel. A caller
// may still observe a channel that is in the process of closing; the publish
// then fails with an error and the caller retries, which is how every caller
// already behaves.
func (c *Connection) Channel() *amqp.Channel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.channel
}

// OnReconnect registers a hook run after every successful reconnect, once the
// base topology has been re-declared. Durable exchanges and queues survive a
// client reconnect but NOT a broker restart, and some topology is declared only
// in a constructor (e.g. CostJobPublisher declares the finance.cost exchange),
// so those declares must be replayed here to keep publishing working.
func (c *Connection) OnReconnect(fn func(*amqp.Channel) error) {
	if fn == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onReconnect = append(c.onReconnect, fn)
}

// OpenChannel opens a new, independent AMQP channel on the same connection
// with its own QoS(prefetchCount) setting. Used by consumers that need a
// prefetch higher than the shared channel's — e.g. a bounded worker pool
// processing several deliveries concurrently — without changing the prefetch
// of any other consumer sharing the default Channel(). The caller owns
// closing the returned channel.
func (c *Connection) OpenChannel(prefetchCount int) (*amqp.Channel, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open channel: %w", err)
	}
	if err := ch.Qos(prefetchCount, 0, false); err != nil {
		if closeErr := ch.Close(); closeErr != nil {
			c.logger.Warn().Err(closeErr).Msg("close channel after qos failure")
		}
		return nil, fmt.Errorf("set qos: %w", err)
	}
	return ch, nil
}

// Close closes the channel and connection. It also marks the connection as
// deliberately closed so a running Supervise does not treat the resulting
// close notification as an outage worth redialing.
func (c *Connection) Close() error {
	c.mu.Lock()
	c.closed = true
	ch, conn := c.channel, c.conn
	c.mu.Unlock()

	if ch != nil {
		if err := ch.Close(); err != nil {
			c.logger.Warn().Err(err).Msg("close rabbitmq channel")
		}
	}
	if conn != nil {
		if err := conn.Close(); err != nil {
			return fmt.Errorf("close rabbitmq connection: %w", err)
		}
	}
	c.logger.Info().Msg("RabbitMQ connection closed")
	return nil
}

// NotifyClose returns a channel that signals closure of the connection that is
// current at call time. After a Supervise-driven reconnect, a previously
// returned notification channel refers to the old connection; call it again to
// observe the new one.
func (c *Connection) NotifyClose() chan *amqp.Error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	return conn.NotifyClose(make(chan *amqp.Error, 1))
}

// ReconnectDelay returns the configured reconnect delay.
func (c *Connection) ReconnectDelay() time.Duration {
	return reconnectDelay(c.config)
}

// reconnectDelay resolves the retry delay, defaulting to 5s when unset.
func reconnectDelay(cfg config.RabbitMQConfig) time.Duration {
	if cfg.ReconnectDelay > 0 {
		return cfg.ReconnectDelay
	}
	return 5 * time.Second
}

// isClosed reports whether Close() was called on this connection.
func (c *Connection) isClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// Supervise blocks until ctx is done, redialing RabbitMQ whenever the current
// connection closes unexpectedly. After each successful redial the base
// topology is re-declared, the conn/channel pair is swapped in place, and every
// OnReconnect hook is replayed — so publishers, which resolve the channel per
// publish, resume working without restarting the process.
//
// Known limitation: this heals PUBLISHERS only. Consumers created via
// OpenChannel or Channel are NOT re-subscribed after a swap — their delivery
// goroutine still holds the dead channel. cmd/worker relies on process restart
// for consumer recovery.
func (c *Connection) Supervise(ctx context.Context) {
	for {
		closeCh := c.NotifyClose()
		select {
		case <-ctx.Done():
			return
		case amqpErr := <-closeCh:
			if c.isClosed() || ctx.Err() != nil {
				return
			}
			c.logger.Warn().
				AnErr("amqp_error", amqpErr).
				Str("url", SanitizeURL(c.config.URL)).
				Msg("RabbitMQ connection lost, reconnecting")
			if !c.redialLoop(ctx) {
				return
			}
		}
	}
}

// redialLoop retries dialing until it succeeds, ctx is cancelled, or the
// connection was deliberately closed. Reports whether a swap happened.
func (c *Connection) redialLoop(ctx context.Context) bool {
	delay := c.ReconnectDelay()
	for attempt := 1; ; attempt++ {
		if c.isClosed() {
			return false
		}
		fresh, err := dial(c.config, c.logger)
		if err == nil {
			c.swap(fresh)
			c.logger.Info().Int("attempt", attempt).Msg("RabbitMQ reconnected, publishers recovered")
			return true
		}
		c.logger.Warn().Err(err).Int("attempt", attempt).Dur("retry_in", delay).
			Msg("RabbitMQ reconnect attempt failed")
		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}
	}
}

// swap installs the freshly dialed conn/channel and replays reconnect hooks.
func (c *Connection) swap(fresh *Connection) {
	c.mu.Lock()
	c.conn, c.channel = fresh.conn, fresh.channel
	hooks := make([]func(*amqp.Channel) error, len(c.onReconnect))
	copy(hooks, c.onReconnect)
	ch := c.channel
	c.mu.Unlock()

	for _, hook := range hooks {
		if err := hook(ch); err != nil {
			c.logger.Error().Err(err).Msg("RabbitMQ reconnect hook failed; some topology may be missing")
		}
	}
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

	// RM cost calculation queue with dead-letter routing.
	if _, err := c.channel.QueueDeclare(QueueRMCostCalc, true, false, false, false, args); err != nil {
		return fmt.Errorf("declare rm cost calc queue: %w", err)
	}
	if err := c.channel.QueueBind(QueueRMCostCalc, RoutingKeyRMCostCalc, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind rm cost calc queue: %w", err)
	}

	// RM cost export queue with dead-letter routing.
	if _, err := c.channel.QueueDeclare(QueueRMCostExport, true, false, false, false, args); err != nil {
		return fmt.Errorf("declare rm cost export queue: %w", err)
	}
	if err := c.channel.QueueBind(QueueRMCostExport, RoutingKeyRMCostExport, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind rm cost export queue: %w", err)
	}

	// Product cost sheet export queue with dead-letter routing.
	if _, err := c.channel.QueueDeclare(QueueProductCostSheetExport, true, false, false, false, args); err != nil {
		return fmt.Errorf("declare product cost sheet export queue: %w", err)
	}
	if err := c.channel.QueueBind(QueueProductCostSheetExport, RoutingKeyProductCostSheetExport, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind product cost sheet export queue: %w", err)
	}

	// Costing import queue with dead-letter routing.
	if _, err := c.channel.QueueDeclare(QueueImportJob, true, false, false, false, args); err != nil {
		return fmt.Errorf("declare costing import queue: %w", err)
	}
	if err := c.channel.QueueBind(QueueImportJob, RoutingKeyImportJob, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind costing import queue: %w", err)
	}

	return nil
}

// SanitizeURL removes credentials from the AMQP URL for logging.
func SanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "amqp://***"
	}
	parsed.User = url.User("***")
	return parsed.String()
}
