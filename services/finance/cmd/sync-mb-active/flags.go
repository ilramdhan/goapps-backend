package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
)

const (
	cmdName = "sync-mb-active"
)

type options struct {
	plan   bool
	report bool
	apply  bool
	revert bool

	input     stringList // --input (--revert, repeatable)
	file      string
	outDir    string
	planRows  string
	planHeads string
	out       string
	phase     string
	actor     string
	batch     int
	limit     int
	onlySysID string

	prevResults stringList        // --prev-results (repeatable)
	prevClones  map[int]uuid.UUID // loaded from prevResults at apply time
	prevRoots   map[int]uuid.UUID // ROOT spins loaded from prevResults
	manualPlan  [][]string        // NEW_RECIPE/REPORT_* records for manual-items.csv (apply phase D)
}

// stringList is a repeatable string flag.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

// Set implements flag.Value.
func (l *stringList) Set(v string) error {
	if strings.TrimSpace(v) == "" {
		return errors.New("empty value")
	}
	*l = append(*l, v)
	return nil
}

func parseFlags(args []string) (options, error) {
	return parseFlagsTo(args, io.Discard)
}

// parseFlagsTo parses args; on -h/--help the usage (flag defaults) goes to usage and
// flag.ErrHelp is returned so main exits 0. Parse errors stay silent (main logs them).
func parseFlagsTo(args []string, usage io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet(cmdName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {} // usage only on -h/--help (below)
	fs.BoolVar(&o.plan, "plan", false, "read-only: build plan CSVs + summary from the DB and --file")
	fs.BoolVar(&o.report, "report", false, "read-only product-impact report (MB_SP_CODE + MB_SP_DOZING) from reviewed --plan-rows/--plan-heads")
	fs.BoolVar(&o.apply, "apply", false, "apply reviewed plan (phases A, B, C and D)")
	fs.BoolVar(&o.revert, "revert", false, "roll back APPLIED items of earlier result CSVs (--input) whose columns still hold the after-image")
	fs.Var(&o.input, "input", "result CSV to revert (--revert, repeatable, oldest first)")
	fs.StringVar(&o.file, "file", "", "mb-active file (normalized or original format), required for --plan")
	fs.StringVar(&o.outDir, "out-dir", ".", "output directory for generated files")
	fs.StringVar(&o.planRows, "plan-rows", "", "reviewed plan-rows CSV (--report/--apply)")
	fs.StringVar(&o.planHeads, "plan-heads", "", "reviewed plan-heads CSV (--report/--apply)")
	fs.StringVar(&o.out, "out", "", "output file (--report)")
	fs.StringVar(&o.phase, "phase", "", "phase A|B|C|D (--apply)")
	fs.StringVar(&o.actor, "actor", "", "actor name (--apply)")
	fs.IntVar(&o.batch, "batch", 0, "items per batch (--apply; 0 = one batch)")
	fs.IntVar(&o.limit, "limit", 0, "stop after N processed items (--apply; 0 = unlimited)")
	fs.StringVar(&o.onlySysID, "only-sysid", "", "comma-separated SYSIDs to apply (--apply)")
	fs.Var(&o.prevResults, "prev-results", "earlier result CSV (--apply, repeatable): ln -> clone_spin_id to resume duplicates")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if _, werr := fmt.Fprintf(usage, "Usage of %s (modes: --plan | --report | --apply | --revert, see README.md):\n", cmdName); werr != nil {
				return o, errors.Join(err, werr)
			}
			fs.SetOutput(usage)
			fs.PrintDefaults()
		}
		return o, err
	}
	return o, validateOptions(&o)
}

func validateOptions(o *options) error {
	modes := 0
	for _, m := range []bool{o.plan, o.report, o.apply, o.revert} {
		if m {
			modes++
		}
	}
	switch {
	case modes == 0:
		return errors.New("exactly one of --plan, --report, --apply, --revert is required")
	case modes > 1:
		return errors.New("--plan, --report, --apply and --revert are mutually exclusive")
	case o.outDir == "":
		return errors.New("--out-dir must not be empty")
	case o.plan:
		return validatePlanOptions(o)
	case o.report:
		return validateReportOptions(o)
	case o.apply:
		return validateApplyOptions(o)
	case o.revert:
		return validateRevertOptions(o)
	}
	return nil
}

func validatePlanOptions(o *options) error {
	switch {
	case o.file == "":
		return errors.New("--plan requires --file")
	case o.planRows != "" || o.planHeads != "" || o.phase != "" || o.actor != "" || o.out != "" || o.batch != 0 || o.limit != 0 || o.onlySysID != "" || len(o.prevResults) > 0 || len(o.input) > 0:
		return errors.New("--plan only accepts --file and --out-dir")
	}
	return nil
}

func validateReportOptions(o *options) error {
	switch {
	case o.planRows == "" || o.planHeads == "":
		return errors.New("--report requires --plan-rows and --plan-heads")
	case o.file != "" || o.phase != "" || o.actor != "" || o.batch != 0 || o.limit != 0 || o.onlySysID != "" || len(o.prevResults) > 0 || len(o.input) > 0:
		return errors.New("--report only accepts --plan-rows, --plan-heads, --out and --out-dir")
	}
	return nil
}

func validateApplyOptions(o *options) error {
	switch {
	case o.planRows == "" || o.planHeads == "":
		return errors.New("--apply requires --plan-rows and --plan-heads")
	case o.file != "" || o.out != "" || len(o.input) > 0:
		return errors.New("--apply does not accept --file, --out or --input")
	}
	return validateApplyPhaseOptions(o)
}

func validateApplyPhaseOptions(o *options) error {
	switch {
	case o.phase != "A" && o.phase != "B" && o.phase != "C" && o.phase != "D":
		return errors.New("--apply requires --phase A|B|C|D")
	case strings.TrimSpace(o.actor) == "":
		return errors.New("--apply requires a non-empty --actor")
	case o.batch < 0 || o.limit < 0:
		return errors.New("--batch and --limit must not be negative")
	case (o.phase == "C" || o.phase == "D") && o.batch <= 0:
		return errors.New("--apply --phase C|D requires --batch N (N > 0)")
	}
	return nil
}

func validateRevertOptions(o *options) error {
	switch {
	case len(o.input) == 0:
		return errors.New("--revert requires at least one --input result CSV")
	case strings.TrimSpace(o.actor) == "":
		return errors.New("--revert requires a non-empty --actor")
	case o.file != "" || o.planRows != "" || o.planHeads != "" || o.out != "" || o.phase != "" || o.batch != 0 || len(o.prevResults) > 0:
		return errors.New("--revert only accepts --input, --actor, --limit, --only-sysid and --out-dir")
	case o.limit < 0:
		return errors.New("--limit must not be negative")
	}
	return nil
}
