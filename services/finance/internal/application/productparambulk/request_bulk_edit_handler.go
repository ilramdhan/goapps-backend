// Package productparambulk implements the Bulk Edit Product Params (F4) use
// case: fanning a list of add-applicable/remove-applicable/upsert-value param
// operations out across many cost_product_master products as one parent
// job.Execution plus one child job.Execution per product_sys_id, each
// published to RabbitMQ independently. Cloned in shape from
// internal/application/mbheadbulk's RequestBulkTransitionHandler — see that
// package's doc comment for the precedent this mirrors.
package productparambulk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// maxProducts mirrors the proto's buf.validate cap on
// BulkEditProductParamsRequest.product_sys_ids (1-500 items) — enforced again
// here defensively since Handle must never trust the transport layer alone.
const maxProducts = 500

// ErrPublisherUnavailable is returned when the finance service has no working
// RabbitMQ publisher, so no bulk edit job can be queued.
var ErrPublisherUnavailable = errors.New("message queue unavailable: RabbitMQ not connected " +
	"(finance service could not reach the broker at startup; check RabbitMQ health and restart the finance service)")

// ErrNoProducts is returned when the command carries zero product_sys_ids.
var ErrNoProducts = errors.New("product_sys_ids is required")

// ErrTooManyProducts is returned when the command exceeds maxProducts.
var ErrTooManyProducts = fmt.Errorf("product_sys_ids exceeds the maximum of %d items", maxProducts)

// ErrNoOperations is returned when the command carries zero operations.
var ErrNoOperations = errors.New("operations is required")

// ErrProductNotFound is returned when one of the requested product_sys_ids
// does not exist in cost_product_master.
var ErrProductNotFound = errors.New("product not found")

// ErrInvalidOperation is returned when an OperationDTO's shape does not match
// exactly one of add_applicable / remove_applicable / upsert_value, or a
// required field within the chosen op is missing/invalid.
var ErrInvalidOperation = errors.New("invalid bulk param operation")

// OpKind discriminates OperationDTO's payload, mirroring the proto's
// BulkParamOperation oneof and costproductparameter.BulkOpKind.
type OpKind string

// Allowed OpKind values.
const (
	OpAddApplicable    OpKind = "ADD_APPLICABLE"
	OpRemoveApplicable OpKind = "REMOVE_APPLICABLE"
	OpUpsertValue      OpKind = "UPSERT_VALUE"
)

// OperationDTO is the transport-agnostic, JSON-serializable shape of one bulk
// param operation. It is built by the gRPC delivery layer from the proto
// request, persisted verbatim (JSON-encoded) as the parent/child jobs'
// params and as each RabbitMQ message's Operations field, and decoded again
// by the async worker into []costproductparameter.BulkOp.
type OperationDTO struct {
	Kind         OpKind  `json:"kind"`
	ParamID      string  `json:"param_id"`
	IsRequired   bool    `json:"is_required,omitempty"`
	DisplayOrder *int32  `json:"display_order,omitempty"`
	ValueNumeric *string `json:"value_numeric,omitempty"`
	ValueText    *string `json:"value_text,omitempty"`
	ValueFlag    *bool   `json:"value_flag,omitempty"`
}

// BulkEditJobPublisher abstracts the RabbitMQ publisher dependency for testability.
type BulkEditJobPublisher interface {
	PublishProductParamBulk(ctx context.Context, jobID string, productSysID int64, operationsJSON string, skipMissingApplicable bool, createdBy string) error
}

// ProductChecker abstracts the product-existence check Handle uses to reject
// unknown product_sys_ids before any job is created.
type ProductChecker interface {
	ProductExists(ctx context.Context, productSysID int64) (bool, error)
}

// RequestBulkEditCommand carries the validated input for queueing a bulk
// product param edit.
type RequestBulkEditCommand struct {
	ProductSysIDs         []int64
	Operations            []OperationDTO
	SkipMissingApplicable bool
	CreatedBy             string
}

// RequestBulkEditResult is the queue acknowledgement.
type RequestBulkEditResult struct {
	Execution *job.Execution
}

// RequestBulkEditHandler queues an asynchronous bulk product param edit job.
type RequestBulkEditHandler struct {
	jobRepo   job.Repository
	publisher BulkEditJobPublisher
	products  ProductChecker
}

// NewRequestBulkEditHandler constructs the handler.
func NewRequestBulkEditHandler(jobRepo job.Repository, publisher BulkEditJobPublisher, products ProductChecker) *RequestBulkEditHandler {
	return &RequestBulkEditHandler{jobRepo: jobRepo, publisher: publisher, products: products}
}

// childParams is the JSON shape persisted on each child job.Execution's
// Params — read back by the gRPC delivery layer (ListBulkProductParamJobFailures)
// to report which product a failed child was targeting.
type childParams struct {
	ProductSysID int64 `json:"product_sys_id"`
}

// Handle creates one parent job.Execution (total_children = len(cmd.ProductSysIDs))
// plus N child job.Execution rows (one per product), then publishes each
// child to RabbitMQ independently. A per-child publish failure fails only
// that child — it never aborts the rest of the batch or the parent job,
// matching RequestBulkTransitionHandler.publishChildren's pattern exactly.
func (h *RequestBulkEditHandler) Handle(ctx context.Context, cmd RequestBulkEditCommand) (*RequestBulkEditResult, error) {
	if err := h.validate(ctx, cmd); err != nil {
		return nil, err
	}

	operationsJSON, err := json.Marshal(cmd.Operations)
	if err != nil {
		return nil, fmt.Errorf("encode operations: %w", err)
	}

	parentParams, err := json.Marshal(map[string]any{
		"product_count":           len(cmd.ProductSysIDs),
		"operations":              cmd.Operations,
		"skip_missing_applicable": cmd.SkipMissingApplicable,
	})
	if err != nil {
		return nil, fmt.Errorf("encode parent params: %w", err)
	}
	parent, err := job.NewParentExecution(job.TypeProductParamBulk, "", "", cmd.CreatedBy, 5, parentParams, len(cmd.ProductSysIDs))
	if err != nil {
		return nil, fmt.Errorf("create parent execution: %w", err)
	}
	if err := h.jobRepo.Create(ctx, parent); err != nil {
		return nil, fmt.Errorf("persist parent job: %w", err)
	}

	children := make([]*job.Execution, 0, len(cmd.ProductSysIDs))
	for _, productSysID := range cmd.ProductSysIDs {
		childParamsJSON, cpErr := json.Marshal(childParams{ProductSysID: productSysID})
		if cpErr != nil {
			return nil, fmt.Errorf("encode child params: %w", cpErr)
		}
		child, childErr := job.NewChildExecution(job.TypeProductParamBulk, "", "", cmd.CreatedBy, 5, childParamsJSON, parent.ID())
		if childErr != nil {
			return nil, fmt.Errorf("create child execution: %w", childErr)
		}
		children = append(children, child)
	}
	if err := h.jobRepo.CreateChildren(ctx, children); err != nil {
		return nil, fmt.Errorf("persist child jobs: %w", err)
	}

	parent = h.publishChildren(ctx, cmd, string(operationsJSON), parent, children)

	return &RequestBulkEditResult{Execution: parent}, nil
}

// publishChildren publishes each child independently to RabbitMQ. A publish
// failure fails only that one child — see
// mbheadbulk.RequestBulkTransitionHandler.publishChildren's doc comment for
// the full rationale, mirrored here verbatim.
func (h *RequestBulkEditHandler) publishChildren(
	ctx context.Context, cmd RequestBulkEditCommand, operationsJSON string, parent *job.Execution, children []*job.Execution,
) *job.Execution {
	anyPublishFailed := false
	for i, child := range children {
		productSysID := cmd.ProductSysIDs[i]
		if err := h.publisher.PublishProductParamBulk(ctx, child.ID().String(), productSysID, operationsJSON, cmd.SkipMissingApplicable, cmd.CreatedBy); err != nil {
			anyPublishFailed = true
			if failErr := h.failJob(ctx, child, err); failErr != nil {
				log.Warn().Err(failErr).Str("child_job_id", child.ID().String()).
					Msg("product param bulk: failed to record child publish failure")
			}
			if _, incErr := h.jobRepo.IncrementChildProgress(ctx, parent.ID(), false); incErr != nil {
				log.Warn().Err(incErr).Str("parent_job_id", parent.ID().String()).
					Msg("product param bulk: failed to increment parent failed-children counter")
			}
		}
	}
	if !anyPublishFailed {
		return parent
	}

	refreshed, getErr := h.jobRepo.GetByID(ctx, parent.ID())
	if getErr != nil {
		log.Warn().Err(getErr).Str("job_id", parent.ID().String()).
			Msg("product param bulk: failed to refresh parent job after publish failures")
		return parent
	}
	return refreshed
}

// validate checks the fields Handle needs before doing any work, including
// resolving every product_sys_id against cost_product_master so an unknown
// product is rejected up front rather than surfacing later as a per-child
// failure the caller has to dig for.
func (h *RequestBulkEditHandler) validate(ctx context.Context, cmd RequestBulkEditCommand) error {
	if h.publisher == nil {
		return ErrPublisherUnavailable
	}
	if len(cmd.ProductSysIDs) == 0 {
		return ErrNoProducts
	}
	if len(cmd.ProductSysIDs) > maxProducts {
		return ErrTooManyProducts
	}
	if len(cmd.Operations) == 0 {
		return ErrNoOperations
	}
	if cmd.CreatedBy == "" {
		return fmt.Errorf("created by is required")
	}
	for _, op := range cmd.Operations {
		if err := validateOperation(op); err != nil {
			return err
		}
	}
	return h.checkProductsExist(ctx, cmd.ProductSysIDs)
}

// checkProductsExist rejects the whole batch if any product_sys_id is unknown.
func (h *RequestBulkEditHandler) checkProductsExist(ctx context.Context, productSysIDs []int64) error {
	if h.products == nil {
		return nil
	}
	for _, id := range productSysIDs {
		exists, err := h.products.ProductExists(ctx, id)
		if err != nil {
			return fmt.Errorf("check product %d exists: %w", id, err)
		}
		if !exists {
			return fmt.Errorf("%w: product_sys_id %d", ErrProductNotFound, id)
		}
	}
	return nil
}

// validateOperation checks one OperationDTO's shape.
func validateOperation(op OperationDTO) error {
	if _, err := uuid.Parse(op.ParamID); err != nil {
		return fmt.Errorf("%w: invalid param_id %q", ErrInvalidOperation, op.ParamID)
	}
	switch op.Kind {
	case OpAddApplicable, OpRemoveApplicable:
		return nil
	case OpUpsertValue:
		return validateUpsertValueShape(op)
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidOperation, op.Kind)
	}
}

// validateUpsertValueShape checks that exactly one of value_numeric/value_text/
// value_flag is set on an UPSERT_VALUE op, mirroring
// costproductparameter.EnsureValueShape's count-must-equal-one rule (data-type
// cross-checking happens later against the real mst_parameter row, once the
// worker has resolved it — this is just shape validation at submit time).
func validateUpsertValueShape(op OperationDTO) error {
	count := 0
	if op.ValueNumeric != nil {
		count++
	}
	if op.ValueText != nil {
		count++
	}
	if op.ValueFlag != nil {
		count++
	}
	if count != 1 {
		return fmt.Errorf("%w: upsert_value for param %s must set exactly one of value_numeric/value_text/value_flag", ErrInvalidOperation, op.ParamID)
	}
	return nil
}

// failJob marks the job failed so it doesn't sit forever in QUEUED after a
// publish error.
func (h *RequestBulkEditHandler) failJob(ctx context.Context, exec *job.Execution, publishErr error) error {
	if failErr := exec.Fail("failed to publish to queue: " + publishErr.Error()); failErr == nil {
		if updErr := h.jobRepo.UpdateStatus(ctx, exec); updErr != nil {
			return errors.Join(fmt.Errorf("publish job: %w", publishErr), fmt.Errorf("persist failed: %w", updErr))
		}
	}
	return fmt.Errorf("publish job: %w", publishErr)
}
