package rmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher publishes JSON messages to the finance.cost exchange.
type Publisher struct {
	conn *Connection
}

// NewPublisher constructs a publisher bound to the given connection.
func NewPublisher(conn *Connection) *Publisher {
	return &Publisher{conn: conn}
}

// Publish serializes payload to JSON and publishes via the finance.cost
// exchange with the given routing key. Message is persistent.
func (p *Publisher) Publish(ctx context.Context, routingKey string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	pub := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}
	if err := p.conn.Channel().PublishWithContext(ctx, ExchangeCost, routingKey, false, false, pub); err != nil {
		return fmt.Errorf("publish to %s: %w", routingKey, err)
	}
	return nil
}
