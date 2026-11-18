package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
)

// MBActiveSyncSnapshotReader implements mbactivesync.SnapshotReader. Every read
// runs inside ONE read-only REPEATABLE READ transaction so the snapshot is
// internally consistent and nothing can be written.
type MBActiveSyncSnapshotReader struct{ db *DB }

// NewMBActiveSyncSnapshotReader creates the reader.
func NewMBActiveSyncSnapshotReader(db *DB) *MBActiveSyncSnapshotReader {
	return &MBActiveSyncSnapshotReader{db: db}
}

var _ mbactivesync.SnapshotReader = (*MBActiveSyncSnapshotReader)(nil)

const mbasHeadsQuery = `
	SELECT mbh_id, COALESCE(mbh_oracle_sys_id, ''), COALESCE(mbh_mb_costing, ''), COALESCE(mbh_orion_item_code, ''),
	       mbh_is_active, deleted_at IS NOT NULL, COALESCE(mbh_check_status, ''), COALESCE(mbh_entry_status, ''),
	       mbh_superseded_by_head_id, mbh_cost_product_id, mbh_fg_product_id,
	       COALESCE(mbh_shade_code, ''), COALESCE(mbh_shade_name, ''), COALESCE(mbh_code, ''), COALESCE(mbh_is_boughtout, FALSE),
	       mbh_denier, mbh_filament, created_at
	FROM mst_mb_head
	ORDER BY created_at, mbh_id`

const mbasSpinsQuery = `
	SELECT mbs_id, mbs_mbh_id, mbs_parent_spin_id, mbs_is_active, mbs_superseded_by_spin_id,
	       mbs_denier, mbs_filament,
	       COALESCE(NULLIF(mbs_cross_section, ''), NULLIF(mbs_lesture, ''), ''),
	       COALESCE(mbs_shade_code, ''), COALESCE(mbs_shade_name, ''), COALESCE(mbs_cc, ''),
	       mbs_run_ldr_pct, COALESCE(mbs_ldr_type, ''), COALESCE(mbs_ldr_is_actual, FALSE), COALESCE(mbs_ldr_is_fixed, FALSE),
	       mbs_fg_product_id, COALESCE(mbs_fg_link_source, ''),
	       COALESCE(mbs_orion_item_code, ''), COALESCE(mbs_mgt_name, ''), COALESCE(mbs_mb_costing, ''), created_at
	FROM mst_mb_spin
	WHERE deleted_at IS NULL
	ORDER BY created_at, mbs_id`

const mbasPOYQuery = `
	SELECT m.cpm_product_sys_id, COALESCE(m.cpm_product_code, ''), COALESCE(m.cpm_erp_item_code, ''),
	       COALESCE(m.cpm_shade_code, ''), COALESCE(m.cpm_shade_name, ''), m.cpm_is_active
	FROM cost_product_master m
	JOIN cost_product_type t ON t.cpt_type_id = m.cpm_product_type_id
	WHERE t.cpt_type_code = 'POY'
	ORDER BY m.cpm_product_sys_id`

// mbasPointerRefsQuery: distinct products per spin by permanent pointer (all filled_by).
const mbasPointerRefsQuery = `
	SELECT c.cpp_value_mb_spin_id, COUNT(DISTINCT c.cpp_product_sys_id)
	FROM cost_product_parameter c
	JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.param_code = 'MB_SP_CODE' AND p.deleted_at IS NULL
	WHERE c.cpp_value_mb_spin_id IS NOT NULL
	GROUP BY c.cpp_value_mb_spin_id`

// mbasTextRefsQuery: distinct products per pointer-less text value (UPPER(TRIM)).
const mbasTextRefsQuery = `
	SELECT UPPER(TRIM(c.cpp_value_text)), COUNT(DISTINCT c.cpp_product_sys_id)
	FROM cost_product_parameter c
	JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.param_code = 'MB_SP_CODE' AND p.deleted_at IS NULL
	WHERE c.cpp_value_mb_spin_id IS NULL AND TRIM(COALESCE(c.cpp_value_text, '')) <> ''
	GROUP BY UPPER(TRIM(c.cpp_value_text))`

const mbasDozingParamQuery = `
	SELECT COALESCE(lookup_source_column, '') FROM mst_parameter
	WHERE param_code = 'MB_SP_DOZING' AND deleted_at IS NULL LIMIT 1`

// Load reads the whole snapshot in one READ ONLY REPEATABLE READ transaction.
func (r *MBActiveSyncSnapshotReader) Load(ctx context.Context) (*mbactivesync.Snapshot, error) {
	tx, err := r.db.BeginTx(ctx, snapshotTxOptions())
	if err != nil {
		return nil, fmt.Errorf("begin read-only snapshot tx: %w", err)
	}
	defer rollbackSnapshotTx(tx) // read-only: nothing to commit

	return loadMBActiveSyncSnapshot(ctx, tx)
}

func loadMBActiveSyncSnapshot(ctx context.Context, q supersedeQuerier) (*mbactivesync.Snapshot, error) {
	snap := &mbactivesync.Snapshot{}
	var err error
	if snap.Heads, err = mbasQueryHeads(ctx, q); err != nil {
		return nil, err
	}
	if snap.Spins, err = mbasQuerySpins(ctx, q); err != nil {
		return nil, err
	}
	if snap.POYs, err = mbasQueryPOYs(ctx, q); err != nil {
		return nil, err
	}
	if err = mbasAttachRefs(ctx, q, snap.Spins); err != nil {
		return nil, err
	}
	err = q.QueryRowContext(ctx, mbasDozingParamQuery).Scan(&snap.DozingSourceColumn)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("snapshot MB_SP_DOZING param: %w", err)
	}
	return snap, nil
}

func mbasQueryHeads(ctx context.Context, q supersedeQuerier) ([]mbactivesync.HeadSnap, error) {
	rows, err := q.QueryContext(ctx, mbasHeadsQuery)
	if err != nil {
		return nil, fmt.Errorf("snapshot heads: %w", err)
	}
	defer closeRows(rows)
	var out []mbactivesync.HeadSnap
	for rows.Next() {
		var (
			h    mbactivesync.HeadSnap
			sup  uuid.NullUUID
			cost sql.NullInt64
			fg   sql.NullInt64
			den  sql.NullFloat64
			fil  sql.NullInt64
		)
		if err := rows.Scan(&h.ID, &h.SysID, &h.MBCosting, &h.OrionItemCode, &h.IsActive, &h.Deleted,
			&h.CheckStatus, &h.EntryStatus, &sup, &cost, &fg, &h.ShadeCode, &h.ShadeName, &h.Code, &h.IsBoughtout,
			&den, &fil, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan snapshot head: %w", err)
		}
		if sup.Valid {
			v := sup.UUID
			h.SupersededBy = &v
		}
		h.CostProductID = nullInt64Ptr(cost)
		h.FGProductID = nullInt64Ptr(fg)
		if den.Valid {
			v := den.Float64
			h.Denier = &v
		}
		if fil.Valid {
			v := int(fil.Int64)
			h.Filament = &v
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func mbasQuerySpins(ctx context.Context, q supersedeQuerier) ([]mbactivesync.SpinSnap, error) {
	rows, err := q.QueryContext(ctx, mbasSpinsQuery)
	if err != nil {
		return nil, fmt.Errorf("snapshot spins: %w", err)
	}
	defer closeRows(rows)
	var out []mbactivesync.SpinSnap
	for rows.Next() {
		var (
			s      mbactivesync.SpinSnap
			parent uuid.NullUUID
			sup    uuid.NullUUID
			den    sql.NullFloat64
			fil    sql.NullInt64
			ldr    sql.NullFloat64
			fg     sql.NullInt64
		)
		if err := rows.Scan(&s.ID, &s.HeadID, &parent, &s.IsActive, &sup, &den, &fil, &s.CrossSection,
			&s.ShadeCode, &s.ShadeName, &s.CC, &ldr, &s.LDRType, &s.LDRIsActual, &s.LDRIsFixed,
			&fg, &s.FGLinkSource, &s.OrionItemCode, &s.MGTName, &s.MBCosting, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan snapshot spin: %w", err)
		}
		if parent.Valid {
			v := parent.UUID
			s.ParentID = &v
		}
		if sup.Valid {
			v := sup.UUID
			s.SupersededBy = &v
		}
		if den.Valid {
			v := den.Float64
			s.Denier = &v
		}
		if fil.Valid {
			v := int(fil.Int64)
			s.Filament = &v
		}
		if ldr.Valid {
			v := ldr.Float64
			s.RunLDRPct = &v
		}
		s.FGProductID = nullInt64Ptr(fg)
		out = append(out, s)
	}
	return out, rows.Err()
}

func mbasQueryPOYs(ctx context.Context, q supersedeQuerier) ([]mbactivesync.POYSnap, error) {
	rows, err := q.QueryContext(ctx, mbasPOYQuery)
	if err != nil {
		return nil, fmt.Errorf("snapshot POY products: %w", err)
	}
	defer closeRows(rows)
	var out []mbactivesync.POYSnap
	for rows.Next() {
		var p mbactivesync.POYSnap
		if err := rows.Scan(&p.SysID, &p.ProductCode, &p.ERPItemCode, &p.ShadeCode, &p.ShadeName, &p.IsActive); err != nil {
			return nil, fmt.Errorf("scan snapshot POY: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// mbasAttachRefs fills the MB_SP_CODE product reference counts on every spin.
func mbasAttachRefs(ctx context.Context, q supersedeQuerier, spins []mbactivesync.SpinSnap) error {
	ptr := make(map[uuid.UUID]int)
	rows, err := q.QueryContext(ctx, mbasPointerRefsQuery)
	if err != nil {
		return fmt.Errorf("snapshot pointer refs: %w", err)
	}
	for rows.Next() {
		var id uuid.UUID
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			closeRows(rows)
			return fmt.Errorf("scan pointer ref: %w", err)
		}
		ptr[id] = n
	}
	err = rows.Err()
	closeRows(rows)
	if err != nil {
		return err
	}

	txt := make(map[string]int)
	rows, err = q.QueryContext(ctx, mbasTextRefsQuery)
	if err != nil {
		return fmt.Errorf("snapshot text refs: %w", err)
	}
	for rows.Next() {
		var key string
		var n int
		if err := rows.Scan(&key, &n); err != nil {
			closeRows(rows)
			return fmt.Errorf("scan text ref: %w", err)
		}
		txt[strings.ToUpper(key)] = n
	}
	err = rows.Err()
	closeRows(rows)
	if err != nil {
		return err
	}

	keyCount := make(map[string]int, len(spins))
	for i := range spins {
		keyCount[spins[i].SpinTextKey()]++
	}
	for i := range spins {
		s := &spins[i]
		s.PointerProducts = ptr[s.ID]
		key := s.SpinTextKey()
		// Mirrors supersede semantics: TextProducts is always the number of
		// pointer-less rows on this key; TextShared means "ambiguous text refs"
		// (key shared by >1 spin AND at least one text row exists).
		s.TextProducts = txt[key]
		s.TextShared = keyCount[key] > 1 && txt[key] > 0
	}
	return nil
}

func rollbackSnapshotTx(tx *sql.Tx) {
	if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
		log.Warn().Err(rbErr).Msg("rollback snapshot tx")
	}
}

func nullInt64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// ProbeMBActiveSyncSnapshotTxReadOnly opens a transaction with the exact options
// Load uses and verifies that a write inside it fails. It writes nothing (the
// attempt is rejected by PostgreSQL). Exported for the integration test.
func ProbeMBActiveSyncSnapshotTxReadOnly(ctx context.Context, db *DB) error {
	tx, err := db.BeginTx(ctx, snapshotTxOptions())
	if err != nil {
		return err
	}
	defer rollbackSnapshotTx(tx)
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE mbas_probe (x int)`); err == nil {
		return errors.New("write succeeded inside snapshot tx: not read-only")
	}
	return nil
}

func snapshotTxOptions() *sql.TxOptions {
	return &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
}
