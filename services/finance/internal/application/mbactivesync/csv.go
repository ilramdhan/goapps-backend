package mbactivesync

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// RowsHeader is the fixed plan-rows.csv header (design §3.8).
var RowsHeader = []string{
	"ln", "sysid", "file_poy", "file_poy_name", "file_shade", "file_shade_name", "file_mbcode", "file_dye", "file_doz",
	"den", "fil", "cs",
	"head_id", "head_name", "head_live", "head_entry_status", "head_cost_product_id", "head_fg_product_id",
	"poy_product_id", "poy_shade_exact",
	"spin_id", "spin_live", "spin_shade", "spin_ldr", "spin_ldr_type", "spin_fg", "alloc_lvl", "alloc_rank", "sh_eq", "ldr_d",
	"action", "role", "reason", "sub_actions", "phase",
	"source_spin_id",
	"target_fg", "target_shade_code", "target_shade_name", "target_den", "target_fil", "target_cs", "target_ldr",
	"item_key", "decision",
}

// HeadsHeader is the fixed plan-heads.csv header (design §3.8).
var HeadsHeader = []string{
	"item_key", "kind", "old_head_id", "old_sysid", "old_name", "old_spin_id", "old_spin_live",
	"new_head_id", "new_sysid", "new_name", "new_spin_ref", "new_head_live",
	"ref_products_pointer", "ref_products_text", "phase", "reason", "decision",
}

// RowsRequiredColumns are the columns apply needs; a missing one is an error.
var RowsRequiredColumns = []string{
	"ln", "sysid", "action", "phase", "sub_actions", "item_key", "decision",
	"head_id", "spin_id", "source_spin_id", "target_fg", "target_shade_code", "target_shade_name",
	"target_den", "target_fil", "target_cs", "target_ldr",
}

// HeadsRequiredColumns are the columns apply needs from plan-heads.csv.
var HeadsRequiredColumns = []string{"item_key", "kind", "old_head_id", "old_spin_id", "new_head_id", "new_spin_ref", "phase", "decision"}

const (
	sep      = ";"
	boolTrue = "true"
	boolNo   = "false"
)

func boolStr(b bool) string {
	if b {
		return boolTrue
	}
	return boolNo
}

func fmtF(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}

func fmtI(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

func fmtI64(p *int64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatInt(*p, 10)
}

// RowRecord renders one RowPlan as a CSV record matching RowsHeader.
func RowRecord(p *RowPlan) []string {
	r := p.Row
	rec := make([]string, 0, len(RowsHeader))
	rec = append(rec,
		strconv.Itoa(r.Ln), r.SysID, r.POY, r.POYName, r.Shade, r.ShadeName, r.MBCode, r.DyeName, r.DozRaw,
		fmtF(r.Den), fmtI(r.Fil), r.CS,
	)
	rec = append(rec, headCols(p.Head)...)
	rec = append(rec, poyCols(p.POY)...)
	rec = append(rec, spinCols(p)...)
	rec = append(rec, string(p.Action), string(p.Role), strings.Join(p.Reasons, sep), strings.Join(p.SubActions, sep), string(p.Phase))
	rec = append(rec, p.SourceRef())
	rec = append(rec, fmtI64(p.TargetFG), p.TargetShadeCode, p.TargetShadeName, fmtF(p.TargetDen), fmtI(p.TargetFil), p.TargetCS, fmtF(p.TargetLDR))
	rec = append(rec, "row:"+strconv.Itoa(r.Ln), string(p.Decision))
	return rec
}

func headCols(h *HeadSnap) []string {
	if h == nil {
		return make([]string, 6)
	}
	return []string{h.ID.String(), h.MBCosting, boolStr(h.Live()), h.EntryStatus, fmtI64(h.CostProductID), fmtI64(h.FGProductID)}
}

func poyCols(p *POYSnap) []string {
	if p == nil {
		return make([]string, 2)
	}
	return []string{strconv.FormatInt(p.SysID, 10), p.ShadeCode}
}

func spinCols(p *RowPlan) []string {
	sp := p.Spin
	a := p.Alloc
	// spin_id/spin_* describe the spin allocated to THIS row only. A spin_taken
	// row (p.Spin == nil) must leave them empty so an Applier never mistakes
	// another row's spin for its target; only the alloc_* diagnostics remain.
	out := make([]string, 10)
	if sp != nil {
		out[0], out[1], out[2] = sp.ID.String(), boolStr(sp.Live()), sp.ShadeCode
		out[3], out[4], out[5] = fmtF(sp.RunLDRPct), sp.LDRType, fmtI64(sp.FGProductID)
	}
	if a != nil {
		out[6], out[7] = strconv.Itoa(a.Lvl), strconv.Itoa(a.Rank)
		if a.ShEq != nil {
			out[8] = boolStr(*a.ShEq)
		}
		if a.LDRD != nil {
			out[9] = strconv.FormatFloat(float64(*a.LDRD)/float64(fixScale), 'f', 4, 64)
		}
	}
	return out
}

// HeadRecord renders one HeadAction as a CSV record matching HeadsHeader.
func HeadRecord(a *HeadAction) []string {
	oldSys, oldName := "", ""
	if a.OldHead != nil {
		oldSys, oldName = a.OldHead.SysID, a.OldHead.MBCosting
	}
	oldSpin, oldLive := "", ""
	if a.OldSpin != nil {
		oldSpin, oldLive = a.OldSpin.ID.String(), boolStr(a.OldSpin.Live())
	}
	newID, newSys, newName, newLive := "", a.NewSysID, "", ""
	if a.NewHead != nil {
		newID, newSys, newName, newLive = a.NewHead.ID.String(), a.NewHead.SysID, a.NewHead.MBCosting, boolStr(a.NewHead.Live())
	}
	oldID := ""
	if a.OldHead != nil {
		oldID = a.OldHead.ID.String()
	}
	return []string{
		a.ItemKey(), string(a.Kind), oldID, oldSys, oldName, oldSpin, oldLive,
		newID, newSys, newName, a.NewSpinRef(), newLive,
		strconv.Itoa(a.RefPointer), strconv.Itoa(a.RefText), string(a.Phase), strings.Join(a.Reasons, sep), string(a.Decision),
	}
}

// WriteRowsCSV writes plan-rows.csv (header + one record per row, ln order).
func WriteRowsCSV(w io.Writer, plans []*RowPlan) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(RowsHeader); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, p := range plans {
		if err := cw.Write(RowRecord(p)); err != nil {
			return fmt.Errorf("write row ln %d: %w", p.Row.Ln, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteHeadsCSV writes plan-heads.csv.
func WriteHeadsCSV(w io.Writer, hp *HeadPlan) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(HeadsHeader); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, a := range hp.Actions {
		if err := cw.Write(HeadRecord(a)); err != nil {
			return fmt.Errorf("write head item %s: %w", a.ItemKey(), err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// PlanCSV is a parsed plan CSV: column index by name plus data records.
type PlanCSV struct {
	Index   map[string]int
	Records [][]string
}

// Get returns the named column of record i ("" when absent).
func (c *PlanCSV) Get(i int, col string) string {
	idx, ok := c.Index[col]
	if !ok || i < 0 || i >= len(c.Records) || idx >= len(c.Records[i]) {
		return ""
	}
	return c.Records[i][idx]
}

// ReadPlanCSV parses a reviewed plan CSV (optional UTF-8 BOM), validating that all
// required columns exist before returning anything.
func ReadPlanCSV(r io.Reader, required []string) (*PlanCSV, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	all, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse plan csv: %w", err)
	}
	if len(all) == 0 {
		return nil, errors.New("plan csv is empty (no header)")
	}
	hdr := all[0]
	if len(hdr) > 0 {
		hdr[0] = strings.TrimPrefix(hdr[0], "\uFEFF")
	}
	idx := make(map[string]int, len(hdr))
	for i, h := range hdr {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	var missing []string
	for _, c := range required {
		if _, ok := idx[c]; !ok {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("plan csv missing required columns: %s", strings.Join(missing, ","))
	}
	for i, rec := range all[1:] {
		if len(rec) != len(hdr) {
			return nil, fmt.Errorf("plan csv record %d: want %d columns, got %d", i+1, len(hdr), len(rec))
		}
	}
	return &PlanCSV{Index: idx, Records: all[1:]}, nil
}

// Expected is one R2 §7 expectation.
type Expected struct {
	Name string
	Want int
}

// R2RowExpectations are the per-action row counts of R2 §7.
var R2RowExpectations = []Expected{
	{string(ActionKeep), 413}, {string(ActionUpdateLDR), 33}, {string(ActionUpdateShade), 63},
	{string(ActionReactivate), 1504}, {string(ActionDuplicateSpin), 1812}, {string(ActionGenerateSpin), 201},
	{string(ActionNewRecipe), 44}, {string(ActionReportOnly), 27},
}

// R2SysIDExpectations are the per-action distinct SYSID counts of R2 §7.
var R2SysIDExpectations = map[string]int{
	string(ActionKeep): 353, string(ActionUpdateLDR): 32, string(ActionUpdateShade): 40, string(ActionReactivate): 1416,
	string(ActionDuplicateSpin): 936, string(ActionGenerateSpin): 144, string(ActionNewRecipe): 43, string(ActionReportOnly): 21,
}

// R2HeadExpectations are the head-level expectations of R2 §7.
var R2HeadExpectations = []Expected{
	{"SUPERSEDE pairs", 13}, {"DEACTIVATE_HEAD", 170}, {"unallocated live spins", 120}, {"live spin on dead head", 14},
}

// WriteSummary writes the plain-text summary incl. expected-vs-actual vs R2 §7.
func WriteSummary(w io.Writer, pl *Plan) error {
	var b strings.Builder
	rows := pl.Rows
	fmt.Fprintf(&b, "mb-active-sync plan summary\nfile rows: %d, distinct SYSIDs: %d\n\n", len(rows), distinctPlanSysIDs(rows))

	byAction, sysByAction := map[string]int{}, map[string]map[string]bool{}
	byPhase, byReason, byDecision := map[string]int{}, map[string]int{}, map[string]int{}
	for _, p := range rows {
		a := string(p.Action)
		byAction[a]++
		if sysByAction[a] == nil {
			sysByAction[a] = map[string]bool{}
		}
		sysByAction[a][p.Row.SysID] = true
		byPhase[string(p.Phase)]++
		byDecision[string(p.Decision)]++
		for _, r := range p.Reasons {
			byReason[r]++
		}
	}
	b.WriteString("Rows: expected vs actual (R2 §7)\n")
	fmt.Fprintf(&b, "%-18s %9s %9s %7s | %9s %9s %7s\n", "action", "exp rows", "act rows", "delta", "exp sysid", "act sysid", "delta")
	totExp, totAct := 0, 0
	for _, e := range R2RowExpectations {
		act := byAction[e.Name]
		es, as := R2SysIDExpectations[e.Name], len(sysByAction[e.Name])
		fmt.Fprintf(&b, "%-18s %9d %9d %+7d | %9d %9d %+7d\n", e.Name, e.Want, act, act-e.Want, es, as, as-es)
		totExp += e.Want
		totAct += act
	}
	fmt.Fprintf(&b, "%-18s %9d %9d %+7d\n\n", "TOTAL", totExp, totAct, totAct-totExp)

	c := pl.Heads.Counts()
	unalloc, dead := 0, 0
	for _, a := range pl.Heads.Actions {
		if a.HasReason(ReasonUnallocated) {
			unalloc++
		}
		if a.HasReason(ReasonDeadHead) {
			dead++
		}
	}
	actual := map[string]int{"SUPERSEDE pairs": c.Pairs, "DEACTIVATE_HEAD": c.DeactivateHead, "unallocated live spins": unalloc, "live spin on dead head": dead}
	b.WriteString("Head/spin: expected vs actual (R2 §7)\n")
	fmt.Fprintf(&b, "%-26s %9s %9s %7s\n", "item", "expected", "actual", "delta")
	for _, e := range R2HeadExpectations {
		fmt.Fprintf(&b, "%-26s %9d %9d %+7d\n", e.Name, e.Want, actual[e.Name], actual[e.Name]-e.Want)
	}
	fmt.Fprintf(&b, "\nSupersede pairs: %d = spin-level %d (SUPERSEDE items=%d) + head-only %d (SUPERSEDE_HEAD items=%d)\n",
		c.Pairs, c.PairsSpinLevel, c.Supersede, c.PairsHeadOnly, c.SupersedeHead)
	fmt.Fprintf(&b, "Head actions: SUPERSEDE=%d SUPERSEDE_HEAD=%d DEACTIVATE_HEAD=%d DEACTIVATE_SPIN=%d REPORT_HEAD=%d REPORT_SPIN=%d\n\n",
		c.Supersede, c.SupersedeHead, c.DeactivateHead, c.DeactivateSpin, c.ReportHead, c.ReportSpin)

	writeCounts(&b, "Rows by action (all)", byAction)
	writeCounts(&b, "Rows by phase", byPhase)
	writeCounts(&b, "Rows by decision", byDecision)
	writeCounts(&b, "Rows by reason", byReason)
	hDec, hKind, hReason, hPhase := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	for _, a := range pl.Heads.Actions {
		hDec[string(a.Decision)]++
		hKind[string(a.Kind)]++
		hPhase[string(a.Phase)]++
		for _, r := range a.Reasons {
			hReason[r]++
		}
	}
	writeCounts(&b, "Head items by kind", hKind)
	writeCounts(&b, "Head items by phase", hPhase)
	writeCounts(&b, "Head items by decision", hDec)
	writeCounts(&b, "Head items by reason", hReason)
	_, err := io.WriteString(w, b.String())
	return err
}

func distinctPlanSysIDs(rows []*RowPlan) int {
	m := map[string]bool{}
	for _, p := range rows {
		m[p.Row.SysID] = true
	}
	return len(m)
}

func writeCounts(b *strings.Builder, title string, m map[string]int) {
	fmt.Fprintf(b, "%s:\n", title)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		name := k
		if name == "" {
			name = "(none)"
		}
		fmt.Fprintf(b, "  %-30s %d\n", name, m[k])
	}
	b.WriteString("\n")
}

// ResultHeader is the result-<phase>-<ts>.csv header (design §5).
var ResultHeader = []string{
	"item_key", "ln", "sysid", "action", "phase", "result", "reason", "spin_id", "clone_spin_id", "head_id",
	"before_json", "after_json", "message", "actor", "applied_at",
}

// ResultRecord renders one outcome matching ResultHeader.
func ResultRecord(o ApplyOutcome) []string {
	it := o.Item
	ln := ""
	if it.Ln > 0 {
		ln = strconv.Itoa(it.Ln)
	}
	return []string{
		it.Key, ln, it.SysID, it.Action, string(it.Phase), o.Result.Result, o.Result.Reason,
		o.Result.SpinID, o.Result.CloneSpinID, o.Result.HeadID, o.Result.BeforeJSON, o.Result.AfterJSON,
		o.Result.Message, o.Actor, o.AppliedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// CSVResultSink writes result rows (every outcome) and error rows (FAILED only, original
// plan columns + "error") flushing after every record so a crash loses nothing.
// errors may be nil. The error file's header is taken from the first failed item.
type CSVResultSink struct {
	res, errs     *csv.Writer
	manual        *csv.Writer
	started       bool
	errHeaderDone bool
	manualStarted bool
	manualPlan    [][]string
}

// ManualItemsHeader is the manual-items CSV header written by the apply sink for
// items that need an operator follow-up (ItemResult.ManualReason, e.g. Q1 guard).
var ManualItemsHeader = []string{"item_key", "ln", "sysid", "action", "phase", "reason", "spin_id", "clone_spin_id", "head_id", "message"}

// WithManual adds a manual-items writer (items with ItemResult.ManualReason).
func (s *CSVResultSink) WithManual(w io.Writer) *CSVResultSink {
	if w != nil {
		s.manual = csv.NewWriter(w)
	}
	return s
}

// WithManualPlan queues plan-level manual items (ManualPlanRecords: NEW_RECIPE,
// REPORT_ONLY, REPORT_SPIN/HEAD) written right after the header of the manual file.
func (s *CSVResultSink) WithManualPlan(recs [][]string) *CSVResultSink {
	s.manualPlan = recs
	return s
}

// startManual writes the manual-items header and the queued plan items once.
func (s *CSVResultSink) startManual() error {
	if s.manualStarted {
		return nil
	}
	if err := s.manual.Write(ManualItemsHeader); err != nil {
		return fmt.Errorf("write manual-items header: %w", err)
	}
	for _, r := range s.manualPlan {
		if err := s.manual.Write(r); err != nil {
			return fmt.Errorf("write manual plan item: %w", err)
		}
	}
	s.manualStarted = true
	return nil
}

func (s *CSVResultSink) recordManual(o ApplyOutcome) error {
	if s.manual == nil || o.Result.ManualReason == "" {
		return nil
	}
	if err := s.startManual(); err != nil {
		return err
	}
	it := o.Item
	ln := ""
	if it.Ln > 0 {
		ln = strconv.Itoa(it.Ln)
	}
	if err := s.manual.Write([]string{it.Key, ln, it.SysID, it.Action, string(it.Phase), o.Result.ManualReason,
		o.Result.SpinID, o.Result.CloneSpinID, o.Result.HeadID, o.Result.Message}); err != nil {
		return fmt.Errorf("write manual item: %w", err)
	}
	s.manual.Flush()
	if err := s.manual.Error(); err != nil {
		return fmt.Errorf("flush manual-items: %w", err)
	}
	return nil
}

// NewCSVResultSink returns a sink over the result writer and the (optional) error writer.
func NewCSVResultSink(result, errors io.Writer) *CSVResultSink {
	s := &CSVResultSink{res: csv.NewWriter(result)}
	if errors != nil {
		s.errs = csv.NewWriter(errors)
	}
	return s
}

// Record implements ResultSink.
func (s *CSVResultSink) Record(o ApplyOutcome) error {
	if !s.started {
		if err := s.res.Write(ResultHeader); err != nil {
			return fmt.Errorf("write result header: %w", err)
		}
		s.started = true
	}
	if err := s.res.Write(ResultRecord(o)); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	s.res.Flush()
	if err := s.res.Error(); err != nil {
		return fmt.Errorf("flush result: %w", err)
	}
	if err := s.recordManual(o); err != nil {
		return err
	}
	if o.Result.Result != ResultFailed || s.errs == nil {
		return nil
	}
	if !s.errHeaderDone {
		if err := s.errs.Write(append(append([]string{}, o.Item.Header...), "error")); err != nil {
			return fmt.Errorf("write errors header: %w", err)
		}
		s.errHeaderDone = true
	}
	if err := s.errs.Write(append(append([]string{}, o.Item.Record...), o.Err)); err != nil {
		return fmt.Errorf("write error row: %w", err)
	}
	s.errs.Flush()
	if err := s.errs.Error(); err != nil {
		return fmt.Errorf("flush errors: %w", err)
	}
	return nil
}

// Close writes the result header when nothing was recorded (so an empty run still
// leaves a valid file) and flushes.
func (s *CSVResultSink) Close() error {
	if !s.started {
		if err := s.res.Write(ResultHeader); err != nil {
			return fmt.Errorf("write result header: %w", err)
		}
		s.started = true
	}
	s.res.Flush()
	if err := s.closeManual(); err != nil {
		return err
	}
	return s.res.Error()
}

// closeManual writes the manual-items header when nothing was listed and flushes.
func (s *CSVResultSink) closeManual() error {
	if s.manual == nil {
		return nil
	}
	if err := s.startManual(); err != nil {
		return err
	}
	s.manual.Flush()
	if err := s.manual.Error(); err != nil {
		return fmt.Errorf("flush manual-items: %w", err)
	}
	return nil
}
