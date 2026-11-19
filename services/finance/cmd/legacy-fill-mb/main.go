// Command legacy-fill-mb fills MB head/spin columns from the reviewed Oracle legacy proposal CSV.
//
// Modes (mutually exclusive):
//
//	--plan --proposal P [--out-dir D]
//	    read-only: writes plan CSV (decision APPLY|CHANGED|ALREADY|REJECT per row) and a summary CSV.
//	--apply --plan-file F --actor NAME [--batch N] [--limit N] [--only-sysid S1,S2] [--out-dir D]
//	    writes to the DB: applies the APPLY rows of a reviewed plan (one tx per head), then recalcs
//	    R-and-D children and lists the products needing a lookup sync. Exit != 0 when any row FAILED.
//	--revert --input RESULT.csv[,RESULT2.csv] --actor NAME [--out-dir D]
//	    writes to the DB: restores before-images of APPLIED rows still holding their after-image.
//
//	--post-effects-only --input RESULT.csv[,RESULT2.csv] --actor NAME [--out-dir D]
//	    recovery: re-runs the recalc + affected-products passes from earlier apply result CSVs.
//
// Run order: legacy-fill-mb BEFORE sync-mb-active. See README.md.
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
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	o, err := parseFlagsTo(os.Args[1:], os.Stderr)
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
	switch {
	case o.revert:
		return runRevert(ctx, o)
	case o.postEffectsOnly:
		return runPostEffectsOnly(ctx, o)
	case o.apply:
		return runApply(ctx, o)
	case o.plan:
		return runPlan(ctx, o)
	}
	return errors.New("no mode selected")
}

// outPath returns <out-dir>/legacy-fill-mb-<kind>-<ts>.<ext>, creating the directory.
type outNamer struct {
	dir, ts string
}

func newOutNamer(dir string) (*outNamer, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create out dir: %w", err)
	}
	return &outNamer{dir: dir, ts: time.Now().Format("20060102-150405")}, nil
}

func (n *outNamer) path(kind string) string {
	return filepath.Join(n.dir, fmt.Sprintf("%s-%s-%s.csv", cmdName, kind, n.ts))
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
	return fn(f)
}
