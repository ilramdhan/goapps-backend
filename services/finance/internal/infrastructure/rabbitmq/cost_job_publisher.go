package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

// Orchestrator topology — kept in sync with finance-cost-orchestrator's
// rmq package. The orchestrator declares these too (idempotently), but we
// declare them here so finance can publish even if the orchestrator hasn't
// booted yet.
const (
	// CostExchange is the direct exchange for non-SINGLE_PRODUCT calc jobs.
	CostExchange = "finance.cost"
	// CostJobTriggeredRoutingKey is the routing key for JobTriggeredEvent.
	CostJobTriggeredRoutingKey = "finance.cost.job_triggered"
	// CostJobTriggeredQueue is the queue the orchestrator consumes from.
	CostJobTriggeredQueue = "finance.cost.job_triggered"
)

// CostJobPublisher publishes JobTriggeredEvent to the finance.cost exchange.
// Implements the costcalc.JobTriggerPublisher interface.
type CostJobPublisher struct {
	mu     sync.Mutex
	conn   *Connection
	logger zerolog.Logger
}

// NewCostJobPublisher constructs the publisher and idempotently declares the
// finance.cost exchange + job_triggered queue so finance can publish without
// requiring the orchestrator to have booted first.
func NewCostJobPublisher(conn *Connection, logger zerolog.Logger) (*CostJobPublisher, error) {
	ch := conn.Channel()
	if err := ch.ExchangeDeclare(CostExchange, "direct", true, false, false, false, nil); err != nil {
		return nil, fmt.Errorf("declare exchange %s: %w", CostExchange, err)
	}
	if _, err := ch.QueueDeclare(CostJobTriggeredQueue, true, false, false, false, nil); err != nil {
		return nil, fmt.Errorf("declare queue %s: %w", CostJobTriggeredQueue, err)
	}
	if err := ch.QueueBind(CostJobTriggeredQueue, CostJobTriggeredRoutingKey, CostExchange, false, nil); err != nil {
		return nil, fmt.Errorf("bind queue %s: %w", CostJobTriggeredQueue, err)
	}
	return &CostJobPublisher{conn: conn, logger: logger}, nil
}

type jobTriggeredEvent struct {
	JobID int64 `json:"job_id"`
}

// PublishJobTriggered signals the orchestrator that a new calc job is queued
// and ready for planning + dispatch.
func (p *CostJobPublisher) PublishJobTriggered(ctx context.Context, jobID int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	body, err := json.Marshal(jobTriggeredEvent{JobID: jobID})
	if err != nil {
		return fmt.Errorf("marshal job_triggered: %w", err)
	}
	pub := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}
	if err := p.conn.Channel().PublishWithContext(ctx, CostExchange, CostJobTriggeredRoutingKey, false, false, pub); err != nil {
		return fmt.Errorf("publish job_triggered: %w", err)
	}
	p.logger.Info().Int64("job_id", jobID).Msg("Cost job_triggered event published")
	return nil
}
