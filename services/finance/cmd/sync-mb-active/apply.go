package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func buildApplier() (*mbactivesync.Applier, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("connect db: %w", err)
	}
	return mbactivesync.NewApplier(newWriter(db)).WithHeadWorkflow(newHeadWorkflow(db)), func() { logClose("database", db) }, nil
}

func splitCSVList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func runApply(ctx context.Context, o options) error {
	rows, err := readPlanCSV(o.planRows, mbactivesync.RowsRequiredColumns)
	if err != nil {
		return err
	}
	heads, err := readPlanCSV(o.planHeads, mbactivesync.HeadsRequiredColumns)
	if err != nil {
		return err
	}
	items, err := mbactivesync.LoadApplyItems(rows, heads)
	if err != nil {
		return err
	}
	if o.prevClones, o.prevRoots, err = readPrevResults(o.prevResults); err != nil {
		return err
	}
	if o.phase == "D" { // NEW_RECIPE / REPORT_* never apply; list them once, with the last phase
		o.manualPlan = mbactivesync.ManualPlanRecords(rows, heads)
	}
	ap, cleanup, err := buildApplier()
	if err != nil {
		return err
	}
	defer cleanup()
	return applyAndWrite(ctx, ap, items, o)
}

// applyAndWrite runs the applier writing result/errors CSV files into --out-dir.
func applyAndWrite(ctx context.Context, ap *mbactivesync.Applier, items []*mbactivesync.ApplyItem, o options) (err error) {
	if err := os.MkdirAll(o.outDir, 0o750); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}
	ts := time.Now().Format("20060102-150405")
	resPath := filepath.Join(o.outDir, fmt.Sprintf("%s-result-%s-%s.csv", cmdName, o.phase, ts))
	errPath := filepath.Join(o.outDir, fmt.Sprintf("%s-errors-%s-%s.csv", cmdName, o.phase, ts))
	manPath := filepath.Join(o.outDir, fmt.Sprintf("%s-manual-items-%s-%s.csv", cmdName, o.phase, ts))
	rf, err := os.Create(resPath) //nolint:gosec // operator-supplied output dir for a one-shot CLI
	if err != nil {
		return fmt.Errorf("create %s: %w", resPath, err)
	}
	defer func() {
		if cErr := rf.Close(); cErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", resPath, cErr)
		}
	}()
	ef, err := os.Create(errPath) //nolint:gosec // operator-supplied output dir for a one-shot CLI
	if err != nil {
		return fmt.Errorf("create %s: %w", errPath, err)
	}
	defer func() {
		if cErr := ef.Close(); cErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", errPath, cErr)
		}
	}()
	mf, err := os.Create(manPath) //nolint:gosec // operator-supplied output dir for a one-shot CLI
	if err != nil {
		return fmt.Errorf("create %s: %w", manPath, err)
	}
	defer func() {
		if cErr := mf.Close(); cErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", manPath, cErr)
		}
	}()
	sink := mbactivesync.NewCSVResultSink(rf, ef).WithManual(mf).WithManualPlan(o.manualPlan)
	opts := mbactivesync.ApplyOptions{
		Phase: mbactivesync.Phase(o.phase), Actor: o.actor, OnlySysID: splitCSVList(o.onlySysID),
		Limit: o.limit, Batch: o.batch, Logf: log.Printf, PrevClones: o.prevClones, PrevRoots: o.prevRoots,
	}
	sum, runErr := ap.Run(ctx, items, opts, sink)
	if cErr := sink.Close(); cErr != nil && runErr == nil {
		runErr = cErr
	}
	log.Printf("wrote %s", resPath)
	log.Printf("wrote %s (FAILED items; usable as --plan-rows for a rerun)", errPath)
	log.Printf("wrote %s (items needing manual follow-up)", manPath)
	if sum != nil {
		logApplySummary(sum)
	}
	return runErr
}

func logApplySummary(s *mbactivesync.ApplySummary) {
	log.Printf("selected %d processed %d stopped=%q results=%v reasons=%v\n", s.Selected, s.Processed, s.Stopped, s.ByResult, s.ByReason)
}

// readPrevResults loads ln -> clone_spin_id from the --prev-results files (§5.4).
func readPrevResults(paths []string) (clones, roots map[int]uuid.UUID, err error) {
	bs := make([][]byte, 0, len(paths))
	for _, p := range paths {
		b, err := os.ReadFile(p) //nolint:gosec // operator-supplied input path for a one-shot CLI
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", p, err)
		}
		bs = append(bs, b)
	}
	rd := func() []io.Reader {
		r := make([]io.Reader, len(bs))
		for i, b := range bs {
			r[i] = bytes.NewReader(b)
		}
		return r
	}
	if clones, err = mbactivesync.LoadPrevClones(rd()...); err != nil {
		return nil, nil, fmt.Errorf("--prev-results: %w", err)
	}
	if roots, err = mbactivesync.LoadPrevRoots(rd()...); err != nil {
		return nil, nil, fmt.Errorf("--prev-results: %w", err)
	}
	log.Printf("prev-results: %d clone(s), %d root spin(s) recorded", len(clones), len(roots))
	return clones, roots, nil
}

// runRevert reads the --input result CSVs and reverts their APPLIED items (design part 2 §8).
func runRevert(ctx context.Context, o options) error {
	bs := make([]io.Reader, 0, len(o.input))
	for _, p := range o.input {
		b, err := os.ReadFile(p) //nolint:gosec // operator-supplied input path for a one-shot CLI
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		bs = append(bs, bytes.NewReader(b))
	}
	items, err := mbactivesync.LoadRevertItems(bs...)
	if err != nil {
		return err
	}
	ap, cleanup, err := buildApplier()
	if err != nil {
		return err
	}
	defer cleanup()
	return revertAndWrite(ctx, ap, items, o)
}

// revertAndWrite runs the revert writing revert-result / manual-items CSV files.
func revertAndWrite(ctx context.Context, ap *mbactivesync.Applier, items []*mbactivesync.RevertItem, o options) (err error) {
	if err := os.MkdirAll(o.outDir, 0o750); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}
	ts := time.Now().Format("20060102-150405")
	resPath := filepath.Join(o.outDir, fmt.Sprintf("%s-revert-result-%s.csv", cmdName, ts))
	manPath := filepath.Join(o.outDir, fmt.Sprintf("%s-revert-manual-items-%s.csv", cmdName, ts))
	rf, err := os.Create(resPath) //nolint:gosec // operator-supplied output dir for a one-shot CLI
	if err != nil {
		return fmt.Errorf("create %s: %w", resPath, err)
	}
	defer func() {
		if cErr := rf.Close(); cErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", resPath, cErr)
		}
	}()
	mf, err := os.Create(manPath) //nolint:gosec // operator-supplied output dir for a one-shot CLI
	if err != nil {
		return fmt.Errorf("create %s: %w", manPath, err)
	}
	defer func() {
		if cErr := mf.Close(); cErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", manPath, cErr)
		}
	}()
	sink := mbactivesync.NewCSVResultSink(rf, nil).WithManual(mf)
	sum, runErr := ap.RunRevert(ctx, items, mbactivesync.RevertOptions{
		Actor: o.actor, OnlySysID: splitCSVList(o.onlySysID), Limit: o.limit, Logf: log.Printf,
	}, sink)
	if cErr := sink.Close(); cErr != nil && runErr == nil {
		runErr = cErr
	}
	log.Printf("wrote %s", resPath)
	log.Printf("wrote %s (items the revert could not restore)", manPath)
	if sum != nil {
		logApplySummary(sum)
	}
	return runErr
}
