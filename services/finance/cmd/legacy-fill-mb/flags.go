package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

const cmdName = "legacy-fill-mb"

type options struct {
	plan            bool
	apply           bool
	revert          bool
	postEffectsOnly bool

	proposal  string
	planFile  string
	input     stringList
	actor     string
	batch     int
	limit     int
	onlySysID string
	outDir    string
}

// stringList is a repeatable string flag; each value may itself be comma separated.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

// Set implements flag.Value.
func (l *stringList) Set(v string) error {
	n := 0
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*l = append(*l, p)
			n++
		}
	}
	if n == 0 {
		return errors.New("empty value")
	}
	return nil
}

func parseFlags(args []string) (options, error) { return parseFlagsTo(args, io.Discard) }

// parseFlagsTo parses args; on -h/--help usage goes to usage and flag.ErrHelp is returned.
func parseFlagsTo(args []string, usage io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet(cmdName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.BoolVar(&o.plan, "plan", false, "read-only: build plan CSV + summary from the DB and --proposal")
	fs.BoolVar(&o.apply, "apply", false, "apply the APPLY rows of a reviewed --plan-file")
	fs.BoolVar(&o.revert, "revert", false, "restore before-images of APPLIED rows of earlier result CSVs (--input)")
	fs.BoolVar(&o.postEffectsOnly, "post-effects-only", false, "re-run recalc + affected-products from apply result CSV(s) (--input), e.g. after an interrupted apply")
	fs.StringVar(&o.proposal, "proposal", "", "oracle-update-proposal CSV (--plan)")
	fs.StringVar(&o.planFile, "plan-file", "", "reviewed plan CSV (--apply)")
	fs.Var(&o.input, "input", "apply result CSV(s) to revert (--revert/--post-effects-only, repeatable or comma separated, oldest first)")
	fs.StringVar(&o.actor, "actor", "", "actor name written to updated_by (--apply/--revert)")
	fs.IntVar(&o.batch, "batch", 0, "head groups (a head and all its spin rows) per batch (--apply; 0 = one batch)")
	fs.IntVar(&o.limit, "limit", 0, "stop after N APPLY rows (--apply; 0 = unlimited)")
	fs.StringVar(&o.onlySysID, "only-sysid", "", "comma-separated SYSIDs; selects whole head groups containing any such row (--apply)")
	fs.StringVar(&o.outDir, "out-dir", ".", "output directory for generated files")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if _, werr := fmt.Fprintf(usage, "Usage of %s (modes: --plan | --apply | --revert | --post-effects-only, see README.md):\n", cmdName); werr != nil {
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
	for _, m := range []bool{o.plan, o.apply, o.revert, o.postEffectsOnly} {
		if m {
			modes++
		}
	}
	switch {
	case modes == 0:
		return errors.New("exactly one of --plan, --apply, --revert, --post-effects-only is required")
	case modes > 1:
		return errors.New("--plan, --apply, --revert and --post-effects-only are mutually exclusive")
	case o.outDir == "":
		return errors.New("--out-dir must not be empty")
	case o.batch < 0 || o.limit < 0:
		return errors.New("--batch and --limit must not be negative")
	case o.plan:
		return validatePlan(o)
	case o.apply:
		return validateApply(o)
	case o.postEffectsOnly:
		return validatePostEffects(o)
	}
	return validateRevert(o)
}

func validatePlan(o *options) error {
	switch {
	case o.proposal == "":
		return errors.New("--plan requires --proposal")
	case o.planFile != "" || len(o.input) > 0 || o.actor != "" || o.batch != 0 || o.limit != 0 || o.onlySysID != "":
		return errors.New("--plan only accepts --proposal and --out-dir")
	}
	return nil
}

func validateApply(o *options) error {
	switch {
	case o.planFile == "":
		return errors.New("--apply requires --plan-file")
	case strings.TrimSpace(o.actor) == "":
		return errors.New("--apply requires a non-empty --actor")
	case o.proposal != "" || len(o.input) > 0:
		return errors.New("--apply does not accept --proposal or --input")
	}
	return nil
}

func validateRevert(o *options) error {
	switch {
	case len(o.input) == 0:
		return errors.New("--revert requires at least one --input result CSV")
	case strings.TrimSpace(o.actor) == "":
		return errors.New("--revert requires a non-empty --actor")
	case o.proposal != "" || o.planFile != "" || o.batch != 0 || o.limit != 0 || o.onlySysID != "":
		return errors.New("--revert only accepts --input, --actor and --out-dir")
	}
	return nil
}

func validatePostEffects(o *options) error {
	switch {
	case len(o.input) == 0:
		return errors.New("--post-effects-only requires at least one --input result CSV")
	case strings.TrimSpace(o.actor) == "":
		return errors.New("--post-effects-only requires a non-empty --actor")
	case o.proposal != "" || o.planFile != "" || o.batch != 0 || o.limit != 0 || o.onlySysID != "":
		return errors.New("--post-effects-only only accepts --input, --actor and --out-dir")
	}
	return nil
}
