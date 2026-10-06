package erpintegration

import "errors"

// Sentinel errors of the erpintegration domain (design Part 1 §5.4). The
// delivery layer (P6) maps them to gRPC codes:
//   - FailedPrecondition: ErrInvalidTransition, ErrPeriodNotLocked,
//     ErrPeriodPosted, ErrPreviewRequired, ErrPreviewExpired, ErrPreviewStale,
//     ErrConfirmMismatch, ErrFeatureDisabled, ErrWriterNotConfigured,
//     ErrCoverageGaps, ErrValidationFailed, ErrBatchFrozen,
//     ErrShadowNotPushable, ErrNeedsRepush, ErrControlTotalsMissing,
//     ErrActiveBatchExists
//   - InvalidArgument / FailedPrecondition: ErrDuplicateMapping,
//     ErrFlexOverflow, ErrCodeTooLong, ErrInvalidBatchStatus,
//     ErrInvalidBatchMode, ErrInvalidBatchPeriod, ErrInvalidBatchSeq,
//     ErrActorRequired, ErrInvalidRuleHash, ErrInvalidControlTotals
//   - NotFound: ErrBatchNotFound
//   - Aborted: ErrConcurrentRun, ErrStaleBatchStatus
//   - Unavailable (retryable): ErrOracleBusy (declared in oracle_writer_port.go)
var (
	ErrInvalidTransition   = errors.New("erpintegration: invalid batch status transition")
	ErrPeriodNotLocked     = errors.New("erpintegration: period is not locked")
	ErrPeriodPosted        = errors.New("erpintegration: period has posted ADJ heads")
	ErrPreviewRequired     = errors.New("erpintegration: preview required")
	ErrPreviewExpired      = errors.New("erpintegration: preview expired")
	ErrPreviewStale        = errors.New("erpintegration: preview is stale")
	ErrConfirmMismatch     = errors.New("erpintegration: confirmation does not match")
	ErrFeatureDisabled     = errors.New("erpintegration: feature disabled")
	ErrWriterNotConfigured = errors.New("erpintegration: oracle writer not configured")

	ErrCoverageGaps     = errors.New("erpintegration: coverage has blocking gaps")
	ErrValidationFailed = errors.New("erpintegration: validation failed")

	ErrDuplicateMapping = errors.New("erpintegration: duplicate ERP mapping")
	ErrFlexOverflow     = errors.New("erpintegration: FLEX text overflow")
	ErrCodeTooLong      = errors.New("erpintegration: ERP code too long")

	ErrBatchNotFound = errors.New("erpintegration: batch not found")
	ErrConcurrentRun = errors.New("erpintegration: another run holds the batch")

	// Batch aggregate errors (P3-T1).
	ErrInvalidBatchStatus   = errors.New("erpintegration: invalid batch status")
	ErrInvalidBatchMode     = errors.New("erpintegration: invalid batch mode")
	ErrInvalidBatchPeriod   = errors.New("erpintegration: invalid batch period (want YYYYMM)")
	ErrInvalidBatchSeq      = errors.New("erpintegration: batch seq must be >= 1")
	ErrActorRequired        = errors.New("erpintegration: actor is required (max 64 chars)")
	ErrInvalidRuleHash      = errors.New("erpintegration: rule hash must be 64 lower-case hex chars")
	ErrInvalidControlTotals = errors.New("erpintegration: invalid control totals")
	ErrControlTotalsMissing = errors.New("erpintegration: control totals and rule hash must be set before push")
	ErrBatchFrozen          = errors.New("erpintegration: batch totals and rule hash are frozen after push")
	ErrShadowNotPushable    = errors.New("erpintegration: a SHADOW batch can never be pushed")
	ErrNeedsRepush          = errors.New("erpintegration: period was unlocked after push; a new batch is required")
	ErrActiveBatchExists    = errors.New("erpintegration: an active or in-flight batch already exists for the period")
	ErrStaleBatchStatus     = errors.New("erpintegration: batch status changed concurrently")
)
