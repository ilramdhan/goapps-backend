package costproductparameter

import (
	"errors"

	"github.com/google/uuid"
)

// BulkOpKind discriminates one BulkOp's action, mirroring the proto's
// BulkParamOperation oneof (add_applicable / remove_applicable / upsert_value).
type BulkOpKind string

// Allowed BulkOpKind values.
const (
	BulkOpAddApplicable    BulkOpKind = "ADD_APPLICABLE"
	BulkOpRemoveApplicable BulkOpKind = "REMOVE_APPLICABLE"
	BulkOpUpsertValue      BulkOpKind = "UPSERT_VALUE"
)

// ErrBulkSkippedNotApplicable is returned by ApplyBulkOperations for an
// UpsertValue op whose param is not (yet) CAPP-applicable to the target
// product and the caller requested skip_missing_applicable=true. It is not a
// hard failure of the product's whole op sequence — ApplyBulkOperations
// treats it as "skip this one op, keep applying the rest", but still reports
// it back to the caller (see ApplyBulkOperations' doc comment) so the bulk
// job's per-product outcome is never silent.
var ErrBulkSkippedNotApplicable = errors.New("param not applicable to product — skipped (skip_missing_applicable=true)")

// BulkOp is one step of an ApplyBulkOperations sequence, applied in order
// against a single product inside one DB transaction.
type BulkOp struct {
	Kind BulkOpKind

	ParamID uuid.UUID

	// AddApplicable fields.
	IsRequired   bool
	DisplayOrder *int32 // nil = inherit mst_parameter.display_order

	// UpsertValue fields — exactly one of these three is set, matching
	// EnsureValueShape's contract.
	ValueNumeric *string
	ValueText    *string
	ValueFlag    *bool
}

// BulkOpOutcome records, for one BulkOp applied to one product, whether it
// was applied or skipped — returned by ApplyBulkOperations so the caller
// (the async worker) can report skipped upsert-value ops even when the
// overall product operation sequence otherwise succeeded.
type BulkOpOutcome struct {
	ParamID uuid.UUID
	Kind    BulkOpKind
	Skipped bool
	// Reason is set only when Skipped is true.
	Reason string
}
