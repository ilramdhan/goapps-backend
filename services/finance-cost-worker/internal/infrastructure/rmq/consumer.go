package rmq

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Consumer wraps a single-queue blocking consumer with manual ack.
type Consumer struct {
	conn        *Connection
	queueName   string
	consumerTag string
}

// NewConsumer constructs a consumer for the given queue.
func NewConsumer(conn *Connection, queueName, consumerTag string) *Consumer {
	return &Consumer{conn: conn, queueName: queueName, consumerTag: consumerTag}
}

// Handler receives a delivery + must call ack/nack. Return non-nil error to
// signal the dispatcher that the consume loop should terminate.
type Handler func(ctx context.Context, d amqp.Delivery) error

// Consume blocks until ctx is canceled or the delivery channel closes.
// Each message is dispatched to handler. Handler is responsible for ack/nack.
func (c *Consumer) Consume(ctx context.Context, handler Handler) error {
	deliveries, err := c.conn.Channel().Consume(
		c.queueName,
		c.consumerTag,
		false, // autoAck -- handler must ack
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,
	)
	if err != nil {
		return fmt.Errorf("consume %s: %w", c.queueName, err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed for queue %s", c.queueName)
			}
			if err := handler(ctx, d); err != nil {
				return err
			}
		}
	}
}
