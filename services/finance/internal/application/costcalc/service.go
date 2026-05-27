package costcalc

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// Service is the dependency-holder + entry point for cost calc operations.
// Constructed once at startup and reused across requests. All methods are safe
// for concurrent use provided the injected repositories and loader are too.
type Service struct {
	jobRepo       costcalcdom.JobRepository
	chunkRepo     costcalcdom.ChunkRepository
	productRepo   costcalcdom.JobProductRepository
	resultRepo    costcalcdom.ResultRepository
	auditRepo     costcalcdom.AuditHistoryRepository
	loader        ProductLoader
	cache         *evaluator.Cache
	auditEmitter  AuditEmitter
	jobTriggerPub JobTriggerPublisher
}

// JobTriggerPublisher signals the orchestrator (via RMQ) to plan + execute a
// non-SINGLE_PRODUCT calc job. Pass nil to fall back to ErrScopeNotYetSupported
// — useful in tests and for dev environments without RMQ.
type JobTriggerPublisher interface {
	PublishJobTriggered(ctx context.Context, jobID int64) error
}

// AuditEmitter is the optional sink for COST_CALC_* events written to the
// existing cost_audit_log stream. nil means skip — useful in tests and in the
// initial S8b wiring where the orchestrator hasn't picked an event taxonomy.
//
// The shape is deliberately minimal: a concrete adapter at wire time can map
// to costauditlog.NewInput, slog.Logger, or any other sink without leaking
// those packages into the calc engine.
type AuditEmitter interface {
	Emit(ctx context.Context, e AuditEvent) error
}

// AuditEvent is the shape ProcessChunk + TriggerJobHandler emit on lifecycle
// transitions. Payload is a JSONB-ready blob (nil when not interesting).
type AuditEvent struct {
	EventType  string
	EntityKind string
	EntityID   string
	Actor      string
	Message    string
	Payload    []byte
}

// NewService constructs the calc engine service. Pass auditEmitter=nil to skip
// the cost_audit_log side-channel. Pass jobTriggerPub=nil to disable the
// orchestrator hand-off (non-SINGLE_PRODUCT scopes will return
// ErrScopeNotYetSupported in that case).
func NewService(
	jobRepo costcalcdom.JobRepository,
	chunkRepo costcalcdom.ChunkRepository,
	productRepo costcalcdom.JobProductRepository,
	resultRepo costcalcdom.ResultRepository,
	auditRepo costcalcdom.AuditHistoryRepository,
	loader ProductLoader,
	cache *evaluator.Cache,
	auditEmitter AuditEmitter,
	jobTriggerPub JobTriggerPublisher,
) *Service {
	return &Service{
		jobRepo:       jobRepo,
		chunkRepo:     chunkRepo,
		productRepo:   productRepo,
		resultRepo:    resultRepo,
		auditRepo:     auditRepo,
		loader:        loader,
		cache:         cache,
		auditEmitter:  auditEmitter,
		jobTriggerPub: jobTriggerPub,
	}
}

// emitAudit is a best-effort fire-and-forget helper: nil emitter means skip,
// emit errors are swallowed (the business operation already succeeded). Caller
// is expected to log via the emitter implementation.
func (s *Service) emitAudit(ctx context.Context, e AuditEvent) {
	if s.auditEmitter == nil {
		return
	}
	if e := s.auditEmitter.Emit(ctx, e); e != nil {
		_ = e
	}
}
