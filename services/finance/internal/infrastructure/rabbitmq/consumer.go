package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

// MessageHandler processes a single job message.
// Return nil to acknowledge, non-nil to reject (will go to DLQ).
type MessageHandler func(ctx context.Context, msg JobMessage) error

// Consumer consumes messages from a RabbitMQ queue.
type Consumer struct {
	conn      *Connection
	queueName string
	handler   MessageHandler
	logger    zerolog.Logger
}

// NewConsumer creates a new Consumer for the given queue.
func NewConsumer(conn *Connection, queueName string, handler MessageHandler, logger zerolog.Logger) *Consumer {
	return &Consumer{
		conn:      conn,
		queueName: queueName,
		handler:   handler,
		logger:    logger,
	}
}

// Start begins consuming messages. Blocks until ctx is cancelled.
func (c *Consumer) Start(ctx context.Context) error {
	deliveries, err := c.conn.Channel().Consume(
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
		Msg("Consumer started")

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
			c.processDelivery(ctx, delivery)
		}
	}
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
