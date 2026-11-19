package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
)

// MBLegacyFillReverter restores the before-images recorded by MBLegacyFillWriter,
// one transaction per head: spin rows first, then the head rows (each head row
// also undoes the cascade it recorded). A column is restored only while it still
// holds the after-image. No cascade is re-run: exact before-images are written.
type MBLegacyFillReverter struct {
	db *DB
}

var _ mblegacyfill.Reverter = (*MBLegacyFillReverter)(nil)

// NewMBLegacyFillReverter creates the reverter.
func NewMBLegacyFillReverter(db *DB) *MBLegacyFillReverter { return &MBLegacyFillReverter{db: db} }

// lfRevTarget identifies a row to restore columns on.
type lfRevTarget struct {
	table, idCol, idCast, auditSet string
	id                             string
	soft                           bool // table has deleted_at
}

func lfEntityTarget(e mblegacyfill.Entity, id string) lfRevTarget {
	t := lfTables[e]
	return lfRevTarget{table: t.table, idCol: t.idCol, idCast: "uuid", id: id, soft: true,
		auditSet: "updated_at = NOW(), updated_by = $3"}
}

func lfProductTarget(pid int64) lfRevTarget {
	return lfRevTarget{table: "cost_product_master", idCol: "cpm_product_sys_id", idCast: "bigint",
		id: strconv.FormatInt(pid, 10), auditSet: "cpm_updated_at = NOW(), cpm_updated_by = $3"}
}

// lfRevCol is one column to restore.
type lfRevCol struct {
	col, before, after string
	kind               mblegacyfill.Kind
}

// lfRevState is the per-target verdict.
type lfRevState int

const (
	lfRevMissing lfRevState = iota
	lfRevRestored
	lfRevAlready
	lfRevChanged
)

func (s lfRevState) String() string {
	return [...]string{"entity_not_found", "restored", "already_reverted", "changed_since_apply"}[s]
}

func lfRevEqual(k mblegacyfill.Kind, a, b string) bool {
	if k == mblegacyfill.KindBool {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	return mblegacyfill.Equal(k, a, b)
}

func lfRevRead(ctx context.Context, tx *sql.Tx, t lfRevTarget, col string) (string, bool, error) {
	q := "SELECT COALESCE(CAST(" + col + " AS text), '') FROM " + t.table + //nolint:gosec // allow-listed identifiers
		" WHERE " + t.idCol + " = CAST($1 AS " + t.idCast + ")"
	if t.soft {
		q += " AND deleted_at IS NULL"
	}
	var v string
	err := tx.QueryRowContext(ctx, q+" FOR UPDATE", t.id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", col, err)
	}
	return v, true, nil
}

var lfRevSQLType = map[mblegacyfill.Kind]string{
	mblegacyfill.KindText: "text", mblegacyfill.KindNum: "numeric", mblegacyfill.KindInt: "numeric", mblegacyfill.KindBool: "boolean",
}

func lfRevWrite(ctx context.Context, tx *sql.Tx, t lfRevTarget, actor string, c lfRevCol) error {
	expr := "CAST(NULLIF($2, '') AS " + lfRevSQLType[c.kind] + ")"
	if c.kind == mblegacyfill.KindInt {
		expr = "CAST(" + expr + " AS integer)"
	}
	q := "UPDATE " + t.table + " SET " + c.col + " = " + expr + ", " + t.auditSet + //nolint:gosec // allow-listed identifiers
		" WHERE " + t.idCol + " = CAST($1 AS " + t.idCast + ")"
	if t.soft {
		q += " AND deleted_at IS NULL"
	}
	if _, err := tx.ExecContext(ctx, q, t.id, mblegacyfill.Normalize(c.before), actor); err != nil {
		return fmt.Errorf("restore %s: %w", c.col, err)
	}
	return nil
}

// lfRevertCols locks the target, decides per column and restores only when every
// column still holds its after-image (all-or-nothing per target).
func lfRevertCols(ctx context.Context, tx *sql.Tx, t lfRevTarget, actor string, cols []lfRevCol) (lfRevState, []string, error) {
	var notes []string
	var toRestore []lfRevCol
	changed := false
	for _, c := range cols {
		cur, found, err := lfRevRead(ctx, tx, t, c.col)
		if err != nil {
			return 0, nil, err
		}
		if !found {
			return lfRevMissing, nil, nil
		}
		switch {
		case lfRevEqual(c.kind, cur, c.before):
			notes = append(notes, c.col+":already_reverted")
		case lfRevEqual(c.kind, cur, c.after):
			toRestore = append(toRestore, c)
			notes = append(notes, c.col+":restored")
		default:
			changed = true
			notes = append(notes, c.col+":changed_since_apply")
		}
	}
	if changed {
		return lfRevChanged, notes, nil
	}
	for _, c := range toRestore {
		if err := lfRevWrite(ctx, tx, t, actor, c); err != nil {
			return 0, nil, err
		}
	}
	if len(toRestore) == 0 {
		return lfRevAlready, notes, nil
	}
	return lfRevRestored, notes, nil
}

// lfRevColsFromImage validates an image against the column allow-list.
func lfRevColsFromImage(before, after map[string]string, kindOf func(string) (mblegacyfill.Kind, bool)) ([]lfRevCol, error) {
	keys := make([]string, 0, len(before))
	for k := range before {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]lfRevCol, 0, len(keys))
	for _, k := range keys {
		kind, ok := kindOf(k)
		if !ok {
			return nil, fmt.Errorf("column %q is not revertable", k)
		}
		a, has := after[k]
		if !has {
			return nil, fmt.Errorf("after-image lacks column %q", k)
		}
		out = append(out, lfRevCol{col: k, before: before[k], after: a, kind: kind})
	}
	return out, nil
}

// Revert implements mblegacyfill.Reverter. Results are in input order.
func (w *MBLegacyFillReverter) Revert(ctx context.Context, actor string, items []mblegacyfill.ResultRow) ([]mblegacyfill.RevertResult, error) {
	res := make([]mblegacyfill.RevertResult, len(items))
	applied := make([]int, 0, len(items))
	for i, it := range items {
		res[i] = mblegacyfill.RevertResult{Item: it}
		if it.Result != mblegacyfill.ResultApplied {
			res[i].Result, res[i].Reason = mblegacyfill.RevertSkipped, mblegacyfill.ReasonNotApplied
			continue
		}
		applied = append(applied, i)
	}
	spinHead, err := w.spinHeadsOf(ctx, items, applied)
	if err != nil {
		return nil, err
	}
	groups, order := map[string][]int{}, []string{}
	for _, i := range applied {
		hid, ok := revHeadID(items[i], spinHead)
		if !ok {
			res[i].Result, res[i].Reason = mblegacyfill.RevertFailed, mblegacyfill.ReasonNotFound
			continue
		}
		if _, seen := groups[hid]; !seen {
			order = append(order, hid)
		}
		groups[hid] = append(groups[hid], i)
	}
	for _, hid := range order {
		w.revertGroup(ctx, actor, hid, items, groups[hid], res)
	}
	return res, nil
}

func revHeadID(it mblegacyfill.ResultRow, spinHead map[string]string) (string, bool) {
	id := strings.ToLower(it.ID)
	if _, err := uuid.Parse(id); err != nil {
		return "", false
	}
	if it.Entity == mblegacyfill.EntitySpin {
		h, ok := spinHead[id]
		return h, ok
	}
	return id, it.Entity == mblegacyfill.EntityHead
}

func (w *MBLegacyFillReverter) spinHeadsOf(ctx context.Context, items []mblegacyfill.ResultRow, idx []int) (m map[string]string, err error) {
	var ids []string
	for _, i := range idx {
		if items[i].Entity != mblegacyfill.EntitySpin {
			continue
		}
		if _, perr := uuid.Parse(items[i].ID); perr == nil {
			ids = append(ids, items[i].ID)
		}
	}
	m = map[string]string{}
	if len(ids) == 0 {
		return m, nil
	}
	rs, err := w.db.QueryContext(ctx, `SELECT CAST(mbs_id AS text), CAST(mbs_mbh_id AS text) FROM mst_mb_spin
		WHERE mbs_id = ANY(CAST($1 AS uuid[]))`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("resolve spin heads: %w", err)
	}
	defer closeLFRows(rs, &err)
	for rs.Next() {
		var s, h string
		if err = rs.Scan(&s, &h); err != nil {
			return nil, fmt.Errorf("scan spin head: %w", err)
		}
		m[strings.ToLower(s)] = strings.ToLower(h)
	}
	return m, rs.Err()
}

// revertGroup runs one head tx. idxs are sorted: spin rows first, newest apply first.
func (w *MBLegacyFillReverter) revertGroup(ctx context.Context, actor, headID string, items []mblegacyfill.ResultRow, idxs []int, res []mblegacyfill.RevertResult) {
	sort.SliceStable(idxs, func(a, b int) bool {
		x, y := items[idxs[a]], items[idxs[b]]
		if (x.Entity == mblegacyfill.EntitySpin) != (y.Entity == mblegacyfill.EntitySpin) {
			return x.Entity == mblegacyfill.EntitySpin
		}
		if x.File != y.File {
			return x.File > y.File
		}
		return x.Line > y.Line
	})
	failAll := func(reason string) {
		for _, i := range idxs {
			res[i].Result, res[i].Reason, res[i].Columns, res[i].Cascade = mblegacyfill.RevertFailed, reason, nil, ""
		}
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		failAll("begin tx: " + err.Error())
		return
	}
	if err := w.revertRows(ctx, tx, actor, headID, items, idxs, res); err != nil {
		_ = tx.Rollback() //nolint:errcheck // already failing
		failAll(err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		failAll("commit: " + err.Error())
	}
}

func (w *MBLegacyFillReverter) revertRows(ctx context.Context, tx *sql.Tx, actor, headID string, items []mblegacyfill.ResultRow, idxs []int, res []mblegacyfill.RevertResult) error {
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM mst_mb_head WHERE mbh_id = CAST($1 AS uuid) FOR UPDATE`, headID); err != nil {
		return fmt.Errorf("lock head: %w", err)
	}
	for _, i := range idxs {
		if _, err := tx.ExecContext(ctx, "SAVEPOINT "+lfSavepoint); err != nil {
			return fmt.Errorf("savepoint: %w", err)
		}
		r, err := w.revertItem(ctx, tx, actor, items[i])
		if err != nil {
			if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+lfSavepoint); rbErr != nil {
				return fmt.Errorf("rollback savepoint: %w", rbErr)
			}
			r = mblegacyfill.RevertResult{Result: mblegacyfill.RevertFailed, Reason: err.Error()}
		}
		if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+lfSavepoint); err != nil {
			return fmt.Errorf("release savepoint: %w", err)
		}
		r.Item = items[i]
		res[i] = r
	}
	return nil
}

func lfRevResult(s lfRevState, notes []string) mblegacyfill.RevertResult {
	r := mblegacyfill.RevertResult{Columns: notes}
	switch s {
	case lfRevRestored:
		r.Result = mblegacyfill.RevertReverted
	case lfRevAlready:
		r.Result, r.Reason = mblegacyfill.RevertAlreadyReverted, mblegacyfill.ReasonAlreadyReverted
	case lfRevChanged:
		r.Result, r.Reason = mblegacyfill.RevertChangedSince, mblegacyfill.ReasonChangedSinceApp
	default:
		r.Result, r.Reason = mblegacyfill.RevertFailed, mblegacyfill.ReasonNotFound
	}
	return r
}

func (w *MBLegacyFillReverter) revertItem(ctx context.Context, tx *sql.Tx, actor string, it mblegacyfill.ResultRow) (mblegacyfill.RevertResult, error) {
	var before, after map[string]string
	if json.Unmarshal([]byte(it.BeforeJSON), &before) != nil || len(before) == 0 {
		return mblegacyfill.RevertResult{Result: mblegacyfill.RevertFailed, Reason: mblegacyfill.ReasonNoBeforeImage}, nil //nolint:nilerr // per-row failure is reported in the result
	}
	if err := json.Unmarshal([]byte(it.AfterJSON), &after); err != nil {
		return mblegacyfill.RevertResult{Result: mblegacyfill.RevertFailed, Reason: "bad after_json: " + err.Error()}, nil //nolint:nilerr // per-row failure is reported in the result
	}
	delete(after, mblegacyfill.ShadeMasterKey)
	cols, err := lfRevColsFromImage(before, after, func(c string) (mblegacyfill.Kind, bool) { return mblegacyfill.ColumnKind(it.Entity, c) })
	if err != nil {
		return mblegacyfill.RevertResult{Result: mblegacyfill.RevertFailed, Reason: err.Error()}, nil //nolint:nilerr // per-row failure is reported in the result
	}
	state, notes, err := lfRevertCols(ctx, tx, lfEntityTarget(it.Entity, it.ID), actor, cols)
	if err != nil {
		return mblegacyfill.RevertResult{}, err
	}
	r := lfRevResult(state, notes)
	if state == lfRevChanged || state == lfRevMissing || it.SideEffects == "" {
		return r, nil
	}
	cascade, cstate, conflict, err := revertCascade(ctx, tx, actor, it.SideEffects)
	if err != nil {
		return mblegacyfill.RevertResult{}, err
	}
	r.Cascade = cascade
	if state == lfRevAlready && cstate == lfRevRestored {
		r.Result, r.Reason = mblegacyfill.RevertReverted, ""
	}
	if conflict {
		// A cascaded spin/product was changed after the apply (or is gone): the item is
		// only partly reverted, so it must not read as a clean REVERTED.
		r.Result, r.Reason = mblegacyfill.RevertChangedSince, mblegacyfill.ReasonCascadePartial
	}
	return r, nil
}

// revertCascade restores the spin/cost-product columns the head cascade changed.
// conflict reports a cascaded target that is changed_since_apply or missing.
func revertCascade(ctx context.Context, tx *sql.Tx, actor, raw string) (string, lfRevState, bool, error) {
	var fx lfSideEffects
	if err := json.Unmarshal([]byte(raw), &fx); err != nil {
		return "", lfRevAlready, false, fmt.Errorf("bad side_effects_json: %w", err)
	}
	summary := map[string]string{}
	overall := lfRevAlready
	conflict := false
	ids := make([]string, 0, len(fx.SpinChanges))
	for id := range fx.SpinChanges {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		img := fx.SpinChanges[id]
		if _, err := uuid.Parse(id); err != nil {
			return "", overall, false, fmt.Errorf("cascade spin id %q: %w", id, err)
		}
		st, err := revertImage(ctx, tx, actor, lfEntityTarget(mblegacyfill.EntitySpin, id), img, func(c string) (mblegacyfill.Kind, bool) { return mblegacyfill.ColumnKind(mblegacyfill.EntitySpin, c) })
		if err != nil {
			return "", overall, false, err
		}
		summary["spin:"+id] = st.String()
		overall = mergeRevState(overall, st)
		conflict = conflict || st == lfRevChanged || st == lfRevMissing
	}
	if fx.CostProductChange != nil && fx.CostProductID != 0 {
		st, err := revertImage(ctx, tx, actor, lfProductTarget(fx.CostProductID), *fx.CostProductChange, mblegacyfill.CostProductKind)
		if err != nil {
			return "", overall, false, err
		}
		summary["cost_product:"+strconv.FormatInt(fx.CostProductID, 10)] = st.String()
		overall = mergeRevState(overall, st)
		conflict = conflict || st == lfRevChanged || st == lfRevMissing
	}
	b, err := json.Marshal(summary)
	if err != nil {
		return "", overall, false, fmt.Errorf("marshal cascade summary: %w", err)
	}
	return string(b), overall, conflict, nil
}

func revertImage(ctx context.Context, tx *sql.Tx, actor string, t lfRevTarget, img lfImage, kindOf func(string) (mblegacyfill.Kind, bool)) (lfRevState, error) {
	cols, err := lfRevColsFromImage(img.Before, img.After, kindOf)
	if err != nil {
		return 0, err
	}
	st, _, err := lfRevertCols(ctx, tx, t, actor, cols)
	return st, err
}

func mergeRevState(a, b lfRevState) lfRevState {
	if a == lfRevRestored || b == lfRevRestored {
		return lfRevRestored
	}
	return a
}
