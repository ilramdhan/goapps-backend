// Package customer provides application-layer use cases for the PPC customer master.
package customer

import (
	"context"

	customerdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/customer"
)

// SyncResult summarizes one Oracle customer-sync run.
type SyncResult struct {
	// Inserted counts customers created by this run.
	Inserted int
	// Updated counts existing customers whose source-owned fields changed.
	Updated int
	// Unchanged counts rows the source re-sent identically, plus MANUAL rows the
	// sync deliberately refuses to overwrite.
	Unchanged int
	// SourceUsed is false when Oracle was unconfigured or unreadable and the run
	// degraded to a no-op.
	SourceUsed bool
}

// SyncUsecase pulls the Orion customer master into ppc_db. It is idempotent:
// re-running with unchanged source data yields Unchanged for every row.
type SyncUsecase struct {
	repo   customerdomain.Repository
	source customerdomain.Source
}

// NewSyncUsecase builds the customer-sync usecase. A nil source (Oracle
// unconfigured) makes Sync a no-op rather than an error.
func NewSyncUsecase(repo customerdomain.Repository, source customerdomain.Source) *SyncUsecase {
	return &SyncUsecase{repo: repo, source: source}
}

// Sync reads every customer from the source and upserts it by code. A source
// failure is returned so the caller can surface it — unlike the machine sync,
// there is no second source to degrade to. A repository failure aborts the run
// and returns the counts accumulated so far.
func (u *SyncUsecase) Sync(ctx context.Context) (SyncResult, error) {
	res := SyncResult{}
	if u == nil || u.source == nil {
		return res, nil
	}

	rows, err := u.source.ListCustomers(ctx)
	if err != nil {
		return res, err
	}
	if len(rows) == 0 {
		return res, nil
	}
	res.SourceUsed = true

	for _, src := range rows {
		outcome, upsertErr := u.repo.UpsertSourced(ctx, src)
		if upsertErr != nil {
			return res, upsertErr
		}
		switch outcome {
		case customerdomain.OutcomeInserted:
			res.Inserted++
		case customerdomain.OutcomeUpdated:
			res.Updated++
		case customerdomain.OutcomeSkipped:
			res.Unchanged++
		}
	}
	return res, nil
}
