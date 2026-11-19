package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
)

// lfSideEffects records what the head cascade touched.
type lfSideEffects struct {
	CascadedSpinIDs []string `json:"cascaded_spin_ids"`
	CostProductID   int64    `json:"cost_product_id,omitempty"`
	// SpinChanges / CostProductChange hold before/after of every column the
	// cascade actually changed (only changed columns), so revert can restore them.
	SpinChanges       map[string]lfImage `json:"spin_changes,omitempty"`
	CostProductChange *lfImage           `json:"cost_product_change,omitempty"`
}

// lfImage is a before/after pair of column values.
type lfImage struct {
	Before map[string]string `json:"before"`
	After  map[string]string `json:"after"`
}

var lfSpinCascadeCols = []string{"mbs_mb_costing", "mbs_mgt_name", "mbs_denier", "mbs_filament", "mbs_dozing",
	"mbs_final_product", "mbs_vs_number", "mbs_lusture_code", "mbs_cross_section", "mbs_shade_code",
	"mbs_shade_name", "mbs_cc", "mbs_orion_item_code"}

// lfSnapSpins reads the cascade-touched columns of every live spin of the head, keyed by spin id.
func lfSnapSpins(ctx context.Context, tx *sql.Tx, headID string) (map[string]map[string]string, error) {
	var qb strings.Builder
	qb.WriteString("SELECT CAST(mbs_id AS text)")
	for _, c := range lfSpinCascadeCols {
		qb.WriteString(", CAST(")
		qb.WriteString(c)
		qb.WriteString(" AS text)")
	}
	qb.WriteString(" FROM mst_mb_spin WHERE mbs_mbh_id = CAST($1 AS uuid) AND deleted_at IS NULL")
	q := qb.String()
	rs, err := tx.QueryContext(ctx, q, headID) //nolint:gosec // static column list
	if err != nil {
		return nil, fmt.Errorf("snapshot spins: %w", err)
	}
	defer closeRows(rs)
	out := map[string]map[string]string{}
	for rs.Next() {
		vals := make([]sql.NullString, len(lfSpinCascadeCols)+1)
		ptrs := make([]any, len(vals))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan spin snapshot: %w", err)
		}
		m := map[string]string{}
		for i, c := range lfSpinCascadeCols {
			m[c] = vals[i+1].String
		}
		out[vals[0].String] = m
	}
	return out, rs.Err()
}

func lfSnapProduct(ctx context.Context, tx *sql.Tx, pid int64) (map[string]string, error) {
	var n, c, sn sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT cpm_product_name, cpm_shade_code, cpm_shade_name FROM cost_product_master WHERE cpm_product_sys_id = $1`, pid).Scan(&n, &c, &sn)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("snapshot cost product: %w", err)
	}
	return map[string]string{"cpm_product_name": n.String, "cpm_shade_code": c.String, "cpm_shade_name": sn.String}, nil
}

// lfDiff returns the changed columns as an image, or nil when nothing changed.
func lfDiff(before, after map[string]string) *lfImage {
	img := &lfImage{Before: map[string]string{}, After: map[string]string{}}
	for k, b := range before {
		if a := after[k]; a != b {
			img.Before[k], img.After[k] = b, a
		}
	}
	if len(img.Before) == 0 {
		return nil
	}
	return img
}

// lfUniqueSQL maps a head column to its in-tx uniqueness probe (TRIM compared, non-empty values only).
var lfUniqueCols = map[string]bool{
	"mbh_mb_costing": true, "mbh_orion_item_code": true, "mbh_vs_number": true, "mbh_dev_code": true,
}

// lfHandleD writes one class-D row. The lock is bypassed on purpose (like
// mb_active_sync_writer); entry_status/is_locked/version are never touched.
func lfHandleD(ctx context.Context, tx *sql.Tx, g *lfGroup, r mblegacyfill.PlanRow, cur string) (lfOutcome, error) {
	spec := r.Row.Spec
	if reject, err := lfCheckUnique(ctx, tx, r); err != nil {
		return lfOutcome{}, err
	} else if reject != "" {
		return lfOutcome{rejectReason: reject}, nil
	}
	if err := lfWriteColumn(ctx, tx, g, r.Row.Entity, r.Row.ID, spec, spec.Column, r.Row.LegacyValue); err != nil {
		return lfOutcome{}, err
	}
	out := lfOutcome{before: map[string]string{spec.Column: mblegacyfill.Normalize(cur)}, after: map[string]string{}}
	if r.Row.Entity == mblegacyfill.EntitySpin && spec.Field == "run_ldr" {
		if err := lfSetSpinActual(ctx, tx, g, r.Row.ID, &out); err != nil {
			return lfOutcome{}, err
		}
	}
	after, _, err := lfReadColumn(ctx, tx, r.Row.Entity, r.Row.ID, spec.Column, "")
	if err != nil {
		return lfOutcome{}, err
	}
	out.after[spec.Column] = after
	return out, nil
}

// lfCheckUnique returns a reject reason when a unique head column would clash.
func lfCheckUnique(ctx context.Context, tx *sql.Tx, r mblegacyfill.PlanRow) (string, error) {
	spec := r.Row.Spec
	if r.Row.Entity == mblegacyfill.EntityHead && spec.Column == "mbh_code" {
		return lfCheckCodeFgShade(ctx, tx, r.Row.ID, mblegacyfill.Normalize(r.Row.LegacyValue))
	}
	if r.Row.Entity != mblegacyfill.EntityHead || !lfUniqueCols[spec.Column] {
		return "", nil
	}
	v := mblegacyfill.Normalize(r.Row.LegacyValue)
	if v == "" {
		return "", nil
	}
	clash, err := lfHeadClash(ctx, tx, r.Row.ID, spec.Column, v)
	if err != nil || clash == "" {
		return "", err
	}
	return fmt.Sprintf("%s%s=%q already held by live head %s", mblegacyfill.ReasonUniquenessPrefix, spec.Column, v, clash), nil
}

// lfCheckCodeFgShade mirrors the UI rule checkHeadShadeConsistent: a head linked to an FG
// POY must keep its code (mbh_code) equal to its shade code, otherwise the root spin would
// carry another shade than the one the user sees.
func lfCheckCodeFgShade(ctx context.Context, tx *sql.Tx, headID, code string) (string, error) {
	if code == "" {
		return "", nil
	}
	var hasFg bool
	var shade string
	err := tx.QueryRowContext(ctx, `SELECT mbh_fg_product_id IS NOT NULL, COALESCE(TRIM(mbh_shade_code), '')
		FROM mst_mb_head WHERE mbh_id = CAST($1 AS uuid)`, headID).Scan(&hasFg, &shade)
	if err != nil {
		return "", fmt.Errorf("read head fg/shade: %w", err)
	}
	if hasFg && shade != "" && !strings.EqualFold(shade, code) {
		return fmt.Sprintf("%sfg shade: head code %q differs from shade code %q of an FG-linked head", mblegacyfill.ReasonUniquenessPrefix, code, shade), nil
	}
	return "", nil
}

// lfHeadClash returns the id of another live head holding the value, or "".
func lfHeadClash(ctx context.Context, tx *sql.Tx, headID, col, val string) (string, error) {
	//nolint:gosec // column comes from the static registry allow-list, value is bound
	q := "SELECT CAST(mbh_id AS text) FROM mst_mb_head WHERE TRIM(" + col + ") = $1 AND deleted_at IS NULL AND mbh_id <> CAST($2 AS uuid) LIMIT 1"
	var id string
	err := tx.QueryRowContext(ctx, q, val, headID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("uniqueness probe %s: %w", col, err)
	}
	return id, nil
}

// lfSetSpinActual marks run_ldr as the actual LDR (before/after images included).
func lfSetSpinActual(ctx context.Context, tx *sql.Tx, g *lfGroup, id string, out *lfOutcome) error {
	var t string
	var a bool
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(mbs_ldr_type, ''), COALESCE(mbs_ldr_is_actual, FALSE) FROM mst_mb_spin WHERE mbs_id = CAST($1 AS uuid)`, id).Scan(&t, &a)
	if err != nil {
		return fmt.Errorf("read ldr type: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE mst_mb_spin SET mbs_ldr_type = 'ACTUAL', mbs_ldr_is_actual = TRUE, updated_at = NOW(), updated_by = $2
		WHERE mbs_id = CAST($1 AS uuid)`, id, g.actor); err != nil {
		return fmt.Errorf("set ldr actual: %w", err)
	}
	out.before["mbs_ldr_type"], out.before["mbs_ldr_is_actual"] = t, fmt.Sprint(a)
	out.after["mbs_ldr_type"], out.after["mbs_ldr_is_actual"] = "ACTUAL", boolTrueString
	return nil
}

// applyRows runs head rows, then (if a head D column changed) the cascade, then spin rows.
func (w *MBLegacyFillWriter) applyRows(ctx context.Context, tx *sql.Tx, g *lfGroup, out []mblegacyfill.RowResult) error {
	firstSpin := len(g.rows)
	for i, r := range g.rows {
		if r.Row.Entity == mblegacyfill.EntitySpin {
			firstSpin = i
			break
		}
	}
	for i := 0; i < firstSpin; i++ {
		out[i] = w.applyRow(ctx, tx, g, g.rows[i])
	}
	if g.dApplied {
		if err := w.lockSpinGuards(ctx, tx, g, firstSpin); err != nil {
			return err
		}
		fx, err := w.cascadeHead(ctx, tx, g)
		if err != nil {
			return err
		}
		lfAttachEffects(g, out[:firstSpin], fx)
	}
	for i := firstSpin; i < len(g.rows); i++ {
		out[i] = w.applyRow(ctx, tx, g, g.rows[i])
	}
	return nil
}

func lfAttachEffects(g *lfGroup, out []mblegacyfill.RowResult, fx lfSideEffects) {
	b, err := json.Marshal(fx)
	if err != nil {
		return
	}
	for i := range out {
		if out[i].Result == mblegacyfill.ResultApplied && g.rows[i].Class == mblegacyfill.ClassD {
			out[i].SideEffects = string(b)
		}
	}
}

// lockSpinGuards captures the pre-cascade value of every spin row column (FOR UPDATE).
func (w *MBLegacyFillWriter) lockSpinGuards(ctx context.Context, tx *sql.Tx, g *lfGroup, firstSpin int) error {
	g.pre = map[string]string{}
	for _, r := range g.rows[firstSpin:] {
		v, found, err := lfLockCurrent(ctx, tx, r.Row)
		if err != nil {
			return err
		}
		if found {
			g.pre[lfPreKey(r.Row)] = v
		}
	}
	return nil
}

// cascadeHead reloads the head inside the tx and runs the UI cascade (spins, cost product).
func (w *MBLegacyFillWriter) cascadeHead(ctx context.Context, tx *sql.Tx, g *lfGroup) (lfSideEffects, error) {
	var fx lfSideEffects
	hid, err := uuid.Parse(g.rows[0].Row.ID)
	if err != nil {
		return fx, fmt.Errorf("head id: %w", err)
	}
	repo := &MBHeadRepository{db: w.db}
	entity, err := repo.scanOne(tx.QueryRowContext(ctx, repo.selectCols()+` WHERE mbh_id = $1 AND deleted_at IS NULL`, hid))
	if err != nil {
		return fx, fmt.Errorf("reload head: %w", err)
	}
	spinsBefore, err := lfSnapSpins(ctx, tx, g.rows[0].Row.ID)
	if err != nil {
		return fx, err
	}
	ids, err := cascadeHeadToSpinsTx(ctx, tx, entity, g.actor)
	if err != nil {
		return fx, fmt.Errorf("cascade spins: %w", err)
	}
	fx.CascadedSpinIDs = make([]string, 0, len(ids))
	for _, id := range ids {
		fx.CascadedSpinIDs = append(fx.CascadedSpinIDs, id.String())
	}
	spinsAfter, err := lfSnapSpins(ctx, tx, g.rows[0].Row.ID)
	if err != nil {
		return fx, err
	}
	fx.SpinChanges = map[string]lfImage{}
	for id, b := range spinsBefore {
		if d := lfDiff(b, spinsAfter[id]); d != nil {
			fx.SpinChanges[id] = *d
		}
	}
	if pid := entity.CostProductID(); pid != 0 {
		pb, err := lfSnapProduct(ctx, tx, pid)
		if err != nil {
			return fx, err
		}
		if err := cascadeHeadToCostProductTx(ctx, tx, entity, pid, g.actor); err != nil {
			return fx, fmt.Errorf("cascade cost product: %w", err)
		}
		pa, err := lfSnapProduct(ctx, tx, pid)
		if err != nil {
			return fx, err
		}
		fx.CostProductID = pid
		fx.CostProductChange = lfDiff(pb, pa)
	}
	return fx, nil
}
