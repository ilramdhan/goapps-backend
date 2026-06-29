package costimportetl

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
)

// RowEmitter receives a single parsed data row. The slice passed to it MAY be
// reused by the producer on the next call (see rowSink semantics in
// streamcopy.go), so an implementation that retains the values MUST copy them.
// Returning an error aborts the producing stream.
type RowEmitter func(row []string) error

// RowProducer drives parsed data rows into emit. It is supplied by the ETL
// orchestrator (e.g. a closure over container.streamCSVEntry or
// container.streamSheet) and called by the staging repository so that rows are
// streamed straight into COPY without ever being accumulated in a slice.
type RowProducer func(emit RowEmitter) error

// StagingRepository streams parsed import rows into the UNLOGGED stg_import_*
// tables via COPY. Each method scopes its rows to jobID and prepends job_id +
// row_num to every staged row; produce is pulled lazily so peak memory stays
// bounded regardless of the total row count. The returned count is the number
// of data rows copied into staging.
type StagingRepository interface {
	// CopyStagingProductMaster streams product-master rows into
	// stg_import_product_master.
	CopyStagingProductMaster(ctx context.Context, jobID int64, produce RowProducer) (int64, error)
	// CopyStagingProductParameter streams product-parameter value rows into
	// stg_import_product_parameter.
	CopyStagingProductParameter(ctx context.Context, jobID int64, produce RowProducer) (int64, error)
	// CopyStagingApplicableParam streams applicable-parameter rows into
	// stg_import_applicable_param.
	CopyStagingApplicableParam(ctx context.Context, jobID int64, produce RowProducer) (int64, error)
	// CopyStagingRouteHead streams route-head rows into stg_import_route_head.
	CopyStagingRouteHead(ctx context.Context, jobID int64, produce RowProducer) (int64, error)
	// CopyStagingRouteSeq streams route-sequence rows into stg_import_route_seq.
	CopyStagingRouteSeq(ctx context.Context, jobID int64, produce RowProducer) (int64, error)
	// CopyStagingRouteRM streams route raw-material rows into
	// stg_import_route_rm.
	CopyStagingRouteRM(ctx context.Context, jobID int64, produce RowProducer) (int64, error)
}

// Resolver runs the set-based resolution that promotes staged rows into the real
// costing tables, one dependency layer at a time. Each method runs in its own
// transaction (commit per layer): it first captures rows whose foreign-key
// references are missing or whose values fail to cast into stg_import_error, then
// upserts the valid rows joined to the already-resolved reference tables. The
// returned count is the number of target rows written (inserted or updated).
// Layers MUST be invoked in order 1..6 because each resolves the previous layer
// via JOIN.
type Resolver interface {
	// ResolveLayer1Products upserts cost_product_master from
	// stg_import_product_master (conflict key cpm_flex_02).
	ResolveLayer1Products(ctx context.Context, jobID int64, actor string) (int, error)
	// ResolveLayer2Params upserts cost_product_parameter from
	// stg_import_product_parameter (joined to master + mst_parameter).
	ResolveLayer2Params(ctx context.Context, jobID int64, actor string) (int, error)
	// ResolveLayer3Applicable upserts cost_product_applicable_param from
	// stg_import_applicable_param.
	ResolveLayer3Applicable(ctx context.Context, jobID int64, actor string) (int, error)
	// ResolveLayer4RouteHead upserts cost_route_head from stg_import_route_head
	// (LOCKED heads are left untouched).
	ResolveLayer4RouteHead(ctx context.Context, jobID int64, actor string) (int, error)
	// ResolveLayer5RouteSeq upserts cost_route_seq from stg_import_route_seq
	// (joined to the active head + node product).
	ResolveLayer5RouteSeq(ctx context.Context, jobID int64, actor string) (int, error)
	// ResolveLayer6RouteRM replaces cost_route_rm for the affected seq set from
	// stg_import_route_rm (delete-then-insert).
	ResolveLayer6RouteRM(ctx context.Context, jobID int64, actor string) (int, error)
}

// MasterOptionsSource supplies the set of valid codes registered for a
// lookup-master code. It is backed by the lookup-master registry
// (mst_lookup_master + the dynamically-named master table), the same source the
// lookup-masters admin page uses. Satisfied by lookupmaster.Repository.
type MasterOptionsSource interface {
	// ListMasterOptions returns the valid code+label options for masterCode.
	ListMasterOptions(ctx context.Context, masterCode string) ([]lookupmaster.MasterOption, error)
}

// MasterLookupCandidate is one distinct MASTER_LOOKUP parameter value staged for
// a job, paired with the registry code its value must exist in. The ETL handler
// validates each distinct value against MasterOptionsSource and rejects unknowns.
type MasterLookupCandidate struct {
	// MasterCode is the lookup-master registry code (mst_parameter.lookup_master_code).
	MasterCode string
	// ParamCode is the parameter whose value is a MASTER_LOOKUP reference.
	ParamCode string
	// Value is the staged value_text that must exist in the master's option set.
	Value string
}

// MasterLookupValidator enforces that MASTER_LOOKUP parameter values staged for a
// job exist in their registered master tables — the dynamic, per-master check the
// old params-only importer performed in Go (it cannot be a single static SQL JOIN
// because the master table name is resolved from the registry at runtime).
type MasterLookupValidator interface {
	// MasterLookupCandidates returns the distinct (master_code, param_code, value)
	// triples staged for jobID whose parameter is a MASTER_LOOKUP type with a
	// configured lookup_master_code and a non-empty value.
	MasterLookupCandidates(ctx context.Context, jobID int64) ([]MasterLookupCandidate, error)
	// RejectMasterLookupValues records one stg_import_error per rejected value
	// (message "unknown_master_value:<master>:<value>" so the report's dedicated
	// sheet picks it up) and deletes the matching staged product-parameter rows so
	// they are not imported by ResolveLayer2Params. Returns the rows removed.
	RejectMasterLookupValues(ctx context.Context, jobID int64, rejected []MasterLookupCandidate) (int, error)
}

// StagingError is one row-level resolve error captured in stg_import_error during
// set-based resolution. Its fields mirror what the costbulkimport error-report
// generator consumes (a sheet label, a row number, the offending row's key, and a
// message) so the ETL orchestrator can group these into per-sheet report results.
type StagingError struct {
	// Sheet names the staging source the error came from, e.g. "product_master"
	// or "route_seq".
	Sheet string
	// RowNumber is the 1-based row index within the staged sheet.
	RowNumber int32
	// KeyInfo is the business key of the offending row (e.g. the legacy product
	// id) used to locate it in the source file.
	KeyInfo string
	// Message is the human-readable (Indonesian) reason the row was rejected.
	Message string
}

// StagingMaintainer reads the row-level errors collected during resolution and
// tears down the staging rows once a job has finished.
type StagingMaintainer interface {
	// CollectErrors returns every row-level error captured in stg_import_error
	// for jobID, ordered by sheet then row number so the caller can group them
	// into per-sheet report results.
	CollectErrors(ctx context.Context, jobID int64) ([]StagingError, error)
	// CleanupStaging deletes every staged row (the six stg_import_* data tables
	// plus stg_import_error) for jobID in a single transaction.
	CleanupStaging(ctx context.Context, jobID int64) error
}
