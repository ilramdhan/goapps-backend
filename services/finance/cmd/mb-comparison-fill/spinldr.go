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

func runSpin(ctx context.Context, o options, pg *postgres.MBComparisonSpinLDR) error {
	switch o.mode {
	case modePlan:
		return runSpinPlan(ctx, o, pg)
	case modeApply:
		return runSpinApply(ctx, o, pg)
	}
	return runSpinRevert(ctx, o, pg)
}

func runSpinPlan(ctx context.Context, o options, pg fill.SpinStore) error {
	rows, err := fill.ReadSpinCSV(o.file)
	if err != nil {
		return err
	}
	live, err := pg.Live(ctx, fill.SpinPlanIDs(rows))
	if err != nil {
		return err
	}
	plan := fill.PlanSpinLDR(rows, live)
	counts := map[fill.Decision]int{}
	for _, r := range plan {
		counts[r.Decision]++
	}
	log.Printf("spin-ldr plan: %d spins, decisions=%v", len(plan), counts)
	p, err := outPath(o.outDir, "spin-ldr-plan")
	if err != nil {
		return err
	}
	return writeFile(p, func(w io.Writer) error { return fill.WriteSpinPlanCSV(w, plan) })
}

func runSpinApply(ctx context.Context, o options, pg fill.SpinStore) error {
	rows, err := fill.ReadSpinCSV(o.planFile)
	if err != nil {
		return err
	}
	res, aerr := fill.ApplySpinLDR(ctx, pg, o.actor, rows, o.limit, o.batch, o.commit)
	kind := "spin-ldr-apply-result"
	if !o.commit {
		kind = "spin-ldr-apply-dryrun"
		log.Printf("DRY-RUN: nothing was written (use --commit to write)")
	}
	counts := map[string]int{}
	for _, r := range res {
		counts[r.Result]++
	}
	log.Printf("spin-ldr apply: %v", counts)
	p, perr := outPath(o.outDir, kind)
	if perr == nil {
		perr = writeFile(p, func(w io.Writer) error { return fill.WriteSpinResultCSV(w, res) })
	}
	if aerr == nil && counts[fill.ResultFailed] > 0 {
		aerr = fmt.Errorf("%d row(s) FAILED", counts[fill.ResultFailed])
	}
	return errors.Join(aerr, perr)
}

func runSpinRevert(ctx context.Context, o options, pg fill.SpinStore) error {
	items, err := fill.ReadSpinResultCSVs(o.results)
	if err != nil {
		return err
	}
	res, rerr := fill.RevertSpinLDR(ctx, pg, o.actor, items)
	counts := map[string]int{}
	for _, r := range res {
		counts[r.Result]++
	}
	log.Printf("spin-ldr revert: %v", counts)
	p, perr := outPath(o.outDir, "spin-ldr-revert-result")
	if perr == nil {
		perr = writeFile(p, func(w io.Writer) error { return fill.WriteSpinResultCSV(w, res) })
	}
	if counts[fill.ResultFailed] > 0 {
		rerr = errors.Join(rerr, fmt.Errorf("%d revert item(s) FAILED", counts[fill.ResultFailed]))
	}
	return errors.Join(rerr, perr)
}
