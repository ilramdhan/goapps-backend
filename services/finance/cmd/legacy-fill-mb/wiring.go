package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
	appmbspin "github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

type closer interface{ Close() error }

func logClose(name string, c closer) {
	if err := c.Close(); err != nil {
		log.Printf("WARN: failed to close %s: %v", name, err)
	}
}

func openDB() (*postgres.DB, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("connect db: %w", err)
	}
	return db, func() { logClose("database", db) }, nil
}

func runPlan(ctx context.Context, o options) error {
	prop, err := mblegacyfill.ParseProposalFile(o.proposal)
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	db, cleanup, err := openDB()
	if err != nil {
		return err
	}
	defer cleanup()
	plan, err := mblegacyfill.BuildPlan(ctx, postgres.NewMBLegacyFillReader(db), prop)
	if err != nil {
		return err
	}
	nm, err := newOutNamer(o.outDir)
	if err != nil {
		return err
	}
	files := []struct {
		path string
		fn   func(io.Writer) error
	}{
		{nm.path("plan"), func(w io.Writer) error { return mblegacyfill.WritePlanCSV(w, plan) }},
		{nm.path("plan-summary"), func(w io.Writer) error { return mblegacyfill.WriteSummaryCSV(w, plan) }},
		{nm.path("plan-rejected"), func(w io.Writer) error { return writeRejected(w, prop) }},
	}
	for _, f := range files {
		if err := writeFile(f.path, f.fn); err != nil {
			return err
		}
		log.Printf("wrote %s", f.path)
	}
	log.Printf("plan: %d proposal rows, %d planned, %d rejected at parse, %d skipped (non executable) decisions=%v",
		prop.Total, len(plan.Rows), len(prop.Rejected), prop.SkippedTotal(), plan.Counts())
	return nil
}

func runApply(ctx context.Context, o options) error {
	rows, err := readPlanFile(o.planFile)
	if err != nil {
		return err
	}
	db, cleanup, err := openDB()
	if err != nil {
		return err
	}
	defer cleanup()
	nm, err := newOutNamer(o.outDir)
	if err != nil {
		return err
	}
	sr := newSpinRecalcer(db)
	return applyAll(ctx, applyDeps{
		w: postgres.NewMBLegacyFillWriter(db), rc: sr, rs: sr, impact: postgres.NewMBDozingImpactRepository(db), nm: nm,
	}, rows, o)
}

func newSpinRecalcer(db *postgres.DB) *mblegacyfill.SpinRecalcer {
	repo := postgres.NewMBSpinRepository(db)
	svc := appmbspin.NewRecalcService(repo, repo, postgres.NewMBDozingImpactRepository(db), postgres.NewMBCrossSectionFactorRepository(db))
	return &mblegacyfill.SpinRecalcer{Repo: repo, Svc: svc}
}

func runPostEffectsOnly(ctx context.Context, o options) error {
	items, err := mblegacyfill.ReadResultCSVs(o.input)
	if err != nil {
		return err
	}
	db, cleanup, err := openDB()
	if err != nil {
		return err
	}
	defer cleanup()
	nm, err := newOutNamer(o.outDir)
	if err != nil {
		return err
	}
	sr := newSpinRecalcer(db)
	return postEffectsOnly(ctx, applyDeps{
		rc: sr, rs: sr, impact: postgres.NewMBDozingImpactRepository(db), nm: nm,
	}, items, o)
}

func runRevert(ctx context.Context, o options) error {
	items, err := mblegacyfill.ReadResultCSVs(o.input)
	if err != nil {
		return err
	}
	db, cleanup, err := openDB()
	if err != nil {
		return err
	}
	defer cleanup()
	nm, err := newOutNamer(o.outDir)
	if err != nil {
		return err
	}
	res, err := postgres.NewMBLegacyFillReverter(db).Revert(ctx, o.actor, items)
	if err != nil {
		return err
	}
	path := nm.path("revert-result")
	if werr := writeFile(path, func(w io.Writer) error { return mblegacyfill.WriteRevertCSV(w, res) }); werr != nil {
		return werr
	}
	log.Printf("wrote %s", path)
	sum := mblegacyfill.RevertSummary(res)
	log.Printf("revert summary: %v", sum)
	rerr := revertPostEffects(ctx, db, nm, o.actor, res)
	if n := sum[mblegacyfill.RevertFailed]; n > 0 {
		return errors.Join(fmt.Errorf("%d revert item(s) FAILED", n), rerr)
	}
	return rerr
}

// revertPostEffects recalcs the R/D children of spins whose den/fil/doz the revert restored
// (the apply-time recalc results are otherwise left in place) and lists affected products.
// It runs detached from ctx so a committed revert always gets its recalc and files.
func revertPostEffects(ctx context.Context, db *postgres.DB, nm *outNamer, actor string, res []mblegacyfill.RevertResult) error {
	pctx := context.WithoutCancel(ctx)
	sr := newSpinRecalcer(db)
	targets := mblegacyfill.SelectRevertRecalcTargets(res)
	log.Printf("revert post-effects: %d recalc target(s)", len(targets))
	outs := mblegacyfill.RunPostEffects(pctx, sr, actor, targets)
	impact := postgres.NewMBDozingImpactRepository(db)
	rows := mblegacyfill.RevertedRowResults(res)
	files := []struct {
		path string
		fn   func(io.Writer) error
	}{
		{nm.path("revert-recalc-results"), func(w io.Writer) error { return mblegacyfill.WriteRecalcCSV(w, outs) }},
		{nm.path("revert-affected-products"), func(w io.Writer) error {
			ap, err := mblegacyfill.CollectAffectedProducts(pctx, impact, sr, rows, outs)
			if err != nil {
				return fmt.Errorf("collect affected products: %w", err)
			}
			return mblegacyfill.WriteAffectedProductsCSV(w, ap)
		}},
	}
	var errs []error
	for _, f := range files {
		if err := writeFile(f.path, f.fn); err != nil {
			errs = append(errs, err)
			continue
		}
		log.Printf("wrote %s", f.path)
	}
	return errors.Join(errs...)
}

func writeRejected(w io.Writer, p *mblegacyfill.Proposal) error {
	if _, err := fmt.Fprintln(w, "line,entity,id,field,usulan,reason"); err != nil {
		return err
	}
	for _, r := range p.Rejected {
		if _, err := fmt.Fprintf(w, "%d,%s,%s,%s,%s,%q\n", r.Row.Line, r.Row.Entity, r.Row.ID, r.Row.Field, r.Row.Usulan, r.Reason); err != nil {
			return err
		}
	}
	return nil
}
