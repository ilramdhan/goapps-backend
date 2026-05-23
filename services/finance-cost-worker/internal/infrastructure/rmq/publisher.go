package rmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
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
	// Inject the current span's trace context so the orchestrator can link the
	// chunk-completed handling back to the same trace. When tracing is disabled
	// the propagator is the global no-op and writes nothing.
	headers := amqp.Table{}
	otel.GetTextMapPropagator().Inject(ctx, propagation.TextMapCarrier(HeaderCarrier(headers)))
	pub := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Headers:      headers,
		Body:         body,
	}
	if err := p.conn.Channel().PublishWithContext(ctx, ExchangeCost, routingKey, false, false, pub); err != nil {
		return fmt.Errorf("publish to %s: %w", routingKey, err)
	}
	return nil
}
