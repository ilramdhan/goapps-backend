package rabbitmq

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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

// PublishRMCostExport publishes an RM cost async export job message.
// requestingUserID is the notification recipient when the file is ready.
func (a *JobPublisherAdapter) PublishRMCostExport(
	ctx context.Context,
	jobID, period, rmType, groupHeadID, search, requestingUserID, createdBy string,
) error {
	msg := JobMessage{
		JobID:            jobID,
		JobType:          "rm_cost_export",
		Subtype:          "xlsx",
		Period:           period,
		CreatedBy:        createdBy,
		RequestingUserID: requestingUserID,
		Search:           search,
		RMType:           rmType,
		GroupHeadID:      groupHeadID,
	}
	return a.publisher.PublishJob(ctx, RoutingKeyRMCostExport, msg)
}

// PublishImportJob publishes a costing data import job message.
// jobID is the int64 primary key from cost_import_job. entity is the entity type (e.g. "product_master").
// requestingUserID is the UUID of the user who submitted the import; used to route
// the completion notification. Empty string is safe (notification is skipped).
func (a *JobPublisherAdapter) PublishImportJob(ctx context.Context, jobID int64, entity, requestingUserID string) error {
	msg := JobMessage{
		JobID:            fmt.Sprintf("%d", jobID),
		JobType:          "costing_import",
		Subtype:          entity,
		RequestingUserID: requestingUserID,
	}
	return a.publisher.PublishJob(ctx, RoutingKeyImportJob, msg)
}

// PublishRMCostCalculation publishes an RM landed-cost calculation job message.
// groupHeadID is optional (nil means recalculate every active group for the period).
func (a *JobPublisherAdapter) PublishRMCostCalculation(
	ctx context.Context,
	jobID string,
	period string,
	groupHeadID *uuid.UUID,
	reason string,
	createdBy string,
) error {
	msg := JobMessage{
		JobID:     jobID,
		JobType:   "rm_cost_calculation",
		Subtype:   "landed_cost",
		Period:    period,
		CreatedBy: createdBy,
		Reason:    reason,
	}
	if groupHeadID != nil {
		msg.GroupHeadID = groupHeadID.String()
	}
	return a.publisher.PublishJob(ctx, RoutingKeyRMCostCalc, msg)
}
