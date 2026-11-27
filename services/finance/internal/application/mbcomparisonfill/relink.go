package mbcomparisonfill

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Relink mode: point cost_product_parameter.cpp_value_mb_spin_id of MB_SP_CODE at the spin the Excel means
// (E/D pointer relinks) or at a per-group child spin (CONFLICT). Writes no other param, no lookup sync.
// Spec: docs/mb-comparison-p2/PARITY-PLAN.md "New code (minimal)".

// Relink decisions (SKIP_SAME / REPORT are shared with the other modes).
const (
	RelinkPointer Decision = "POINTER" // set the pointer to the target spin
	RelinkChild   Decision = "CHILD"   // create/reuse the child, then point at it
)

// Input action values the planner consumes; every other action is ignored.
const (
	ActionRelink      = "RELINK"
	ActionRelinkNamed = "RELINK_TO_NAMED_HEAD_SPIN"
	ActionMoveToChild = "MOVE_TO_CHILD"

	// StatusKept is a child spin that revert left alone (still referenced or has children).
	StatusKept = "KEPT_REFERENCED"
	// DecisionChildDelete marks the revert rows that soft-delete a created child.
	DecisionChildDelete Decision = "CHILD_DELETE"
)

// RelinkRow is one product of the plan.
type RelinkRow struct {
	Row               int // 1-based data row in the plan CSV
	Env, Source       string
	Product           int64
	Item              string
	Kind              Decision // RelinkPointer | RelinkChild (what the input asked for)
	TargetSpin        string   // pointer target
	ParentSpin        string   // child parent
	ChildKey          string   // label only: children are grouped by (parent, den, fil, ldr)
	Den, LDR          float64  // child spec
	Fil               int
	SnapText, SnapPtr string // live MB_SP_CODE text + pointer at plan time (apply guard)
	Decision          Decision
	Reason            string
	Resolved          string // apply-time: spin to point at ("" = dry-run child that would be created)
	exportPtr         string // input's idea of the current pointer (plan-time stale check only)
	checkExport       bool
}

// RelinkProduct is the live state of a product's MB_SP_CODE row.
type RelinkProduct struct {
	Found, Active, Locked, HasRow bool
	Text, Ptr                     string
}

// RelinkSpin is the live state of a non-deleted spin (absent from the map = missing or deleted).
type RelinkSpin struct {
	Parent   string
	Den, LDR float64
	Fil      int
	Actual   bool
	Active   bool
}

// RelinkChildSpec identifies a child to reuse/create.
type RelinkChildSpec struct {
	Parent   string
	Den, LDR float64
	Fil      int
}

// RelinkResult is the outcome of one row (also the revert input).
type RelinkResult struct {
	Env          string
	Row          int
	Product      int64
	Decision     Decision
	Status       string
	Reason       string
	Before       string
	After        string
	ChildID      string
	ChildCreated bool
}

// RelinkImage is the before/after JSON of a pointer row.
type RelinkImage struct {
	Ptr  string `json:"ptr"`
	Text string `json:"text"`
}

// RelinkStore is the DB port of the relink mode.
type RelinkStore interface {
	Products(ctx context.Context, ids []int64) (map[int64]RelinkProduct, error)
	Spins(ctx context.Context, ids []string) (map[string]RelinkSpin, error)
	// EnsureChild reuses or creates (own tx; commit=false never writes: created=true, id="" when it would create).
	EnsureChild(ctx context.Context, actor string, c RelinkChildSpec, commit bool) (id string, created bool, reason string, err error)
	// ApplyBatch points products in ONE tx (lock + guard + savepoint per row); commit=false rolls back.
	ApplyBatch(ctx context.Context, actor string, rows []RelinkRow, commit bool) ([]RelinkResult, error)
	RevertPointer(ctx context.Context, actor string, r RelinkResult) (RelinkResult, error)
	// DeleteChild soft-deletes an unreferenced, childless spin; status is REVERTED, ALREADY_REVERTED or KEPT_REFERENCED.
	DeleteChild(ctx context.Context, actor, id string) (status, detail string, err error)
}

// ---- planning (pure) ----

func (r RelinkRow) spec() RelinkChildSpec {
	return RelinkChildSpec{Parent: r.ParentSpin, Den: Fix4(r.Den), Fil: r.Fil, LDR: Fix4(r.LDR)}
}

func (s RelinkSpin) matches(c RelinkChildSpec) bool {
	return s.Parent == c.Parent && s.Actual && s.Active && s.Fil == c.Fil && Fix4(s.Den) == Fix4(c.Den) && Fix4(s.LDR) == Fix4(c.LDR)
}

// RelinkProductIDs / RelinkSpinIDs list what PlanRelink needs from the store.
func RelinkProductIDs(rows []RelinkRow) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Product)
	}
	return ids
}

// RelinkSpinIDs lists target/parent spins plus the pointers the products hold now.
func RelinkSpinIDs(rows []RelinkRow, prods map[int64]RelinkProduct) []string {
	set := map[string]bool{}
	for _, r := range rows {
		for _, id := range []string{r.TargetSpin, r.ParentSpin, prods[r.Product].Ptr} {
			if id != "" {
				set[id] = true
			}
		}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// PlanRelink decides each row against the live state and snapshots text + pointer. Pure.
func PlanRelink(rows []RelinkRow, prods map[int64]RelinkProduct, spins map[string]RelinkSpin) []RelinkRow {
	seen := map[int64]int{}
	for _, r := range rows {
		seen[r.Product]++
	}
	out := make([]RelinkRow, len(rows))
	for i, r := range rows {
		p := prods[r.Product]
		r.SnapText, r.SnapPtr = p.Text, p.Ptr
		if seen[r.Product] > 1 {
			r.Decision, r.Reason = DecisionReport, "product_in_several_input_rows"
		} else {
			r.Decision, r.Reason = decideRelink(r, p, spins)
		}
		out[i] = r
	}
	return out
}

func decideRelink(r RelinkRow, p RelinkProduct, spins map[string]RelinkSpin) (Decision, string) {
	target := r.TargetSpin
	if r.Kind == RelinkChild {
		target = r.ParentSpin
	}
	_, targetOK := spins[target]
	switch {
	case !p.Found:
		return DecisionReport, "product_not_found"
	case !p.Active:
		return DecisionReport, "product_inactive"
	case p.Locked:
		return DecisionReport, "product_locked"
	case !p.HasRow:
		return DecisionReport, "no_mb_sp_code_row"
	case !targetOK:
		return DecisionReport, "target_spin_missing_or_deleted"
	case r.checkExport && !strings.EqualFold(r.exportPtr, p.Ptr):
		return DecisionReport, "pointer_changed_since_export"
	case r.Kind == RelinkPointer && strings.EqualFold(p.Ptr, r.TargetSpin):
		return DecisionSkipSame, "already_target"
	case r.Kind == RelinkChild && spins[p.Ptr].matches(r.spec()):
		return DecisionSkipSame, "already_on_matching_child"
	}
	return r.Kind, ""
}

// validate rejects a plan row that could widen what apply writes.
func (r RelinkRow) validate() error {
	if r.Product <= 0 {
		return errors.New("bad product_sys_id")
	}
	switch r.Kind {
	case RelinkPointer:
		if !isUUID(r.TargetSpin) {
			return errors.New("bad target_spin_id")
		}
	case RelinkChild:
		if !isUUID(r.ParentSpin) || r.ChildKey == "" || r.Fil <= 0 || r.Den <= 0 || Fix4(r.LDR) <= 0 {
			return errors.New("bad child spec")
		}
	default:
		return fmt.Errorf("bad kind %q", r.Kind)
	}
	return nil
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !strings.ContainsRune("0123456789abcdefABCDEF", c):
			return false
		}
	}
	return true
}

// ---- apply / revert ----

type childRes struct {
	id      string
	created bool
	reason  string
}

// ApplyRelink runs the POINTER/CHILD rows in batches. limit 0 = all, batch 0 = one batch.
func ApplyRelink(ctx context.Context, s RelinkStore, actor string, rows []RelinkRow, limit, batch int, commit bool) ([]RelinkResult, error) {
	envs := make([]string, len(rows))
	for i, r := range rows {
		envs[i] = r.Env
	}
	if _, err := checkActorEnv(actor, envs); err != nil {
		return nil, err
	}
	var todo []RelinkRow
	var out []RelinkResult
	for _, r := range rows {
		if r.Decision != RelinkPointer && r.Decision != RelinkChild {
			continue
		}
		if r.Decision != r.Kind || r.validate() != nil {
			out = append(out, RelinkResult{Env: r.Env, Row: r.Row, Product: r.Product, Decision: r.Decision, Status: ResultFailed, Reason: "plan_tampered"})
			continue
		}
		todo = append(todo, r)
	}
	sort.SliceStable(todo, func(i, j int) bool { return todo[i].Product < todo[j].Product }) // stable lock order
	if limit > 0 && len(todo) > limit {
		todo = todo[:limit]
	}
	if batch <= 0 {
		batch = len(todo)
	}
	children := map[RelinkChildSpec]*childRes{}
	for i := 0; i < len(todo); i += batch {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		res, err := applyRelinkBatch(ctx, s, actor, todo[i:min(i+batch, len(todo))], commit, children)
		out = append(out, res...)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func applyRelinkBatch(ctx context.Context, s RelinkStore, actor string, rows []RelinkRow, commit bool, children map[RelinkChildSpec]*childRes) ([]RelinkResult, error) {
	var out []RelinkResult
	var run []RelinkRow
	extra := map[int]RelinkResult{} // row -> child info
	for _, r := range rows {
		if r.Decision == RelinkPointer {
			r.Resolved = r.TargetSpin
			run = append(run, r)
			continue
		}
		c := children[r.spec()]
		first := false
		if c == nil {
			id, created, reason, err := s.EnsureChild(ctx, actor, r.spec(), commit)
			if err != nil {
				return out, err
			}
			c, first = &childRes{id: id, created: created, reason: reason}, true
			children[r.spec()] = c
		}
		if c.reason != "" {
			out = append(out, RelinkResult{Env: r.Env, Row: r.Row, Product: r.Product, Decision: r.Decision, Status: ResultFailed, Reason: "child: " + c.reason})
			continue
		}
		r.Resolved = c.id
		extra[r.Row] = RelinkResult{ChildID: c.id, ChildCreated: first && c.created}
		run = append(run, r)
	}
	if len(run) == 0 {
		return out, nil
	}
	res, err := s.ApplyBatch(ctx, actor, run, commit)
	envOf := map[int]string{}
	for _, r := range run {
		envOf[r.Row] = r.Env
	}
	for _, x := range res {
		x.Env = envOf[x.Row]
		if e, ok := extra[x.Row]; ok {
			x.ChildID, x.ChildCreated = e.ChildID, e.ChildCreated
		}
		out = append(out, x)
	}
	return out, err
}

func checkActorEnv(actor string, envs []string) (string, error) {
	if err := ValidateActor(actor); err != nil {
		return "", err
	}
	env := ""
	for _, e := range envs {
		if e == "" || (env != "" && e != env) {
			return "", errors.New("empty or mixed env: one env per run")
		}
		env = e
	}
	if env != "" && actor != ActorPrefix+env {
		return "", fmt.Errorf("actor %q must be %s<env> = %q", actor, ActorPrefix, ActorPrefix+env)
	}
	return env, nil
}

// childCandidates lists every child id any row recorded, whatever the row status: the child commits in its own tx,
// so a FAILED first row or a re-run that reused it still leaves it behind. DeleteChild is the guard.
func childCandidates(items []RelinkResult) []string {
	var cands []string
	for _, it := range items {
		if it.ChildID != "" && !slices.Contains(cands, it.ChildID) {
			cands = append(cands, it.ChildID)
		}
	}
	return cands
}

// RevertRelink restores APPLIED pointer rows newest first, then soft-deletes the candidate children
// (only when unreferenced, childless and created by the actor).
func RevertRelink(ctx context.Context, s RelinkStore, actor string, items []RelinkResult) ([]RelinkResult, error) {
	envs := make([]string, len(items))
	for i, it := range items {
		envs[i] = it.Env
	}
	env, err := checkActorEnv(actor, envs)
	if err != nil {
		return nil, err
	}
	cands := childCandidates(items)
	var out []RelinkResult
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		if it.Status != ResultApplied || (it.Decision != RelinkPointer && it.Decision != RelinkChild) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out, err
		}
		r, err := s.RevertPointer(ctx, actor, it)
		if err != nil {
			r = it
			r.Status, r.Reason = ResultFailed, err.Error()
		}
		out = append(out, r)
	}
	for _, id := range cands {
		status, detail, err := s.DeleteChild(ctx, actor, id)
		if err != nil {
			status, detail = ResultFailed, err.Error()
		}
		out = append(out, RelinkResult{Env: env, Decision: DecisionChildDelete, Status: status, Reason: detail, ChildID: id})
	}
	return out, nil
}

// ---- CSV ----

var relinkPlanHeader = []string{"env", "source", "product_sys_id", "item_code", "kind", "target_spin_id", "parent_spin_id", "child_key",
	"den", "fil", "ldr", "snap_text", "snap_ptr", "decision", "reason"}

// RelinkResultHeader is the result CSV header.
var RelinkResultHeader = []string{"env", "row", "product_sys_id", "decision", "status", "reason", "before_json", "after_json", "child_spin_id", "child_created"}

func csvMaps(path string) ([]map[string]string, error) {
	recs, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("%s: empty csv", path)
	}
	var out []map[string]string
	for _, c := range recs[1:] {
		m := make(map[string]string, len(c))
		for i, h := range recs[0] {
			if i < len(c) {
				m[h] = strings.TrimSpace(c[i])
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func needCols(path string, ms []map[string]string, cols ...string) error {
	if len(ms) == 0 {
		return nil
	}
	for _, c := range cols {
		if _, ok := ms[0][c]; !ok {
			return fmt.Errorf("%s: column %q missing", path, c)
		}
	}
	return nil
}

func parseProduct(s string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("bad product_sys_id %q", s)
	}
	return v, nil
}

// ParseDenFilLDR parses "250.00/144/1.4500".
func ParseDenFilLDR(s string) (den float64, fil int, ldr float64, err error) {
	p := strings.Split(strings.TrimSpace(s), "/")
	if len(p) != 3 {
		return 0, 0, 0, fmt.Errorf("bad den/fil/ldr %q", s)
	}
	var e1, e2, e3 error
	den, e1 = strconv.ParseFloat(p[0], 64)
	fil, e2 = strconv.Atoi(p[1])
	ldr, e3 = strconv.ParseFloat(p[2], 64)
	if err = errors.Join(e1, e2, e3); err != nil {
		return 0, 0, 0, fmt.Errorf("bad den/fil/ldr %q", s)
	}
	return den, fil, ldr, nil
}

// ReadRelinkInputs reads the sql/10 exports (E, optional D and CONFLICT). Only RELINK,
// RELINK_TO_NAMED_HEAD_SPIN and MOVE_TO_CHILD rows are used; mixed env values are rejected.
func ReadRelinkInputs(ePath, dPath, cPath string) ([]RelinkRow, error) {
	var rows []RelinkRow
	for _, in := range []struct {
		path, source, action string
		cols                 []string
	}{
		{ePath, "E", ActionRelink, []string{"env", "product_sys_id", "item_code", "current_spin_id", "target_spin_id", "action"}},
		{dPath, "D", ActionRelinkNamed, []string{"env", "product_sys_id", "goapps_value", "target_spin_id", "action"}},
		{cPath, "CONFLICT", ActionMoveToChild, []string{"env", "product_sys_id", "item_code", "parent_spin_id", "target_den_fil_ldr", "child_key", "action"}},
	} {
		if in.path == "" {
			continue
		}
		ms, err := csvMaps(in.path)
		if err != nil {
			return nil, err
		}
		if err = needCols(in.path, ms, in.cols...); err != nil {
			return nil, err
		}
		for i, m := range ms {
			if m["action"] != in.action {
				continue
			}
			r, perr := relinkInputRow(in.source, m)
			if perr != nil {
				return nil, fmt.Errorf("%s row %d: %w", in.path, i+2, perr)
			}
			rows = append(rows, r)
		}
	}
	return rows, checkEnv(rows)
}

func relinkInputRow(source string, m map[string]string) (r RelinkRow, err error) {
	r.Source, r.Env, r.Item = source, m["env"], m["item_code"]
	if r.Product, err = parseProduct(m["product_sys_id"]); err != nil {
		return r, err
	}
	switch source {
	case "CONFLICT":
		r.Kind, r.ParentSpin, r.ChildKey = RelinkChild, strings.ToLower(m["parent_spin_id"]), m["child_key"]
		r.Den, r.Fil, r.LDR, err = ParseDenFilLDR(m["target_den_fil_ldr"])
	case "D":
		r.Kind, r.TargetSpin = RelinkPointer, strings.ToLower(m["target_spin_id"])
		r.exportPtr, r.checkExport = m["goapps_value"], true
	default:
		r.Kind, r.TargetSpin = RelinkPointer, strings.ToLower(m["target_spin_id"])
		r.exportPtr, r.checkExport = m["current_spin_id"], true
	}
	if err != nil {
		return r, err
	}
	return r, r.validate()
}

func checkEnv(rows []RelinkRow) error {
	for _, r := range rows {
		if r.Env == "" || r.Env != rows[0].Env {
			return fmt.Errorf("mixed or empty env column (%q vs %q): one env per run", r.Env, rows[0].Env)
		}
	}
	return nil
}

func relinkRecord(r RelinkRow) []string {
	return []string{r.Env, r.Source, strconv.FormatInt(r.Product, 10), r.Item, string(r.Kind), r.TargetSpin, r.ParentSpin, r.ChildKey,
		strconv.FormatFloat(r.Den, 'f', -1, 64), strconv.Itoa(r.Fil), strconv.FormatFloat(r.LDR, 'f', -1, 64),
		r.SnapText, r.SnapPtr, string(r.Decision), r.Reason}
}

// WriteRelinkPlanCSV writes the plan.
func WriteRelinkPlanCSV(w io.Writer, rows []RelinkRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(relinkPlanHeader); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, r := range rows {
		if err := cw.Write(relinkRecord(r)); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// ReadRelinkPlanCSV reads a plan CSV written by WriteRelinkPlanCSV (the reviewed file apply consumes).
func ReadRelinkPlanCSV(path string) ([]RelinkRow, error) {
	recs, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 || !slices.Equal(recs[0], relinkPlanHeader) {
		return nil, fmt.Errorf("%s: not a relink plan csv (header mismatch)", path)
	}
	var rows []RelinkRow
	for i, c := range recs[1:] {
		if len(c) != len(relinkPlanHeader) {
			return nil, fmt.Errorf("%s row %d: column count mismatch", path, i+2)
		}
		r := RelinkRow{Row: i + 1, Env: c[0], Source: c[1], Item: c[3], Kind: Decision(c[4]), TargetSpin: c[5], ParentSpin: c[6],
			ChildKey: c[7], SnapText: c[11], SnapPtr: c[12], Decision: Decision(c[13]), Reason: c[14]}
		var e1, e2, e3, e4 error
		r.Product, e1 = strconv.ParseInt(c[2], 10, 64)
		if c[8] != "" {
			r.Den, e2 = strconv.ParseFloat(c[8], 64)
			r.Fil, e3 = strconv.Atoi(c[9])
			r.LDR, e4 = strconv.ParseFloat(c[10], 64)
		}
		if err = errors.Join(e1, e2, e3, e4); err != nil {
			return nil, fmt.Errorf("%s row %d: %w", path, i+2, err)
		}
		rows = append(rows, r)
	}
	return rows, checkEnv(rows)
}

// WriteRelinkResultCSV writes results.
func WriteRelinkResultCSV(w io.Writer, rs []RelinkResult) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(RelinkResultHeader); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, r := range rs {
		if err := cw.Write([]string{r.Env, strconv.Itoa(r.Row), strconv.FormatInt(r.Product, 10), string(r.Decision), r.Status, r.Reason,
			r.Before, r.After, r.ChildID, strconv.FormatBool(r.ChildCreated)}); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// ReadRelinkResultCSVs loads result CSVs in order (oldest first).
func ReadRelinkResultCSVs(paths []string) ([]RelinkResult, error) {
	var out []RelinkResult
	for _, p := range paths {
		recs, err := readCSV(p)
		if err != nil {
			return nil, err
		}
		if len(recs) == 0 || !slices.Equal(recs[0], RelinkResultHeader) {
			return nil, fmt.Errorf("%s: not a relink result csv (header mismatch)", p)
		}
		for _, c := range recs[1:] {
			if len(c) != len(RelinkResultHeader) {
				return nil, errors.New("result row column count mismatch in " + p)
			}
			row, e1 := strconv.Atoi(c[1])
			pid, e2 := strconv.ParseInt(c[2], 10, 64)
			if err = errors.Join(e1, e2); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			out = append(out, RelinkResult{Env: c[0], Row: row, Product: pid, Decision: Decision(c[3]), Status: c[4], Reason: c[5],
				Before: c[6], After: c[7], ChildID: c[8], ChildCreated: c[9] == "true"})
		}
	}
	return out, nil
}

// EncodeRelinkImage renders a before/after image.
func EncodeRelinkImage(i RelinkImage) string {
	return fmt.Sprintf(`{"ptr":%s,"text":%s}`, quoteJSON(i.Ptr), quoteJSON(i.Text))
}

func quoteJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil { // a string cannot fail; keep the image valid JSON regardless
		return `""`
	}
	return string(b)
}
