package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

// JobMessage represents a message published to the job queue.
type JobMessage struct {
	JobID     string `json:"job_id"`
	JobType   string `json:"job_type"`
	Subtype   string `json:"subtype"`
	Period    string `json:"period"`
	CreatedBy string `json:"created_by"`
}

// Publisher publishes messages to RabbitMQ exchanges.
type Publisher struct {
	conn   *Connection
	logger zerolog.Logger
}

// NewPublisher creates a new Publisher.
func NewPublisher(conn *Connection, logger zerolog.Logger) *Publisher {
	return &Publisher{
		conn:   conn,
		logger: logger,
	}
}

// PublishJob publishes a job message to the finance jobs exchange.
func (p *Publisher) PublishJob(ctx context.Context, routingKey string, msg JobMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal job message: %w", err)
	}

	publishing := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Body:         body,
	}

	if err := p.conn.Channel().PublishWithContext(ctx, ExchangeName, routingKey, false, false, publishing); err != nil {
		return fmt.Errorf("publish to %s/%s: %w", ExchangeName, routingKey, err)
	}

	p.logger.Info().
		Str("routing_key", routingKey).
		Str("job_id", msg.JobID).
		Str("job_type", msg.JobType).
		Msg("Job message published")

	return nil
}
