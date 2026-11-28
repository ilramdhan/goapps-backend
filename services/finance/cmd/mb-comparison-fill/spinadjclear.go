package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	fill "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomparisonfill"
)

func runAdj(ctx context.Context, o options, pg fill.AdjStore) error {
	switch o.mode {
	case modePlan:
		return runAdjPlan(ctx, o, pg)
	case modeApply:
		return runAdjApply(ctx, o, pg)
	}
	return runAdjRevert(ctx, o, pg)
}

func runAdjPlan(ctx context.Context, o options, pg fill.AdjStore) error {
	cand, err := pg.Candidates(ctx)
	if err != nil {
		return err
	}
	plan := fill.PlanAdjClear(cand)
	counts := map[fill.Decision]int{}
	for _, r := range plan {
		counts[r.Decision]++
	}
	log.Printf("spin-adj-clear plan: %d spins, decisions=%v", len(plan), counts)
	p, err := outPath(o.outDir, "spin-adj-clear-plan")
	if err != nil {
		return err
	}
	return writeFile(p, func(w io.Writer) error { return fill.WriteAdjPlanCSV(w, plan) })
}

func runAdjApply(ctx context.Context, o options, pg fill.AdjStore) error {
	rows, err := fill.ReadAdjCSV(o.planFile)
	if err != nil {
		return err
	}
	res, aerr := fill.ApplyAdjClear(ctx, pg, o.actor, rows, o.limit, o.batch, o.commit)
	kind := "spin-adj-clear-apply-result"
	if !o.commit {
		kind = "spin-adj-clear-apply-dryrun"
		log.Printf("DRY-RUN: nothing was written (use --commit to write)")
	}
	counts := map[string]int{}
	for _, r := range res {
		counts[r.Result]++
	}
	log.Printf("spin-adj-clear apply: %v", counts)
	p, perr := outPath(o.outDir, kind)
	if perr == nil {
		perr = writeFile(p, func(w io.Writer) error { return fill.WriteAdjResultCSV(w, res) })
	}
	if aerr == nil && counts[fill.ResultFailed] > 0 {
		aerr = fmt.Errorf("%d row(s) FAILED", counts[fill.ResultFailed])
	}
	return errors.Join(aerr, perr)
}

func runAdjRevert(ctx context.Context, o options, pg fill.AdjStore) error {
	items, err := fill.ReadAdjResultCSVs(o.results)
	if err != nil {
		return err
	}
	res, rerr := fill.RevertAdjClear(ctx, pg, o.actor, items)
	counts := map[string]int{}
	for _, r := range res {
		counts[r.Result]++
	}
	log.Printf("spin-adj-clear revert: %v", counts)
	p, perr := outPath(o.outDir, "spin-adj-clear-revert-result")
	if perr == nil {
		perr = writeFile(p, func(w io.Writer) error { return fill.WriteAdjResultCSV(w, res) })
	}
	if counts[fill.ResultFailed] > 0 {
		rerr = errors.Join(rerr, fmt.Errorf("%d revert item(s) FAILED", counts[fill.ResultFailed]))
	}
	return errors.Join(rerr, perr)
}
