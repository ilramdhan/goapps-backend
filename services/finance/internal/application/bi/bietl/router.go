package bietl

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/datasource"
)

// Source codes in bi_data_source that select an ETL runner.
const (
	SourceERPOracle  = "ERP_ORACLE"
	SourceHRISOracle = "HRIS_ORACLE"
)

// Runner is the ETL contract (identical to job.BIETLRunner; declared here because the job
// package imports bietl).
type Runner interface {
	Load(ctx context.Context, targetType, sourceView string, sourceID uuid.UUID) (int, error)
}

// SourceLookup resolves a data source by id.
type SourceLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*datasource.DataSource, error)
}

// Router dispatches ETL loads to the runner registered for the job's data source + target_type,
// falling back to the runner registered for the source alone (HD-01/HD-02, PP-03).
// A source without a registered runner (e.g. HRIS env not set) fails with a clear message instead
// of silently falling back to the ERP runner.
type Router struct {
	lookup  SourceLookup
	runners map[string]Runner
	targets map[targetKey]Runner
}

// targetKey pairs a data source code with a job target_type (PP-03).
type targetKey struct{ source, targetType string }

// NewRouter builds a Router. Runners are added with Register; nil runners are ignored.
func NewRouter(lookup SourceLookup) *Router {
	return &Router{lookup: lookup, runners: map[string]Runner{}, targets: map[targetKey]Runner{}}
}

// Register installs the runner for a source code. A nil runner leaves the source unregistered.
func (r *Router) Register(sourceCode string, runner Runner) {
	if runner != nil {
		r.runners[sourceCode] = runner
	}
}

// RegisterTarget installs the runner for a source code + target_type pair (PP-03). It takes
// precedence over the per-source runner from Register, which remains the fallback. A nil runner
// leaves the pair unregistered.
func (r *Router) RegisterTarget(sourceCode, targetType string, runner Runner) {
	if runner != nil {
		r.targets[targetKey{sourceCode, targetType}] = runner
	}
}

// Load implements job.BIETLRunner.
func (r *Router) Load(ctx context.Context, targetType, sourceView string, sourceID uuid.UUID) (int, error) {
	ds, err := r.lookup.GetByID(ctx, sourceID)
	if err != nil {
		return 0, fmt.Errorf("resolve data source: %w", err)
	}
	runner, ok := r.targets[targetKey{ds.Code, targetType}]
	if !ok {
		runner, ok = r.runners[ds.Code]
	}
	if !ok {
		return 0, fmt.Errorf("ETL runner for data source %s is not configured or its connection is unavailable", ds.Code)
	}
	return runner.Load(ctx, targetType, sourceView, sourceID)
}
