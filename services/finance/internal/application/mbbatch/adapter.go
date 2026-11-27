package mbbatch

import (
	"context"
	"database/sql"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// MBHeadReaderAdapter adapts *postgres.MBHeadRepository to MBHeadReader. Kept in the
// application layer (importing infrastructure directly) mirroring mbpush's precedent — no
// domain package wraps MB_BATCH candidate-selection concerns.
type MBHeadReaderAdapter struct {
	repo *postgres.MBHeadRepository
}

// NewMBHeadReaderAdapter constructs an MBHeadReaderAdapter.
func NewMBHeadReaderAdapter(repo *postgres.MBHeadRepository) *MBHeadReaderAdapter {
	return &MBHeadReaderAdapter{repo: repo}
}

var _ MBHeadReader = (*MBHeadReaderAdapter)(nil)

// ListValidated (batch candidates: active + transitively referenced inactive) maps postgres.MBHeadCandidate rows into mbbatch's own port type, including
// CurrentVersion (which mbpush's equivalent adapter deliberately omits).
func (a *MBHeadReaderAdapter) ListValidated(ctx context.Context) ([]MBHeadCandidate, error) {
	rows, err := a.repo.ListBatchCandidates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MBHeadCandidate, len(rows))
	for i, r := range rows {
		out[i] = MBHeadCandidate{
			MBHID:          r.MBHID,
			Code:           r.Code,
			Name:           r.Name,
			CostProductID:  r.CostProductID,
			IsBoughtout:    r.IsBoughtout,
			CurrentVersion: r.CurrentVersion,
		}
	}
	return out, nil
}

// MBEdgeReaderAdapter adapts *postgres.MBCompositionRepository to MBEdgeReader.
type MBEdgeReaderAdapter struct {
	repo *postgres.MBCompositionRepository
}

// NewMBEdgeReaderAdapter constructs an MBEdgeReaderAdapter.
func NewMBEdgeReaderAdapter(repo *postgres.MBCompositionRepository) *MBEdgeReaderAdapter {
	return &MBEdgeReaderAdapter{repo: repo}
}

var _ MBEdgeReader = (*MBEdgeReaderAdapter)(nil)

// ListMBEdgesBulk maps postgres.MBEdgeRow rows into mbbatch's own MBEdge port type, dropping
// rows with an empty RefMbhID (PRODUCT-type composition rows carry no MB-to-MB dependency).
func (a *MBEdgeReaderAdapter) ListMBEdgesBulk(ctx context.Context, mbhIDs []string, versions []int32) ([]MBEdge, error) {
	rows, err := a.repo.ListMBEdgesBulk(ctx, mbhIDs, versions)
	if err != nil {
		return nil, err
	}
	out := make([]MBEdge, 0, len(rows))
	for _, r := range rows {
		if r.RefMbhID == "" {
			continue
		}
		out = append(out, MBEdge{MBHID: r.MbhID, RefMBHID: r.RefMbhID})
	}
	return out, nil
}

// ResultWriterAdapter adapts *postgres.CostResultRepository to ResultWriter.
type ResultWriterAdapter struct {
	repo *postgres.CostResultRepository
}

// NewResultWriterAdapter constructs a ResultWriterAdapter.
func NewResultWriterAdapter(repo *postgres.CostResultRepository) *ResultWriterAdapter {
	return &ResultWriterAdapter{repo: repo}
}

var _ ResultWriter = (*ResultWriterAdapter)(nil)

// UpsertWithSupersedeTx delegates directly to CostResultRepository's existing tx-aware method.
func (a *ResultWriterAdapter) UpsertWithSupersedeTx(ctx context.Context, tx *sql.Tx, r *costcalcdom.Result) (int64, int, float64, int64, error) {
	return a.repo.UpsertWithSupersedeTx(ctx, tx, r)
}
