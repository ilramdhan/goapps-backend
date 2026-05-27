package rmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// CronJobPublisher publishes JobTriggeredEvent to the finance.cost exchange
// from inside the orchestrator (used by the monthly cron auto-trigger,
// S8e.6). Mirrors the finance-side CostJobPublisher surface so the
// orchestrator's cron does not depend on the finance service being up.
type CronJobPublisher struct {
	mu   sync.Mutex
	conn *Connection
}

// NewCronJobPublisher constructs a publisher bound to the given connection.
// The topology is assumed to already be declared via DeclareTopology in main.
func NewCronJobPublisher(conn *Connection) *CronJobPublisher {
	return &CronJobPublisher{conn: conn}
}

type cronJobTriggeredEvent struct {
	JobID int64 `json:"job_id"`
}

// PublishJobTriggered signals the coordinator that a new calc job is queued
// and ready for planning + dispatch.
func (p *CronJobPublisher) PublishJobTriggered(ctx context.Context, jobID int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	body, err := json.Marshal(cronJobTriggeredEvent{JobID: jobID})
	if err != nil {
		return fmt.Errorf("marshal job_triggered: %w", err)
	}
	pub := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}
	if err := p.conn.Channel().PublishWithContext(ctx, ExchangeCost, RoutingKeyJobTriggered, false, false, pub); err != nil {
		return fmt.Errorf("publish job_triggered: %w", err)
	}
	return nil
}
