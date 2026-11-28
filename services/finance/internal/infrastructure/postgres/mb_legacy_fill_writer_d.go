package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
// mbh_mb_costing is not unique since migration 000619 (MB name follows the Finance Excel).
var lfUniqueCols = map[string]bool{
	"mbh_orion_item_code": true, "mbh_vs_number": true, "mbh_dev_code": true,
}

// lfSpinUniqueCols are spin columns unique among live spins.
var lfSpinUniqueCols = map[string]bool{"mbs_orion_item_code": true}

// lfHandleD writes one class-D row. The lock is bypassed on purpose (like
// mb_active_sync_writer); entry_status/is_locked/version are never touched.
func lfHandleD(ctx context.Context, tx *sql.Tx, g *lfGroup, r mblegacyfill.PlanRow, cur string) (lfOutcome, error) {
	spec := r.Row.Spec
	if r.Row.Entity == mblegacyfill.EntityHead && spec.Column == "mbh_mgt_name" {
		// MB Name always equals MB Costing (mb-comparison-p2 item 3): never written independently.
		return lfOutcome{rejectReason: mblegacyfill.ReasonNameFollowsCosting}, nil
	}
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
	if r.Row.Entity == mblegacyfill.EntityHead && spec.Column == "mbh_mb_costing" {
		// MB Name follows MB Costing: write it too; images carry it so revert restores it.
		if err := lfFollowName(ctx, tx, g, r, spec, &out); err != nil {
			return lfOutcome{}, err
		}
	}
	return out, nil
}

// lfFollowName sets head mbh_mgt_name = the new costing and records before/after images.
func lfFollowName(ctx context.Context, tx *sql.Tx, g *lfGroup, r mblegacyfill.PlanRow, spec mblegacyfill.FieldSpec, out *lfOutcome) error {
	const nameCol = "mbh_mgt_name"
	prev, _, err := lfReadColumn(ctx, tx, r.Row.Entity, r.Row.ID, nameCol, "")
	if err != nil {
		return err
	}
	if err := lfWriteColumn(ctx, tx, g, r.Row.Entity, r.Row.ID, spec, nameCol, r.Row.LegacyValue); err != nil {
		return err
	}
	now, _, err := lfReadColumn(ctx, tx, r.Row.Entity, r.Row.ID, nameCol, "")
	if err != nil {
		return err
	}
	out.before[nameCol], out.after[nameCol] = mblegacyfill.Normalize(prev), now
	return nil
}

// lfCheckUnique returns a reject reason when a unique head column would clash.
func lfCheckUnique(ctx context.Context, tx *sql.Tx, r mblegacyfill.PlanRow) (string, error) {
	spec := r.Row.Spec
	if r.Row.Entity == mblegacyfill.EntityHead && spec.Column == "mbh_code" {
		return lfCheckCodeFgShade(ctx, tx, r.Row.ID, mblegacyfill.Normalize(r.Row.LegacyValue))
	}
	isSpin := r.Row.Entity == mblegacyfill.EntitySpin
	if (isSpin && !lfSpinUniqueCols[spec.Column]) || (!isSpin && !lfUniqueCols[spec.Column]) {
		return "", nil
	}
	v := mblegacyfill.Normalize(r.Row.LegacyValue)
	if v == "" {
		return "", nil
	}
	clash, err := lfClash(ctx, tx, r.Row.Entity, r.Row.ID, spec.Column, v)
	if err != nil || clash == "" {
		return "", err
	}
	return fmt.Sprintf("%s%s=%q already held by live %s %s", mblegacyfill.ReasonUniquenessPrefix, spec.Column, v, r.Row.Entity, clash), nil
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

// lfClash returns the id of another live head/spin holding the value, or "".
func lfClash(ctx context.Context, tx *sql.Tx, e mblegacyfill.Entity, rowID, col, val string) (string, error) {
	t := lfTables[e]
	//nolint:gosec // column comes from the static registry allow-list, value is bound
	q := "SELECT CAST(" + t.idCol + " AS text) FROM " + t.table + " WHERE TRIM(" + col +
		") = $1 AND deleted_at IS NULL AND " + t.idCol + " <> CAST($2 AS uuid) LIMIT 1"
	var id string
	err := tx.QueryRowContext(ctx, q, val, rowID).Scan(&id)
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

// lfHeadToSpin maps a head column to the spin column that follows it, and its kind.
// Static allow-list: column names in SQL come only from here. dev, code, orion, run_ldr,
// labels and shade are deliberately absent (shade/lusture are never cascaded by legacy-fill).
var lfHeadToSpin = map[string]struct {
	spin string
	kind mblegacyfill.Kind
}{
	"mbh_mb_costing":    {"mbs_mb_costing", mblegacyfill.KindText},
	"mbh_mgt_name":      {"mbs_mgt_name", mblegacyfill.KindText},
	"mbh_denier":        {"mbs_denier", mblegacyfill.KindNum},
	"mbh_filament":      {"mbs_filament", mblegacyfill.KindInt},
	"mbh_dozing":        {"mbs_dozing", mblegacyfill.KindNum},
	"mbh_final_product": {"mbs_final_product", mblegacyfill.KindText},
	"mbh_vs_number":     {"mbs_vs_number", mblegacyfill.KindText},
}

// lfChange is the before/after of one applied head column.
type lfChange struct{ before, after string }

// lfPropagateSpin moves one head column change to the spins that still carry the OLD
// head value (spins that deliberately differ are untouched).
func lfPropagateSpin(ctx context.Context, tx *sql.Tx, g *lfGroup, headID, headCol string, c lfChange) error {
	m, ok := lfHeadToSpin[headCol]
	if !ok {
		return nil
	}
	newV, oldV := mblegacyfill.Normalize(c.after), mblegacyfill.Normalize(c.before)
	var set, cmp string
	switch m.kind {
	case mblegacyfill.KindText:
		if newV == "" {
			return nil // never blank a spin text column from a head change
		}
		set = "NULLIF(TRIM(CAST($2 AS text)), '')"
		cmp = "NULLIF(TRIM(" + m.spin + "), '') IS NOT DISTINCT FROM NULLIF(TRIM(CAST($3 AS text)), '')"
	case mblegacyfill.KindInt:
		set = "CAST(CAST(NULLIF(CAST($2 AS text), '') AS numeric) AS integer)"
		cmp = "CAST(" + m.spin + " AS numeric) IS NOT DISTINCT FROM CAST(NULLIF(CAST($3 AS text), '') AS numeric)"
	default:
		set = "CAST(NULLIF(CAST($2 AS text), '') AS numeric)"
		cmp = "CAST(" + m.spin + " AS numeric) IS NOT DISTINCT FROM CAST(NULLIF(CAST($3 AS text), '') AS numeric)"
	}
	//nolint:gosec // column names come from the static lfHeadToSpin map, values are bound
	q := "UPDATE mst_mb_spin SET " + m.spin + " = " + set + ", updated_at = NOW(), updated_by = $4 " +
		"WHERE mbs_mbh_id = CAST($1 AS uuid) AND deleted_at IS NULL AND " + cmp
	if _, err := tx.ExecContext(ctx, q, headID, newV, oldV, g.actor); err != nil {
		return fmt.Errorf("propagate %s to spins: %w", headCol, err)
	}
	return nil
}

// lfRenameCostProduct renames the MB-recipe cost product only when it still equals the old costing.
func lfRenameCostProduct(ctx context.Context, tx *sql.Tx, g *lfGroup, headID string, c lfChange) error {
	var pid int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(mbh_cost_product_id, 0) FROM mst_mb_head WHERE mbh_id = CAST($1 AS uuid)`, headID).Scan(&pid); err != nil {
		return fmt.Errorf("read head cost product: %w", err)
	}
	newV := mblegacyfill.Normalize(c.after)
	if pid == 0 || newV == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cost_product_master SET cpm_product_name = $1, cpm_updated_at = NOW(), cpm_updated_by = $2
		WHERE cpm_product_sys_id = $3 AND cpm_source = $4 AND cpm_product_name IS NOT DISTINCT FROM $5`,
		newV, g.actor, pid, mbCostProductSource, mblegacyfill.Normalize(c.before)); err != nil {
		return fmt.Errorf("rename cost product: %w", err)
	}
	return nil
}

// lfFillOrion fills the (empty) root spin orion code after the head orion changed (guarded helper).
func (w *MBLegacyFillWriter) lfFillOrion(ctx context.Context, tx *sql.Tx, g *lfGroup, headID string) error {
	hid, err := uuid.Parse(headID)
	if err != nil {
		return fmt.Errorf("head id: %w", err)
	}
	repo := &MBHeadRepository{db: w.db}
	entity, err := repo.scanOne(tx.QueryRowContext(ctx, repo.selectCols()+` WHERE mbh_id = $1 AND deleted_at IS NULL`, hid))
	if err != nil {
		return fmt.Errorf("reload head: %w", err)
	}
	return fillRootSpinOrionTx(ctx, tx, entity, g.actor, nil)
}

// lfApplyTargeted runs the targeted cascade for the head columns applied in this tx.
func (w *MBLegacyFillWriter) lfApplyTargeted(ctx context.Context, tx *sql.Tx, g *lfGroup, headID string) error {
	cols := make([]string, 0, len(g.headChanges))
	for c := range g.headChanges {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	for _, c := range cols {
		if err := lfPropagateSpin(ctx, tx, g, headID, c, g.headChanges[c]); err != nil {
			return err
		}
	}
	if c, ok := g.headChanges["mbh_mb_costing"]; ok {
		if err := lfRenameCostProduct(ctx, tx, g, headID, c); err != nil {
			return err
		}
	}
	if _, ok := g.headChanges["mbh_orion_item_code"]; ok {
		return w.lfFillOrion(ctx, tx, g, headID)
	}
	return nil
}

// cascadeHead runs the targeted cascade (NOT the UI cascade) and records before/after images
// of the spins and cost product it changed, so revert can restore them.
func (w *MBLegacyFillWriter) cascadeHead(ctx context.Context, tx *sql.Tx, g *lfGroup) (lfSideEffects, error) {
	var fx lfSideEffects
	headID := g.rows[0].Row.ID
	spinsBefore, err := lfSnapSpins(ctx, tx, headID)
	if err != nil {
		return fx, err
	}
	var pid int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(mbh_cost_product_id, 0) FROM mst_mb_head WHERE mbh_id = CAST($1 AS uuid)`, headID).Scan(&pid); err != nil {
		return fx, fmt.Errorf("read head cost product: %w", err)
	}
	var pb map[string]string
	if pid != 0 {
		if pb, err = lfSnapProduct(ctx, tx, pid); err != nil {
			return fx, err
		}
	}
	if err := w.lfApplyTargeted(ctx, tx, g, headID); err != nil {
		return fx, err
	}
	spinsAfter, err := lfSnapSpins(ctx, tx, headID)
	if err != nil {
		return fx, err
	}
	fx.CascadedSpinIDs = []string{}
	fx.SpinChanges = map[string]lfImage{}
	for id, b := range spinsBefore {
		if d := lfDiff(b, spinsAfter[id]); d != nil {
			fx.SpinChanges[id] = *d
			fx.CascadedSpinIDs = append(fx.CascadedSpinIDs, id)
		}
	}
	sort.Strings(fx.CascadedSpinIDs)
	if pid != 0 {
		pa, err := lfSnapProduct(ctx, tx, pid)
		if err != nil {
			return fx, err
		}
		fx.CostProductID = pid
		fx.CostProductChange = lfDiff(pb, pa)
	}
	return fx, nil
}
