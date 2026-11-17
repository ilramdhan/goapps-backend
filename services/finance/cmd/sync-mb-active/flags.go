package main

import (
	"errors"
	"flag"
	"io"
)

const (
	cmdName    = "sync-mb-active"
	errNotImpl = "not implemented yet"
)

type options struct {
	plan   bool
	report bool
	apply  bool

	file      string
	outDir    string
	planRows  string
	planHeads string
	out       string
	phase     string
	actor     string
}

func parseFlags(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet(cmdName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.plan, "plan", false, "read-only: build plan CSVs + summary from the DB and --file")
	fs.BoolVar(&o.report, "report", false, "read-only product-impact report (not implemented yet)")
	fs.BoolVar(&o.apply, "apply", false, "apply reviewed plan (not implemented yet)")
	fs.StringVar(&o.file, "file", "", "mb-active file (normalized or original format), required for --plan")
	fs.StringVar(&o.outDir, "out-dir", ".", "output directory for generated files")
	fs.StringVar(&o.planRows, "plan-rows", "", "reviewed plan-rows CSV (--report/--apply)")
	fs.StringVar(&o.planHeads, "plan-heads", "", "reviewed plan-heads CSV (--report/--apply)")
	fs.StringVar(&o.out, "out", "", "output file (--report)")
	fs.StringVar(&o.phase, "phase", "", "phase A|B|C|D (--apply)")
	fs.StringVar(&o.actor, "actor", "", "actor name (--apply)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	return o, validateOptions(&o)
}

func validateOptions(o *options) error {
	modes := 0
	for _, m := range []bool{o.plan, o.report, o.apply} {
		if m {
			modes++
		}
	}
	switch {
	case modes == 0:
		return errors.New("exactly one of --plan, --report, --apply is required")
	case modes > 1:
		return errors.New("--plan, --report and --apply are mutually exclusive")
	case o.plan && o.file == "":
		return errors.New("--plan requires --file")
	case o.plan && (o.planRows != "" || o.planHeads != "" || o.phase != "" || o.actor != "" || o.out != ""):
		return errors.New("--plan only accepts --file and --out-dir")
	case o.outDir == "":
		return errors.New("--out-dir must not be empty")
	}
	return nil
}
