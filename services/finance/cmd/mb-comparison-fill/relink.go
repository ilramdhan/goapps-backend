package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	fill "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomparisonfill"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func runRelink(ctx context.Context, o options, pg *postgres.MBComparisonRelink) error {
	switch o.mode {
	case modePlan:
		return runRelinkPlan(ctx, o, pg)
	case modeApply:
		return runRelinkApply(ctx, o, pg)
	}
	return runRelinkRevert(ctx, o, pg)
}

func runRelinkPlan(ctx context.Context, o options, pg fill.RelinkStore) error {
	rows, err := fill.ReadRelinkInputs(o.eFile, o.dFile, o.conflictFile)
	if err != nil {
		return err
	}
	prods, err := pg.Products(ctx, fill.RelinkProductIDs(rows))
	if err != nil {
		return err
	}
	spins, err := pg.Spins(ctx, fill.RelinkSpinIDs(rows, prods))
	if err != nil {
		return err
	}
	plan := fill.PlanRelink(rows, prods, spins)
	counts := map[fill.Decision]int{}
	for _, r := range plan {
		counts[r.Decision]++
	}
	log.Printf("relink plan: %d rows, decisions=%v", len(plan), counts)
	p, err := outPath(o.outDir, "relink-plan")
	if err != nil {
		return err
	}
	return writeFile(p, func(w io.Writer) error { return fill.WriteRelinkPlanCSV(w, plan) })
}

func runRelinkApply(ctx context.Context, o options, pg fill.RelinkStore) error {
	rows, err := fill.ReadRelinkPlanCSV(o.planFile)
	if err != nil {
		return err
	}
	res, aerr := fill.ApplyRelink(ctx, pg, o.actor, rows, o.limit, o.batch, o.commit)
	kind := "relink-apply-result"
	if !o.commit {
		kind = "relink-apply-dryrun"
		log.Printf("DRY-RUN: nothing was written (use --commit to write)")
	}
	return finishRelink(o, "relink apply", kind, res, aerr)
}

func runRelinkRevert(ctx context.Context, o options, pg fill.RelinkStore) error {
	items, err := fill.ReadRelinkResultCSVs(o.results)
	if err != nil {
		return err
	}
	res, rerr := fill.RevertRelink(ctx, pg, o.actor, items)
	return finishRelink(o, "relink revert", "relink-revert-result", res, rerr)
}

func finishRelink(o options, label, kind string, res []fill.RelinkResult, runErr error) error {
	counts := map[string]int{}
	for _, r := range res {
		counts[r.Status]++
	}
	log.Printf("%s: %v", label, counts)
	p, perr := outPath(o.outDir, kind)
	if perr == nil {
		perr = writeFile(p, func(w io.Writer) error { return fill.WriteRelinkResultCSV(w, res) })
	}
	if runErr == nil && counts[fill.ResultFailed] > 0 {
		runErr = fmt.Errorf("%d row(s) FAILED", counts[fill.ResultFailed])
	}
	return errors.Join(runErr, perr)
}
