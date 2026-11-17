// Command sync-mb-active synchronizes MB active state with the Finance mb-active file.
//
// Modes (mutually exclusive):
//
//	--plan --file F [--out-dir D]   read-only: writes plan-rows, plan-heads CSV and a summary
//	--report                        declared, not implemented yet
//	--apply                         declared, not implemented yet
//
// --plan never writes to the database (the snapshot runs in a read-only transaction).
package main

import (
	"context"
	"errors"
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
	o, err := parseFlags(os.Args[1:])
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
	case o.report:
		return fmt.Errorf("--report: %s", errNotImpl)
	case o.apply:
		return fmt.Errorf("--apply: %s", errNotImpl)
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
