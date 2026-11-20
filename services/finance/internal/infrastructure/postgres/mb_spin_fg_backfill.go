package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// MBSpinFgBackfillRepository serves the one-shot FG backfill of legacy MB spins
// (cmd/backfill-mb-spin-fg, design §6 + Revisi 3). ListBackfillCandidates is
// read-only; ApplyBackfillRow writes one reviewed row per transaction.
type MBSpinFgBackfillRepository struct {
	db *DB
}

// NewMBSpinFgBackfillRepository creates the backfill repository.
func NewMBSpinFgBackfillRepository(db *DB) *MBSpinFgBackfillRepository {
	return &MBSpinFgBackfillRepository{db: db}
}

// backfillCandidatesSQL mirrors docs/mb-spin-fg/sql/postgres/P-13-backfill-match-by-spec-prefix.sql.
//
// Normalization (poy.shade/nspec, s.shade/nfp) and the spec-match predicate are
// copied verbatim from P-13. The only difference is the join: P-13 joins
// `poy.shade = s.shade OR (s.nfp <> ” AND poy.nspec LIKE s.nfp || '%')`, here we
// join on `poy.shade = s.shade` only. This yields identical buckets because:
//   - the buckets use only n_spec_shade and n_shade_only (plus shade/nfp);
//   - both counts FILTER on `poy.shade = s.shade`, so every POY row they count
//     satisfies the first disjunct and is still joined;
//   - rows joined only through the second disjunct (spec prefix, different
//     shade) feed only P-13's n_spec, which no bucket reads;
//   - COUNT(poy.id) ignores the NULL row of an unmatched LEFT JOIN in both.
//
// The bucket CASE itself is evaluated in Go (mbspin.ClassifyBackfill) in the same
// order. LIKE safety: nfp contains only [A-Z0-9/] (every other char was replaced by
// '/'), so it never carries the LIKE wildcards '%' or '_'.
//
// The single POY id per count is MIN(id) FILTER(...), used only when that count = 1.
const backfillCandidatesSQL = `
WITH poy AS (
  SELECT p.cpm_product_sys_id, UPPER(TRIM(p.cpm_shade_code)) AS shade,
         REGEXP_REPLACE(REGEXP_REPLACE(UPPER(TRIM(p.cpm_product_name)), '[^A-Z0-9]+', '/', 'g'), '^POY/', '') AS nspec
    FROM cost_product_master p
    JOIN cost_product_type t ON t.cpt_type_id = p.cpm_product_type_id
   WHERE t.cpt_type_code = 'POY' AND p.cpm_is_active
),
s AS (
  SELECT mbs_id, mbs_mbh_id, mbs_parent_spin_id, mbs_orion_item_code, mbs_mgt_name,
         mbs_shade_code, mbs_final_product, mbs_fg_product_id, mbs_superseded_by_spin_id,
         UPPER(TRIM(mbs_shade_code)) AS shade,
         TRIM(BOTH '/' FROM REGEXP_REPLACE(REGEXP_REPLACE(UPPER(TRIM(mbs_final_product)), '[^A-Z0-9]+', '/', 'g'), '^/?POY/', '')) AS nfp
    FROM mst_mb_spin
   WHERE deleted_at IS NULL AND mbs_is_active
),
m AS (
  SELECT s.mbs_id,
         COUNT(poy.cpm_product_sys_id) FILTER (WHERE s.nfp <> '' AND (poy.nspec = s.nfp OR poy.nspec LIKE s.nfp || '/%') AND poy.shade = s.shade) AS n_spec_shade,
         COUNT(poy.cpm_product_sys_id) FILTER (WHERE poy.shade = s.shade) AS n_shade_only,
         MIN(poy.cpm_product_sys_id) FILTER (WHERE s.nfp <> '' AND (poy.nspec = s.nfp OR poy.nspec LIKE s.nfp || '/%') AND poy.shade = s.shade) AS spec_shade_id,
         MIN(poy.cpm_product_sys_id) FILTER (WHERE poy.shade = s.shade) AS shade_only_id
    FROM s LEFT JOIN poy ON poy.shade = s.shade
   GROUP BY s.mbs_id
)
SELECT CAST(s.mbs_id AS text), CAST(s.mbs_mbh_id AS text), s.mbs_parent_spin_id IS NULL,
       COALESCE(s.mbs_orion_item_code, ''), COALESCE(s.mbs_mgt_name, ''),
       COALESCE(s.mbs_shade_code, ''), COALESCE(s.mbs_final_product, ''),
       COALESCE(s.shade, ''), COALESCE(s.nfp, ''),
       m.n_spec_shade, m.n_shade_only,
       COALESCE(s.mbs_fg_product_id, 0), s.mbs_superseded_by_spin_id IS NOT NULL,
       p1.cpm_product_sys_id, COALESCE(p1.cpm_product_code, ''), COALESCE(p1.cpm_product_name, ''), COALESCE(p1.cpm_erp_item_code, ''),
       p2.cpm_product_sys_id, COALESCE(p2.cpm_product_code, ''), COALESCE(p2.cpm_product_name, ''), COALESCE(p2.cpm_erp_item_code, '')
  FROM s
  JOIN m ON m.mbs_id = s.mbs_id
  LEFT JOIN cost_product_master p1 ON m.n_spec_shade = 1 AND p1.cpm_product_sys_id = m.spec_shade_id
  LEFT JOIN cost_product_master p2 ON m.n_shade_only = 1 AND p2.cpm_product_sys_id = m.shade_only_id
 ORDER BY s.mbs_id`

// ListBackfillCandidates returns every live (active, not deleted) spin with the
// P-13 matcher counts. It runs inside a READ ONLY transaction and never writes.
func (r *MBSpinFgBackfillRepository) ListBackfillCandidates(ctx context.Context) (out []mbspin.BackfillCandidate, err error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin read-only tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback read-only tx: %w", rbErr)
		}
	}()

	rows, err := tx.QueryContext(ctx, backfillCandidatesSQL)
	if err != nil {
		return nil, fmt.Errorf("query backfill candidates: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil && err == nil {
			err = fmt.Errorf("close backfill rows: %w", cErr)
		}
	}()
	for rows.Next() {
		c, scanErr := scanBackfillCandidate(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backfill candidates: %w", err)
	}
	return out, nil
}

func scanBackfillCandidate(rows *sql.Rows) (mbspin.BackfillCandidate, error) {
	var (
		c      mbspin.BackfillCandidate
		p1ID   sql.NullInt64
		p2ID   sql.NullInt64
		p1, p2 mbspin.BackfillPoy
	)
	if err := rows.Scan(&c.SpinID, &c.HeadID, &c.IsRoot, &c.OrionItemCode, &c.MgtName,
		&c.ShadeCode, &c.FinalProduct, &c.NormShade, &c.NormSpec,
		&c.NSpecShade, &c.NShadeOnly, &c.FgProductID, &c.Superseded,
		&p1ID, &p1.ProductCode, &p1.ProductName, &p1.ErpItemCode,
		&p2ID, &p2.ProductCode, &p2.ProductName, &p2.ErpItemCode); err != nil {
		return c, fmt.Errorf("scan backfill candidate: %w", err)
	}
	if p1ID.Valid {
		p1.ProductSysID = p1ID.Int64
		c.SpecShadePoy = &p1
	}
	if p2ID.Valid {
		p2.ProductSysID = p2ID.Int64
		c.ShadeOnlyPoy = &p2
	}
	return c, nil
}

// backfillSpinState is the locked spin row read at apply time.
type backfillSpinState struct {
	headID    string
	isRoot    bool
	live      bool
	fgID      int64
	normShade string
}

// backfillPoyState is the locked POY row read at apply time.
type backfillPoyState struct {
	isPoy     bool
	active    bool
	normShade string
	erpItem   sql.NullString
}

func skipped(reason, msg string) mbspin.BackfillApplyOutcome {
	return mbspin.BackfillApplyOutcome{Result: mbspin.BackfillResultSkipped, Reason: reason, Message: msg}
}

// ApplyBackfillRow links one spin to its FG POY in its own transaction, after
// re-checking every condition under row locks. It never overwrites an FG that is
// already set (spin or head), never recalculates and never touches
// cost_product_parameter. Skip outcomes roll back (nothing written); a non-nil
// error means a database failure (the transaction is rolled back too).
func (r *MBSpinFgBackfillRepository) ApplyBackfillRow(ctx context.Context, in mbspin.BackfillApplyRequest) (out mbspin.BackfillApplyOutcome, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return out, fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback: %w", rbErr)
		}
	}()

	out, err = applyBackfillInTx(ctx, tx, in)
	if err != nil || out.Result != mbspin.BackfillResultApplied {
		return out, err
	}
	if err := tx.Commit(); err != nil {
		return mbspin.BackfillApplyOutcome{}, fmt.Errorf("commit: %w", err)
	}
	committed = true
	return out, nil
}

func applyBackfillInTx(ctx context.Context, tx *sql.Tx, in mbspin.BackfillApplyRequest) (mbspin.BackfillApplyOutcome, error) {
	spin, found, err := lockBackfillSpin(ctx, tx, in.SpinID)
	if err != nil {
		return mbspin.BackfillApplyOutcome{}, err
	}
	if !found || !spin.live {
		return skipped(mbspin.BackfillReasonSpinNotLive, "spin missing, deleted, inactive or superseded"), nil
	}
	if spin.fgID == in.PoyProductSysID {
		return skipped(mbspin.BackfillReasonAlreadyLinked, "spin already linked to this POY"), nil
	}
	if spin.fgID != 0 {
		return skipped(mbspin.BackfillReasonFgAlreadySet, fmt.Sprintf("spin already has FG %d", spin.fgID)), nil
	}
	if spin.normShade == "" {
		return skipped(mbspin.BackfillReasonShadeMismatch, "spin has no shade"), nil
	}

	poy, found, err := lockBackfillPoy(ctx, tx, in.PoyProductSysID)
	if err != nil {
		return mbspin.BackfillApplyOutcome{}, err
	}
	if !found || !poy.isPoy || !poy.active {
		return skipped(mbspin.BackfillReasonPoyInvalid, "POY missing, inactive or not of type POY"), nil
	}
	if poy.normShade != spin.normShade {
		return skipped(mbspin.BackfillReasonShadeMismatch,
			fmt.Sprintf("POY shade %q != spin shade %q", poy.normShade, spin.normShade)), nil
	}

	other, err := findLiveSpinWithShadeFg(ctx, tx, in.SpinID, spin.normShade, in.PoyProductSysID)
	if err != nil {
		return mbspin.BackfillApplyOutcome{}, err
	}
	if other != "" {
		return skipped(mbspin.BackfillReasonConflictExistingLive, "live spin "+other+" already uses this (shade, FG)"), nil
	}

	return writeBackfill(ctx, tx, in, spin, poy)
}

func writeBackfill(ctx context.Context, tx *sql.Tx, in mbspin.BackfillApplyRequest, spin backfillSpinState, poy backfillPoyState) (mbspin.BackfillApplyOutcome, error) {
	var item sql.NullString
	if poy.erpItem.Valid && strings.TrimSpace(poy.erpItem.String) != "" {
		item = sql.NullString{String: strings.TrimSpace(poy.erpItem.String), Valid: true}
	}

	const updSpin = `
		UPDATE mst_mb_spin
		   SET mbs_fg_product_id = $2, mbs_fg_item_code = $3, mbs_fg_link_source = $4,
		       updated_at = NOW(), updated_by = $5
		 WHERE mbs_id = CAST($1 AS uuid) AND mbs_fg_product_id IS NULL`
	res, err := tx.ExecContext(ctx, updSpin, in.SpinID, in.PoyProductSysID, item, mbspin.FgLinkSourceBackfill, in.Actor)
	if err != nil {
		if isSpinShadeFgViolation(err) || isPGUniqueViolation(err) {
			return skipped(mbspin.BackfillReasonConflictUnique, "unique (shade, FG) violation"), nil
		}
		return mbspin.BackfillApplyOutcome{}, fmt.Errorf("update spin: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return mbspin.BackfillApplyOutcome{}, fmt.Errorf("spin rows affected: %w", err)
	}
	if n != 1 {
		return skipped(mbspin.BackfillReasonFgAlreadySet, "spin FG set concurrently"), nil
	}

	headUpdated, msg := false, ""
	if spin.isRoot {
		const updHead = `
			UPDATE mst_mb_head
			   SET mbh_fg_product_id = $2, mbh_fg_item_code = $3, updated_at = NOW(), updated_by = $4
			 WHERE mbh_id = CAST($1 AS uuid) AND mbh_fg_product_id IS NULL AND deleted_at IS NULL
			   AND NOT EXISTS (
			         SELECT 1 FROM mst_mb_spin o
			          WHERE o.mbs_mbh_id = CAST($1 AS uuid) AND o.mbs_parent_spin_id IS NULL
			            AND o.deleted_at IS NULL AND o.mbs_superseded_by_spin_id IS NULL
			            AND o.mbs_id <> CAST($5 AS uuid))`
		hres, err := tx.ExecContext(ctx, updHead, spin.headID, in.PoyProductSysID, item, in.Actor, in.SpinID)
		if err != nil {
			return mbspin.BackfillApplyOutcome{}, fmt.Errorf("update head: %w", err)
		}
		hn, err := hres.RowsAffected()
		if err != nil {
			return mbspin.BackfillApplyOutcome{}, fmt.Errorf("head rows affected: %w", err)
		}
		headUpdated = hn == 1
		if !headUpdated {
			msg = "head FG not set (head already has FG, or head has other root spins, active or not)"
		}
	}
	return mbspin.BackfillApplyOutcome{
		Result: mbspin.BackfillResultApplied, HeadUpdated: headUpdated, HeadID: spin.headID, Message: msg,
	}, nil
}

func lockBackfillSpin(ctx context.Context, tx *sql.Tx, spinID string) (backfillSpinState, bool, error) {
	const q = `
		SELECT CAST(mbs_mbh_id AS text), mbs_parent_spin_id IS NULL,
		       (deleted_at IS NULL AND mbs_is_active AND mbs_superseded_by_spin_id IS NULL),
		       COALESCE(mbs_fg_product_id, 0), COALESCE(UPPER(TRIM(mbs_shade_code)), '')
		  FROM mst_mb_spin
		 WHERE mbs_id = CAST($1 AS uuid)
		   FOR UPDATE`
	var s backfillSpinState
	err := tx.QueryRowContext(ctx, q, spinID).Scan(&s.headID, &s.isRoot, &s.live, &s.fgID, &s.normShade)
	if errors.Is(err, sql.ErrNoRows) {
		return s, false, nil
	}
	if err != nil {
		return s, false, fmt.Errorf("lock spin: %w", err)
	}
	return s, true, nil
}

func lockBackfillPoy(ctx context.Context, tx *sql.Tx, poyID int64) (backfillPoyState, bool, error) {
	const q = `
		SELECT t.cpt_type_code = 'POY', p.cpm_is_active,
		       COALESCE(UPPER(TRIM(p.cpm_shade_code)), ''), p.cpm_erp_item_code
		  FROM cost_product_master p
		  JOIN cost_product_type t ON t.cpt_type_id = p.cpm_product_type_id
		 WHERE p.cpm_product_sys_id = $1
		   FOR SHARE OF p`
	var p backfillPoyState
	err := tx.QueryRowContext(ctx, q, poyID).Scan(&p.isPoy, &p.active, &p.normShade, &p.erpItem)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return p, false, fmt.Errorf("lock POY: %w", err)
	}
	return p, true, nil
}

// findLiveSpinWithShadeFg returns another live spin already holding (shade, FG)
// under the uix_mbs_shade_fg_live predicate, or "" when none.
func findLiveSpinWithShadeFg(ctx context.Context, tx *sql.Tx, spinID, normShade string, poyID int64) (string, error) {
	const q = `
		SELECT CAST(mbs_id AS text)
		  FROM mst_mb_spin
		 WHERE UPPER(TRIM(mbs_shade_code)) = $2 AND mbs_fg_product_id = $3
		   AND deleted_at IS NULL AND mbs_is_active AND mbs_superseded_by_spin_id IS NULL
		   AND mbs_id <> CAST($1 AS uuid)
		 LIMIT 1`
	var other string
	err := tx.QueryRowContext(ctx, q, spinID, normShade, poyID).Scan(&other)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("check live (shade, FG): %w", err)
	}
	return other, nil
}
