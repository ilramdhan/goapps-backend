// Package lot provides application-layer handlers for lot-master CRUD.
package lot

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	lotdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/lot"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
)

// OracleLotSource lists lots from the legacy Oracle master ASPAK.MMSMERGE. A nil
// implementation (Oracle unavailable) is tolerated by the usecase.
type OracleLotSource interface {
	ListLots(ctx context.Context) ([]oracle.LotRow, error)
}

// SyncResult summarizes a lot-sync run.
type SyncResult struct {
	Inserted int
	Updated  int
	Skipped  int
	// OracleUsed reports whether the source contributed (false when Oracle was
	// unreachable and the run degraded to a no-op).
	OracleUsed bool
}

// SyncUsecase imports the legacy Oracle lot master into ppc.lot_master,
// preserving PPC-local corrections.
//
// This mirrors machinesync (design §4.2, anti-drift): a lot's yarn
// specification is owned by the legacy system and must not be hand-authored in
// PPC, or the two drift and the packing ETL joins on a lot whose denier PPC
// disagrees with. PPC-minted lots (source PPC) are untouched by the import
// unless Oracle happens to know the same lot number.
type SyncUsecase struct {
	repo   lotdomain.Repository
	oracle OracleLotSource
	now    func() time.Time
}

// NewSyncUsecase builds the lot-sync usecase. oracleSrc may be nil, in which
// case Sync is a no-op.
func NewSyncUsecase(repo lotdomain.Repository, oracleSrc OracleLotSource) *SyncUsecase {
	return &SyncUsecase{repo: repo, oracle: oracleSrc, now: time.Now}
}

// Sync pulls MMSMERGE and merges every row in one batched write. It returns an
// error only for a repository failure while writing; an unreachable source
// degrades to an empty result, matching the machine sync's contract.
//
// Rows the projection rejects (no lot number, no item code, an over-long lot
// number) are counted as skipped rather than aborting the import: MMSMERGE is a
// sparse legacy master and one malformed row must not cost the other ~66k.
func (u *SyncUsecase) Sync(ctx context.Context) (SyncResult, error) {
	res := SyncResult{}
	if u.oracle == nil {
		return res, nil
	}
	rows, err := u.oracle.ListLots(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("lot sync: Oracle MMSMERGE read failed, nothing imported")
		return res, nil //nolint:nilerr // an unreachable source degrades, it does not fail the run
	}
	if len(rows) > 0 {
		res.OracleUsed = true
	}

	syncedAt := u.now().UTC()
	batch := make([]lotdomain.SourcedLot, 0, len(rows))
	// MMSMERGE is not guaranteed unique on MERGE_CODE (one duplicate observed on
	// the live master), and a single ON CONFLICT statement cannot touch the same
	// row twice. Last row wins, matching the previous row-at-a-time behavior.
	seen := make(map[string]int, len(rows))
	for _, row := range rows {
		src, ok := toSourcedLot(row, syncedAt)
		if !ok {
			res.Skipped++
			continue
		}
		if idx, dup := seen[src.LotNo]; dup {
			batch[idx] = src
			res.Skipped++
			continue
		}
		seen[src.LotNo] = len(batch)
		batch = append(batch, src)
	}

	written, err := u.repo.UpsertSourcedBatch(ctx, batch)
	if err != nil {
		return res, err
	}
	res.Inserted = written.Inserted
	res.Updated = written.Updated
	res.Skipped += written.Skipped
	return res, nil
}

// toSourcedLot projects an MMSMERGE row onto the domain's sourced-lot shape.
// Rows with no lot number or no item code are rejected: lm_lot_no is the primary
// key and lm_item_code is NOT NULL, so neither can be defaulted.
func toSourcedLot(row oracle.LotRow, syncedAt time.Time) (lotdomain.SourcedLot, bool) {
	code := strings.TrimSpace(row.Code)
	itemCode := strings.TrimSpace(row.ItemCode)
	if code == "" || itemCode == "" {
		return lotdomain.SourcedLot{}, false
	}
	if len(code) > maxSourcedLotNoLen {
		return lotdomain.SourcedLot{}, false
	}

	return lotdomain.SourcedLot{
		LotNo:     code,
		SourceKey: code,
		ItemCode:  itemCode,
		ShadeCode: row.ShadeCode,
		// MERGE_BOB seeds the full standard weight on insert only. It is the
		// only per-bobbin figure the legacy master carries, and its reading as
		// kilograms is unconfirmed — hence insert-only, never an overwrite.
		StdWeightFull: row.BobWeight,
		Spec: lotdomain.Spec{
			ProdType:         row.ProdType,
			YarnType:         row.YarnType,
			Denier:           row.Denier,
			Filament:         row.Filament,
			CrossSection:     row.CrossSection,
			QCGrade:          row.QCGrade,
			Description:      row.Description,
			ShadeColor:       row.ShadeColor,
			TareBoxWeight:    row.TareBoxWeight,
			TareBobbinWeight: row.TareBobbinWeight,
			BobbinsPerBox:    row.BobbinsPerBox,
			SourceBobWeight:  row.BobWeight,
			OrionItemCode:    row.OrionItemCode,
			MachineNo:        row.MachineNo,
			EfficiencyPct:    row.EfficiencyPct,
			SourceStatus:     row.Status,
			SourcePakStatus:  row.PakStatus,
		},
		SyncedAt: syncedAt,
	}, true
}

// maxSourcedLotNoLen matches lot_master.lm_lot_no VARCHAR(30). MMSMERGE declares
// MERGE_CODE as VARCHAR2(15), so this only guards against a source that has
// outgrown its own declaration.
const maxSourcedLotNoLen = 30
