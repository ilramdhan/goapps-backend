// Command mb-comparison-fill writes the Finance mb-comparison Excel values (cols 63-71) into
// cost_product_parameter (+ CAPP) of the matching active products.
//
//	mb-comparison-fill plan   --file dozing.csv --orion O-01b.csv [--out-dir D]
//	mb-comparison-fill apply  --plan-file PLAN.csv --actor mbcmp-<env> [--limit N] [--batch N] [--commit] [--out-dir D]
//	mb-comparison-fill revert --results RESULT.csv[,R2.csv] --actor mbcmp-<env> [--out-dir D]
//
//	mb-comparison-fill spin-ldr plan   --file 06-q1.csv [--out-dir D]
//	mb-comparison-fill spin-ldr apply  --plan-file PLAN.csv --actor mbcmp-<env> [--limit N] [--batch N] [--commit] [--out-dir D]
//	mb-comparison-fill spin-ldr revert --results RESULT.csv[,R2.csv] --actor mbcmp-<env> [--out-dir D]
//
// apply is a DRY-RUN (everything rolled back) unless --commit is given. See README.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	fill "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomparisonfill"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const (
	cmdName    = "mb-comparison-fill"
	modePlan   = "plan"
	modeApply  = "apply"
	modeRevert = "revert"
	modeSpin   = "spin-ldr"
)

type options struct {
	mode                         string
	file, orion, planFile, actor string
	results                      []string
	limit, batch                 int
	commit                       bool
	outDir                       string
	spin                         bool // spin-ldr mode: mode holds the sub-mode
}

func parseArgs(args []string, usage io.Writer) (o options, err error) {
	if len(args) == 0 {
		return o, errors.New("mode required: plan | apply | revert")
	}
	o.mode = args[0]
	if o.mode == modeSpin {
		if len(args) < 2 {
			return o, errors.New("spin-ldr needs a sub-mode: plan | apply | revert")
		}
		o.spin, o.mode, args = true, args[1], args[1:]
	}
	fs := flag.NewFlagSet(cmdName+" "+o.mode, flag.ContinueOnError)
	fs.SetOutput(usage)
	var results string
	fs.StringVar(&o.outDir, "out-dir", ".", "output directory")
	switch o.mode {
	case modePlan:
		fs.StringVar(&o.file, "file", "", "Excel dump CSV (dozing.csv); spin-ldr: 06-q1.csv")
		fs.StringVar(&o.orion, "orion", "", "Oracle head dump O-01b-head-all-by-sysid.csv (empty MB_SP_CODE fallback)")
	case modeApply:
		fs.StringVar(&o.planFile, "plan-file", "", "reviewed plan CSV")
		fs.StringVar(&o.actor, "actor", "", "actor, mbcmp-<env>")
		fs.IntVar(&o.limit, "limit", 0, "apply at most N APPLY rows (0 = all)")
		fs.IntVar(&o.batch, "batch", 100, "rows per transaction")
		fs.BoolVar(&o.commit, "commit", false, "really write (default: dry-run, rolled back)")
	case modeRevert:
		fs.StringVar(&results, "results", "", "apply result CSV(s), comma separated, oldest first")
		fs.StringVar(&o.actor, "actor", "", "actor, mbcmp-<env>")
	default:
		return o, fmt.Errorf("unknown mode %q (plan | apply | revert)", o.mode)
	}
	if err = fs.Parse(args[1:]); err != nil {
		return o, err
	}
	for _, p := range strings.Split(results, ",") {
		if p = strings.TrimSpace(p); p != "" {
			o.results = append(o.results, p)
		}
	}
	return o, o.validate()
}

func (o options) validate() error {
	switch {
	case o.outDir == "":
		return errors.New("--out-dir must not be empty")
	case o.mode == modePlan && (o.file == "" || (!o.spin && o.orion == "")):
		return errors.New("plan requires --file (and --orion unless spin-ldr)")
	case o.mode == modeApply && o.planFile == "":
		return errors.New("apply requires --plan-file")
	case o.mode == modeApply && (o.limit < 0 || o.batch < 0):
		return errors.New("--limit and --batch must not be negative")
	case o.mode == modeRevert && len(o.results) == 0:
		return errors.New("revert requires --results")
	case o.mode != modePlan:
		return fill.ValidateActor(o.actor)
	}
	return nil
}

func main() {
	o, err := parseArgs(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		log.Fatalf("%s: %v", cmdName, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err = run(ctx, o)
	stop()
	if err != nil {
		log.Fatalf("%s: %v", cmdName, err)
	}
}

func run(ctx context.Context, o options) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			log.Printf("WARN: close database: %v", cErr)
		}
	}()
	if o.spin {
		return runSpin(ctx, o, postgres.NewMBComparisonSpinLDR(db))
	}
	pg := postgres.NewMBComparisonFill(db)
	switch o.mode {
	case modePlan:
		return runPlan(ctx, o, pg)
	case modeApply:
		return runApply(ctx, o, pg)
	}
	return runRevert(ctx, o, pg)
}

func outPath(dir, kind string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create out dir: %w", err)
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%s-%s.csv", cmdName, kind, time.Now().Format("20060102-150405"))), nil
}

func writeFile(path string, fn func(io.Writer) error) (err error) {
	f, err := os.Create(path) //nolint:gosec // operator-supplied output path for a one-shot CLI
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() {
		if cErr := f.Close(); cErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", path, cErr)
		}
	}()
	if err = fn(f); err != nil {
		return err
	}
	log.Printf("wrote %s", path)
	return nil
}

func runPlan(ctx context.Context, o options, pg *postgres.MBComparisonFill) error {
	rows, err := fill.ParseExcel(o.file)
	if err != nil {
		return err
	}
	orion, err := fill.ParseOrion(o.orion)
	if err != nil {
		return err
	}
	prods, err := pg.Snapshot(ctx)
	if err != nil {
		return err
	}
	plan := fill.BuildPlan(rows, prods, orion)
	p, err := outPath(o.outDir, "plan")
	if err != nil {
		return err
	}
	if err = writeFile(p, func(w io.Writer) error { return fill.WritePlanCSV(w, plan) }); err != nil {
		return err
	}
	s, err := outPath(o.outDir, "plan-summary")
	if err != nil {
		return err
	}
	log.Printf("plan: %d excel rows, decisions=%v, products with CPP but no CAPP=%d", len(rows), plan.Counts(), plan.CPPWithoutCAPP)
	return writeFile(s, func(w io.Writer) error { return fill.WriteSummaryCSV(w, plan) })
}

func runApply(ctx context.Context, o options, pg *postgres.MBComparisonFill) error {
	rows, err := fill.ReadPlanCSV(o.planFile)
	if err != nil {
		return err
	}
	res, aerr := fill.Apply(ctx, pg, o.actor, rows, o.limit, o.batch, o.commit)
	kind := "apply-result"
	if !o.commit {
		kind = "apply-dryrun"
		log.Printf("DRY-RUN: nothing was written (use --commit to write)")
	}
	counts := map[string]int{}
	failed := 0
	for _, r := range res {
		counts[r.Result]++
		if r.Result == fill.ResultFailed {
			failed++
		}
	}
	log.Printf("apply: %v", counts)
	p, perr := outPath(o.outDir, kind)
	if perr == nil {
		perr = writeFile(p, func(w io.Writer) error { return fill.WriteResultCSV(w, res) })
	}
	if aerr == nil && failed > 0 {
		aerr = fmt.Errorf("%d row(s) FAILED", failed)
	}
	return errors.Join(aerr, perr)
}

func runRevert(ctx context.Context, o options, pg *postgres.MBComparisonFill) error {
	items, err := fill.ReadResultCSVs(o.results)
	if err != nil {
		return err
	}
	res, rerr := fill.Revert(ctx, pg, o.actor, items)
	counts := map[string]int{}
	for _, r := range res {
		counts[r.Result]++
	}
	log.Printf("revert: %v", counts)
	p, perr := outPath(o.outDir, "revert-result")
	if perr == nil {
		perr = writeFile(p, func(w io.Writer) error { return fill.WriteResultCSV(w, res) })
	}
	if counts[fill.ResultFailed] > 0 {
		rerr = errors.Join(rerr, fmt.Errorf("%d revert item(s) FAILED", counts[fill.ResultFailed]))
	}
	return errors.Join(rerr, perr)
}
