package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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
	// GroupHeadID scopes rm_cost_calculation jobs to a single group. Empty = all active groups.
	GroupHeadID string `json:"group_head_id,omitempty"`
	// Reason is the HistoryTriggerReason for rm_cost_calculation jobs.
	Reason string `json:"reason,omitempty"`
}

// Publisher publishes messages to RabbitMQ exchanges.
// Methods are safe for concurrent use.
type Publisher struct {
	mu     sync.Mutex
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
// Thread-safe: serializes access to the underlying AMQP channel.
func (p *Publisher) PublishJob(ctx context.Context, routingKey string, msg JobMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()

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
