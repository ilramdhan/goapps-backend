// Package lookupsyncjob queues and tracks master-lookup-sync apply jobs: one
// parent job.Execution plus one child per product that has applicable changes,
// each child published to RabbitMQ independently and applied by the worker
// (worker.ProductLookupSyncHandler → lookupsync.Applier.ApplyProduct). Shaped
// after internal/application/productparambulk's RequestBulkEditHandler.
package lookupsyncjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// ErrPublisherUnavailable is returned when the finance service has no working
// RabbitMQ publisher, so no lookup sync job can be queued.
var ErrPublisherUnavailable = errors.New("message queue unavailable: RabbitMQ not connected " +
	"(finance service could not reach the broker at startup; check RabbitMQ health and restart the finance service)")

// ErrNothingToApply is returned when no product in scope has an applicable change.
var ErrNothingToApply = errors.New("no product in scope has lookup values to sync")

// ErrInvalidJobID is returned when a job id is not a valid UUID.
var ErrInvalidJobID = errors.New("invalid job_id")

// JobPublisher abstracts the RabbitMQ publisher for testability.
type JobPublisher interface {
	PublishProductLookupSync(ctx context.Context, jobID string, productSysID int64, scopeJSON, fingerprint, createdBy string) error
}

// ScopeWalker streams the plan of a scope (lookupsync.Planner.Walk).
type ScopeWalker interface {
	Walk(ctx context.Context, scope lookupsync.Scope, visit func(lookupsync.Item) error) (lookupsync.Summary, error)
}

// ChildParams is the JSON shape persisted on each child job's params.
type ChildParams struct {
	ProductSysID int64 `json:"product_sys_id"`
}

// ChildResult is the JSON shape the worker stores as a child's result_summary.
type ChildResult struct {
	ProductSysID  int64  `json:"product_sys_id"`
	ProductCode   string `json:"product_code,omitempty"`
	ValuesWritten int    `json:"values_written"`
	ValuesCleared int    `json:"values_cleared"`
	Skipped       bool   `json:"skipped,omitempty"`
	SkipReason    string `json:"skip_reason,omitempty"`
}

// EncodeScope JSON-encodes scope without its product list: each child
// re-plans scope ∩ {its product}, so the list is redundant on the wire.
func EncodeScope(scope lookupsync.Scope) (string, error) {
	scope.ProductSysIDs = nil
	b, err := json.Marshal(scope)
	if err != nil {
		return "", fmt.Errorf("encode lookup sync scope: %w", err)
	}
	return string(b), nil
}

// DecodeScope is the inverse of EncodeScope.
func DecodeScope(s string) (lookupsync.Scope, error) {
	var scope lookupsync.Scope
	if s == "" {
		return scope, nil
	}
	if err := json.Unmarshal([]byte(s), &scope); err != nil {
		return lookupsync.Scope{}, fmt.Errorf("decode lookup sync scope: %w", err)
	}
	return scope, nil
}

// Enqueuer queues a lookup sync apply job.
type Enqueuer struct {
	jobRepo   job.Repository
	publisher JobPublisher
	walker    ScopeWalker
}

// NewEnqueuer constructs the enqueuer. publisher may be nil (→ ErrPublisherUnavailable).
func NewEnqueuer(jobRepo job.Repository, publisher JobPublisher, walker ScopeWalker) *Enqueuer {
	return &Enqueuer{jobRepo: jobRepo, publisher: publisher, walker: walker}
}

// Enqueue walks scope, creates a parent job plus one child per product with
// applicable changes, and publishes every child. A per-child publish failure
// fails only that child (mirrors RequestBulkEditHandler.publishChildren).
func (e *Enqueuer) Enqueue(ctx context.Context, scope lookupsync.Scope, fingerprint, actor string) (*job.Execution, error) {
	if e.publisher == nil {
		return nil, ErrPublisherUnavailable
	}
	if actor == "" {
		return nil, errors.New("created by is required")
	}
	ids, err := e.productsWithChanges(ctx, scope)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, ErrNothingToApply
	}
	scopeJSON, err := EncodeScope(scope)
	if err != nil {
		return nil, err
	}
	parentParams, err := json.Marshal(map[string]any{
		"product_count": len(ids), "scope": json.RawMessage(scopeJSON), "plan_fingerprint": fingerprint,
	})
	if err != nil {
		return nil, fmt.Errorf("encode parent params: %w", err)
	}
	parent, err := job.NewParentExecution(job.TypeProductLookupSync, "", "", actor, 5, parentParams, len(ids))
	if err != nil {
		return nil, fmt.Errorf("create parent execution: %w", err)
	}
	if err := e.jobRepo.Create(ctx, parent); err != nil {
		return nil, fmt.Errorf("persist parent job: %w", err)
	}
	children := make([]*job.Execution, 0, len(ids))
	for _, id := range ids {
		cp, cpErr := json.Marshal(ChildParams{ProductSysID: id})
		if cpErr != nil {
			return nil, fmt.Errorf("encode child params: %w", cpErr)
		}
		child, childErr := job.NewChildExecution(job.TypeProductLookupSync, "", "", actor, 5, cp, parent.ID())
		if childErr != nil {
			return nil, fmt.Errorf("create child execution: %w", childErr)
		}
		children = append(children, child)
	}
	if err := e.jobRepo.CreateChildren(ctx, children); err != nil {
		return nil, fmt.Errorf("persist child jobs: %w", err)
	}
	return e.publishChildren(ctx, ids, scopeJSON, fingerprint, actor, parent, children), nil
}

// productsWithChanges returns, in plan order and deduplicated, every product
// with at least one applicable (non-skipped) change.
func (e *Enqueuer) productsWithChanges(ctx context.Context, scope lookupsync.Scope) ([]int64, error) {
	seen := make(map[int64]struct{})
	var ids []int64
	_, err := e.walker.Walk(ctx, scope, func(it lookupsync.Item) error {
		if it.SkipReason != "" || len(it.Changes) == 0 {
			return nil
		}
		if _, ok := seen[it.ProductSysID]; ok {
			return nil
		}
		seen[it.ProductSysID] = struct{}{}
		ids = append(ids, it.ProductSysID)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("plan lookup sync: %w", err)
	}
	return ids, nil
}

func (e *Enqueuer) publishChildren(
	ctx context.Context, ids []int64, scopeJSON, fingerprint, actor string, parent *job.Execution, children []*job.Execution,
) *job.Execution {
	anyFailed := false
	for i, child := range children {
		if err := e.publisher.PublishProductLookupSync(ctx, child.ID().String(), ids[i], scopeJSON, fingerprint, actor); err != nil {
			anyFailed = true
			e.failChild(ctx, child, err)
			if _, incErr := e.jobRepo.IncrementChildProgress(ctx, parent.ID(), false); incErr != nil {
				log.Warn().Err(incErr).Str("parent_job_id", parent.ID().String()).
					Msg("lookup sync: failed to increment parent failed-children counter")
			}
		}
	}
	if !anyFailed {
		return parent
	}
	refreshed, err := e.jobRepo.GetByID(ctx, parent.ID())
	if err != nil {
		log.Warn().Err(err).Str("job_id", parent.ID().String()).
			Msg("lookup sync: failed to refresh parent job after publish failures")
		return parent
	}
	return refreshed
}

func (e *Enqueuer) failChild(ctx context.Context, child *job.Execution, publishErr error) {
	if err := child.Fail("failed to publish to queue: " + publishErr.Error()); err != nil {
		return
	}
	if err := e.jobRepo.UpdateStatus(ctx, child); err != nil {
		log.Warn().Err(err).Str("child_job_id", child.ID().String()).
			Msg("lookup sync: failed to record child publish failure")
	}
}
