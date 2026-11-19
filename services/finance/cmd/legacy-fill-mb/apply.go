package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
)

type applyDeps struct {
	w      mblegacyfill.Writer
	rc     mblegacyfill.Recalculator
	rs     mblegacyfill.OrionResolver
	impact mbdozing.ImpactRepository
	nm     *outNamer
}

// applyAll applies the selected APPLY rows batch by batch, then runs the post-commit
// recalc and affected-products pass over all results, writing the output CSVs.
// It returns an error when any row FAILED (after writing every file).
func applyAll(ctx context.Context, d applyDeps, rows []mblegacyfill.PlanRow, o options) error {
	spinHead, err := d.w.SpinHeads(ctx, rows)
	if err != nil {
		return fmt.Errorf("resolve spin heads: %w", err)
	}
	groups := selectApplyRows(rows, spinHead, splitList(o.onlySysID), o.limit)
	batches := chunkGroups(groups, o.batch)
	log.Printf("apply: %d head group(s) in %d batch(es)", len(groups), len(batches))
	var results []mblegacyfill.RowResult
	var runErr error
	for i, b := range batches {
		if ctx.Err() != nil {
			runErr = fmt.Errorf("interrupted before batch %d/%d: %w", i+1, len(batches), ctx.Err())
			break
		}
		res, err := d.w.Apply(ctx, o.actor, b)
		results = append(results, res...)
		if err != nil {
			runErr = fmt.Errorf("batch %d/%d: %w", i+1, len(batches), err)
			break
		}
		log.Printf("batch %d/%d done: %d row(s)", i+1, len(batches), len(res))
	}
	failed := countFailed(results)
	// Committed batches must get their recalc and output files even after SIGINT/SIGTERM.
	pctx := context.WithoutCancel(ctx)
	outs := mblegacyfill.RunPostEffects(pctx, d.rc, o.actor, mblegacyfill.SelectRecalcTargets(results))
	if err := writeApplyFiles(pctx, d, results, outs); err != nil && runErr == nil {
		runErr = err
	}
	log.Printf("apply summary: %v", summarize(results))
	if runErr == nil && failed > 0 {
		runErr = fmt.Errorf("%d row(s) FAILED (see result CSV)", failed)
	}
	return runErr
}

func countFailed(results []mblegacyfill.RowResult) int {
	n := 0
	for _, r := range results {
		if r.Result == mblegacyfill.ResultFailed {
			n++
		}
	}
	return n
}

func summarize(results []mblegacyfill.RowResult) map[string]int {
	m := map[string]int{}
	for _, r := range results {
		k := r.Result
		if r.Reason != "" && r.Result != mblegacyfill.ResultApplied {
			k += ":" + r.Reason
		}
		m[k]++
	}
	return m
}

func writeApplyFiles(ctx context.Context, d applyDeps, results []mblegacyfill.RowResult, outs []mblegacyfill.RecalcOutcome) error {
	var errs []error
	var failedRows []mblegacyfill.RowResult
	for _, r := range results {
		if r.Result == mblegacyfill.ResultFailed {
			failedRows = append(failedRows, r)
		}
	}
	files := []struct {
		path string
		fn   func(io.Writer) error
	}{
		{d.nm.path("result"), func(w io.Writer) error { return mblegacyfill.WriteResultCSV(w, results) }},
		{d.nm.path("errors"), func(w io.Writer) error { return mblegacyfill.WriteResultCSV(w, failedRows) }},
		{d.nm.path("recalc-results"), func(w io.Writer) error { return mblegacyfill.WriteRecalcCSV(w, outs) }},
		{d.nm.path("affected-products"), func(w io.Writer) error {
			rows, err := mblegacyfill.CollectAffectedProducts(ctx, d.impact, d.rs, results, outs)
			if err != nil {
				return fmt.Errorf("collect affected products: %w", err)
			}
			return mblegacyfill.WriteAffectedProductsCSV(w, rows)
		}},
	}
	for _, f := range files {
		if err := writeFile(f.path, f.fn); err != nil {
			errs = append(errs, err)
			continue
		}
		log.Printf("wrote %s", f.path)
	}
	return errors.Join(errs...)
}

// postEffectsOnly re-runs the recalc and affected-products passes over earlier apply result
// CSV rows (recovery after an interrupted apply) and writes the two output files.
func postEffectsOnly(ctx context.Context, d applyDeps, items []mblegacyfill.ResultRow, o options) error {
	results := mblegacyfill.ResultRowsToRowResults(items)
	targets := mblegacyfill.SelectRecalcTargets(results)
	log.Printf("post-effects-only: %d result row(s), %d recalc target(s)", len(results), len(targets))
	pctx := context.WithoutCancel(ctx)
	outs := mblegacyfill.RunPostEffects(pctx, d.rc, o.actor, targets)
	var errs []error
	files := []struct {
		path string
		fn   func(io.Writer) error
	}{
		{d.nm.path("recalc-results"), func(w io.Writer) error { return mblegacyfill.WriteRecalcCSV(w, outs) }},
		{d.nm.path("affected-products"), func(w io.Writer) error {
			rows, err := mblegacyfill.CollectAffectedProducts(pctx, d.impact, d.rs, results, outs)
			if err != nil {
				return fmt.Errorf("collect affected products: %w", err)
			}
			return mblegacyfill.WriteAffectedProductsCSV(w, rows)
		}},
	}
	for _, f := range files {
		if err := writeFile(f.path, f.fn); err != nil {
			errs = append(errs, err)
			continue
		}
		log.Printf("wrote %s", f.path)
	}
	return errors.Join(errs...)
}
