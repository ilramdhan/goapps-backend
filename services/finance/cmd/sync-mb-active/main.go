// Command sync-mb-active synchronizes MB active state with the Finance mb-active file.
//
// Modes (mutually exclusive):
//
//	--plan --file F [--out-dir D]   read-only: writes plan-rows, plan-heads CSV and a summary
//	--report --plan-rows R --plan-heads H [--out F]
//	                                read-only: simulates MB_SP_CODE + MB_SP_DOZING product impact
//	--apply --plan-rows R --plan-heads H --phase A|B|C|D --actor NAME [--batch N] [--limit N] [--only-sysid S1,S2]
//	                                writes to the DB; result/errors/manual-items CSV in --out-dir.
//	                                Phases: A keep/LDR/shade/FG of existing spins, B supersede/deactivate,
//	                                C duplicate/generate spins (+ supersede), D reactivate (+ supersede).
//	                                --batch N (N > 0) is REQUIRED for phases C and D.
//	--revert --input RESULT.csv [--input ...] --actor NAME [--limit N] [--only-sysid S1,S2]
//	                                writes to the DB: restores the before-image of APPLIED items still holding
//	                                their after-image (reverse order); SUPERSEDE items are not reverted.
//
// --plan never writes to the database (the snapshot runs in a read-only transaction).
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

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
)

func main() {
	o, err := parseFlagsTo(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return // usage already printed to stderr; exit 0
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
	case o.report:
		return runReport(ctx, o)
	case o.apply:
		return runApply(ctx, o)
	case o.plan:
		return runPlan(ctx, o)
	}
	return errors.New("no mode selected")
}

func planAndWrite(ctx context.Context, planner *mbactivesync.Planner, rows []mbactivesync.FileRow, outDir string) error {
	pl, err := planner.Build(ctx, rows)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}
	ts := time.Now().Format("20060102-150405")
	name := func(kind, ext string) string {
		return filepath.Join(outDir, fmt.Sprintf("%s-%s-%s.%s", cmdName, kind, ts, ext))
	}
	files := []struct {
		path string
		fn   func(io.Writer) error
	}{
		{name("plan-rows", "csv"), func(w io.Writer) error { return mbactivesync.WriteRowsCSV(w, pl.Rows) }},
		{name("plan-heads", "csv"), func(w io.Writer) error { return mbactivesync.WriteHeadsCSV(w, pl.Heads) }},
		{name("plan-summary", "txt"), func(w io.Writer) error { return mbactivesync.WriteSummary(w, pl) }},
	}
	for _, f := range files {
		if err := writeFile(f.path, f.fn); err != nil {
			return err
		}
		log.Printf("wrote %s", f.path)
	}
	return nil
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
