package etl

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/ppc/internal/application/demand"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

// SoPendingSource lists the full pending sales-order backlog from Oracle. A nil
// implementation (Oracle unavailable) is tolerated by the usecase.
type SoPendingSource interface {
	ListSoPending(ctx context.Context) ([]oracle.SoPendingRow, error)
}

// SoStagingRepo is the write surface the SO ETL needs.
type SoStagingRepo interface {
	ReplaceSalesOrderStaging(ctx context.Context, rows []postgres.SoStagingRow) error
}

// StagingProductResolver resolves freshly synced staging rows to finance
// products. Implemented by the demand application service; may be nil.
type StagingProductResolver interface {
	ResolveStaging(ctx context.Context) (demand.ResolveStagingResult, error)
}

// SoResult summarizes a sales-order staging ETL run.
type SoResult struct {
	Replaced int
	OracleUp bool
	// Resolved is how many staging rows received a product resolution after the
	// replace. Zero when no resolver is wired or finance is degraded.
	Resolved int64
}

// SoStagingETL is the full-replace sales-order staging ETL usecase.
type SoStagingETL struct {
	source   SoPendingSource
	repo     SoStagingRepo
	resolver StagingProductResolver
}

// NewSoStagingETL builds the usecase. source may be nil (Oracle unavailable).
func NewSoStagingETL(source SoPendingSource, repo SoStagingRepo) *SoStagingETL {
	return &SoStagingETL{source: source, repo: repo}
}

// WithProductResolver attaches the post-sync product resolution pass. A nil
// resolver leaves the freshly replaced rows UNRESOLVED for the lazy read-path
// pass to pick up.
func (e *SoStagingETL) WithProductResolver(resolver StagingProductResolver) *SoStagingETL {
	e.resolver = resolver
	return e
}

// Run pulls the full pending backlog and full-replaces sales_order_staging,
// preserving pull state. A nil source (Oracle unavailable) is a no-op.
func (e *SoStagingETL) Run(ctx context.Context) (SoResult, error) {
	res := SoResult{}
	if e.source == nil {
		log.Info().Msg("so staging ETL: oracle unavailable, skipping run")
		return res, nil
	}

	rows, err := e.source.ListSoPending(ctx)
	if err != nil {
		return res, err
	}
	res.OracleUp = true

	staging := make([]postgres.SoStagingRow, 0, len(rows))
	for i := range rows {
		staging = append(staging, mapSoPendingRow(rows[i]))
	}
	if err := e.repo.ReplaceSalesOrderStaging(ctx, staging); err != nil {
		return res, err
	}
	res.Replaced = len(staging)
	res.Resolved = e.resolveProducts(ctx)
	return res, nil
}

// resolveProducts runs the post-sync product resolution pass. Failures are
// logged, never propagated: a completed sync must not be reported as failed
// because finance was unreachable, and the lazy read-path pass will retry.
func (e *SoStagingETL) resolveProducts(ctx context.Context) int64 {
	if e.resolver == nil {
		return 0
	}
	out, err := e.resolver.ResolveStaging(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("so staging ETL: product resolution failed, rows left unresolved")
		return 0
	}
	if out.Skipped {
		return 0
	}
	log.Info().
		Int("pairs", out.Pairs).
		Int("auto", out.Auto).
		Int("ambiguous", out.Ambiguous).
		Int("not_found", out.NotFound).
		Int64("rows_updated", out.RowsUpdated).
		Msg("so staging ETL: product resolution completed")
	return out.RowsUpdated
}

// mapSoPendingRow maps an Oracle pending row to a staging row.
func mapSoPendingRow(r oracle.SoPendingRow) postgres.SoStagingRow {
	return postgres.SoStagingRow{
		ContractNo:    r.ContractNo,
		ContractDate:  r.ContractDate,
		ContractSysID: r.ContractSysID,
		CustomerCode:  r.CustomerCode,
		CustomerName:  r.CustomerName,
		ItemCode:      r.ItemCode,
		GradeCode:     r.GradeCode,
		ShadeCode:     r.ShadeCode,
		QtyOrdered:    r.QtyOrdered,
		QtyDelivered:  r.QtyDelivered,
		QtyRemaining:  r.QtyRemaining,
		Deadline:      r.Deadline,
		MergeNo:       r.MergeNo,
		Term:          r.Term,
		Rate:          r.Rate,
		Currency:      r.Currency,
		BlockedStatus: r.BlockedStatus,
		OutstandingAR: r.OutstandingAR,
	}
}
