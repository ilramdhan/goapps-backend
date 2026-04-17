package rabbitmq

import (
	"context"

	"github.com/rs/zerolog"
)

// JobPublisherAdapter adapts the RabbitMQ publisher to the application-layer JobPublisher interface.
type JobPublisherAdapter struct {
	publisher *Publisher
	logger    zerolog.Logger
}

// NewJobPublisherAdapter creates a new JobPublisherAdapter.
func NewJobPublisherAdapter(publisher *Publisher, logger zerolog.Logger) *JobPublisherAdapter {
	return &JobPublisherAdapter{
		publisher: publisher,
		logger:    logger,
	}
}

// PublishOracleSync publishes an Oracle sync job message.
func (a *JobPublisherAdapter) PublishOracleSync(ctx context.Context, jobID string, period string, createdBy string) error {
	msg := JobMessage{
		JobID:     jobID,
		JobType:   "oracle_sync",
		Subtype:   "item_cons_stk_po",
		Period:    period,
		CreatedBy: createdBy,
	}
	return a.publisher.PublishJob(ctx, RoutingKeyOracleSync, msg)
}
