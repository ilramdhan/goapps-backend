package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

// MessageHandler processes a single job message.
// Return nil to acknowledge, non-nil to reject (will go to DLQ).
type MessageHandler func(ctx context.Context, msg JobMessage) error

// Consumer consumes messages from a RabbitMQ queue, optionally processing up
// to concurrency deliveries in parallel via a bounded worker pool.
type Consumer struct {
	conn        *Connection
	queueName   string
	handler     MessageHandler
	logger      zerolog.Logger
	concurrency int
}

// NewConsumer creates a new Consumer for the given queue that processes
// deliveries strictly one at a time (concurrency 1), sharing conn's default
// channel and its prefetch_count. This is the correct choice for job types
// whose handlers are not known to be safe under concurrent execution.
func NewConsumer(conn *Connection, queueName string, handler MessageHandler, logger zerolog.Logger) *Consumer {
	return NewConcurrentConsumer(conn, queueName, handler, logger, 1)
}

// NewConcurrentConsumer creates a Consumer that processes up to concurrency
// deliveries in parallel using a bounded worker pool. concurrency <= 1 falls
// back to strictly sequential processing on the connection's shared channel
// (same as NewConsumer). concurrency > 1 opens a dedicated AMQP channel with
// its own QoS(concurrency) prefetch, so raising this consumer's concurrency
// never changes the prefetch of any other consumer sharing the connection's
// default channel.
func NewConcurrentConsumer(
	conn *Connection, queueName string, handler MessageHandler, logger zerolog.Logger, concurrency int,
) *Consumer {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Consumer{
		conn:        conn,
		queueName:   queueName,
		handler:     handler,
		logger:      logger,
		concurrency: concurrency,
	}
}

// Start begins consuming messages. Blocks until ctx is canceled or the
// delivery channel closes, and waits for all in-flight deliveries to finish
// processing before returning.
func (c *Consumer) Start(ctx context.Context) error {
	ch := c.conn.Channel()
	if c.concurrency > 1 {
		dedicated, err := c.conn.OpenChannel(c.concurrency)
		if err != nil {
			return fmt.Errorf("open dedicated channel for %s: %w", c.queueName, err)
		}
		defer func() {
			if closeErr := dedicated.Close(); closeErr != nil {
				c.logger.Warn().Err(closeErr).Str("queue", c.queueName).Msg("close dedicated channel")
			}
		}()
		ch = dedicated
	}

	deliveries, err := ch.Consume(
		c.queueName,
		"",    // consumer tag (auto-generated)
		false, // auto-ack disabled (manual ack)
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("start consuming %s: %w", c.queueName, err)
	}

	c.logger.Info().
		Str("queue", c.queueName).
		Int("concurrency", c.concurrency).
		Msg("Consumer started")

	// sem bounds the number of deliveries being processed concurrently to
	// c.concurrency; wg lets Start block until every in-flight goroutine has
	// finished (ack/nack'd) before returning, so a context cancellation never
	// abandons a delivery mid-processing.
	sem := make(chan struct{}, c.concurrency)
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info().Msg("Consumer stopping due to context cancellation")
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				c.logger.Warn().Msg("Delivery channel closed, consumer exiting")
				return nil
			}
			c.dispatch(ctx, delivery, sem, &wg)
		}
	}
}

// dispatch acquires a pool slot and processes delivery in its own goroutine,
// tracked by wg. Acquiring sem before spawning bounds the number of
// deliveries ever mid-flight to cap(sem); this blocks pulling the next
// delivery off the channel once the pool is full, which is the desired
// backpressure (bounded together with the channel's own prefetch). With
// cap(sem) == 1 (the NewConsumer default) this degrades to strictly
// sequential processing: the next delivery cannot dispatch until the
// previous goroutine has released its slot.
func (c *Consumer) dispatch(ctx context.Context, delivery amqp.Delivery, sem chan struct{}, wg *sync.WaitGroup) {
	sem <- struct{}{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { <-sem }()
		c.processDelivery(ctx, delivery)
	}()
}

func (c *Consumer) processDelivery(ctx context.Context, delivery amqp.Delivery) {
	var msg JobMessage
	if err := json.Unmarshal(delivery.Body, &msg); err != nil {
		c.logger.Error().Err(err).Msg("Failed to unmarshal message, rejecting to DLQ")
		if nackErr := delivery.Nack(false, false); nackErr != nil {
			c.logger.Error().Err(nackErr).Msg("Failed to nack message")
		}
		return
	}

	c.logger.Info().
		Str("job_id", msg.JobID).
		Str("job_type", msg.JobType).
		Str("period", msg.Period).
		Msg("Processing job message")

	if err := c.handler(ctx, msg); err != nil {
		c.logger.Error().Err(err).
			Str("job_id", msg.JobID).
			Msg("Handler failed, rejecting to DLQ")
		if nackErr := delivery.Nack(false, false); nackErr != nil {
			c.logger.Error().Err(nackErr).Msg("Failed to nack message")
		}
		return
	}

	if err := delivery.Ack(false); err != nil {
		c.logger.Error().Err(err).
			Str("job_id", msg.JobID).
			Msg("Failed to ack message")
	}
}
