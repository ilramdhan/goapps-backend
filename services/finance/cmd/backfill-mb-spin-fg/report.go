package main

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// reportBatchSize caps the number of product ids per FG candidate / resolve / current-value call.
const reportBatchSize = 500

// Change kinds written to the report's change column.
const (
	changeNew    = "NEW"
	changeChange = "CHANGE"
)

// Overlay skip reasons (simulation only, logged).
const (
	overlaySkipMalformed   = "malformed_row"
	overlaySkipSpinMissing = "spin_not_found"
	overlaySkipNotLive     = "spin_not_live"
	overlaySkipFgSet       = "fg_already_set"
	overlaySkipPoyInvalid  = "poy_invalid"
	overlaySkipShade       = "shade_mismatch"
	overlaySkipDuplicate   = "duplicate_target"
)

// reportSource is the read-only data the report needs besides the P4 resolver ports.
type reportSource interface {
	ListReportProducts(ctx context.Context) ([]postgres.BackfillReportProduct, error)
	BackfillFgProductIDs(ctx context.Context) ([]int64, error)
	ActivePoyShades(ctx context.Context, ids []int64) (map[int64]string, error)
	CurrentSPCodes(ctx context.Context, ids []int64) (map[int64]postgres.BackfillCurrentSPCode, error)
}

// spinGetter loads one live (not deleted) spin; mbspin.ErrNotFound when absent.
type spinGetter interface {
	GetByID(ctx context.Context, id uuid.UUID) (*mbspin.Entity, error)
}

// reportDeps groups the read-only ports of the spin-move report.
type reportDeps struct {
	src    reportSource
	cands  mbsource.FgCandidateFinder
	spins  mbsource.SpinShadeFgFinder
	getter spinGetter
	params mbsource.FillGroupLister
}

// reportRow is one product whose MB_SP_CODE would change on a forced FG relink.
type reportRow struct {
	Product       postgres.BackfillReportProduct
	CurrentSPCode string
	CurrentSpinID *uuid.UUID
	NewSPCode     string
	NewSpinID     *uuid.UUID
	FgProductID   int64
	Change        string
}

// reportStats summarizes a report run.
type reportStats struct {
	Products   int
	Considered int
	New        int
	Change     int
	Overlay    int
	OverlaySkp map[string]int
}

// buildReport computes the spin-move report. Read-only: only SELECT-backed ports are used.
// entries are the parsed APPLY rows of --input (nil = current DB state only).
func buildReport(ctx context.Context, d reportDeps, entries []applyEntry, simulate bool) ([]reportRow, reportStats, error) {
	st := reportStats{OverlaySkp: map[string]int{}}
	fgIDs, err := d.src.BackfillFgProductIDs(ctx)
	if err != nil {
		return nil, st, fmt.Errorf("list backfill fg products: %w", err)
	}
	finder := d.spins
	if simulate {
		extra, poyIDs, skipped, oErr := buildOverlay(ctx, d, entries)
		if oErr != nil {
			return nil, st, oErr
		}
		fgIDs = append(fgIDs, poyIDs...)
		finder = &mbsource.OverlaySpinShadeFgFinder{Base: d.spins, Extra: extra}
		st.Overlay, st.OverlaySkp = len(extra), skipped
	}
	products, err := d.src.ListReportProducts(ctx)
	if err != nil {
		return nil, st, fmt.Errorf("list report products: %w", err)
	}
	st.Products = len(products)
	considered, memo, err := filterByFgSet(ctx, d.cands, products, toSet(fgIDs))
	if err != nil {
		return nil, st, err
	}
	st.Considered = len(considered)
	provider := mbsource.NewMBSpinFgProvider(memo, finder, d.params)
	var rows []reportRow
	for start := 0; start < len(considered); start += reportBatchSize {
		batch := considered[start:minInt(start+reportBatchSize, len(considered))]
		got, bErr := diffBatch(ctx, d.src, provider, batch)
		if bErr != nil {
			return nil, st, bErr
		}
		rows = append(rows, got...)
	}
	for _, r := range rows {
		if r.Change == changeNew {
			st.New++
		} else {
			st.Change++
		}
	}
	return rows, st, nil
}

// filterByFgSet keeps the products whose FG candidate list contains a POY of fgSet.
func filterByFgSet(ctx context.Context, cands mbsource.FgCandidateFinder, products []postgres.BackfillReportProduct, fgSet map[int64]bool) ([]postgres.BackfillReportProduct, memoCandidates, error) {
	memo := memoCandidates{}
	var out []postgres.BackfillReportProduct
	if len(fgSet) == 0 {
		return out, memo, nil
	}
	for start := 0; start < len(products); start += reportBatchSize {
		batch := products[start:minInt(start+reportBatchSize, len(products))]
		ids := make([]int64, len(batch))
		for i, p := range batch {
			ids[i] = p.ProductSysID
		}
		got, err := cands.FgCandidates(ctx, ids)
		if err != nil {
			return nil, nil, fmt.Errorf("find FG candidates: %w", err)
		}
		for _, p := range batch {
			if c := got[p.ProductSysID]; containsAny(c, fgSet) {
				memo[p.ProductSysID] = c
				out = append(out, p)
			}
		}
	}
	return out, memo, nil
}

// diffBatch resolves one batch through the FG provider and keeps the products whose
// resolution exists and differs from their current MB_SP_CODE.
func diffBatch(ctx context.Context, src reportSource, p *mbsource.MBSpinFgProvider, batch []postgres.BackfillReportProduct) ([]reportRow, error) {
	targets := make([]mbsource.Target, len(batch))
	ids := make([]int64, len(batch))
	for i, pr := range batch {
		targets[i] = mbsource.Target{ProductSysID: pr.ProductSysID, Shade: pr.ShadeCode}
		ids[i] = pr.ProductSysID
	}
	res, err := p.ResolveByTargets(ctx, targets)
	if err != nil {
		return nil, fmt.Errorf("resolve FG spins: %w", err)
	}
	cur, err := src.CurrentSPCodes(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load current MB_SP_CODE: %w", err)
	}
	var out []reportRow
	for _, pr := range batch {
		r, ok := res[pr.ProductSysID]
		if !ok {
			continue
		}
		if row, changed := diffProduct(pr, cur[pr.ProductSysID], r); changed {
			out = append(out, row)
		}
	}
	return out, nil
}

// diffProduct applies the diff rule: a pointer present must equal the new spin id;
// without a pointer the text must equal the new SPCode. No current value at all = NEW.
func diffProduct(p postgres.BackfillReportProduct, cur postgres.BackfillCurrentSPCode, r mbsource.Resolution) (reportRow, bool) {
	row := reportRow{
		Product: p, CurrentSPCode: cur.Text, CurrentSpinID: cur.SpinID,
		NewSPCode: r.SPCode, NewSpinID: r.SpinID, Change: changeChange,
	}
	if r.FgProductID != nil {
		row.FgProductID = *r.FgProductID
	}
	if cur.SpinID != nil {
		if r.SpinID != nil && *cur.SpinID == *r.SpinID {
			return row, false
		}
		return row, true
	}
	if cur.Text == r.SPCode {
		return row, false
	}
	if cur.Text == "" {
		row.Change = changeNew
	}
	return row, true
}

func toSet(ids []int64) map[int64]bool {
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func containsAny(ids []int64, set map[int64]bool) bool {
	for _, id := range ids {
		if set[id] {
			return true
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// logReportStats prints the report summary.
func logReportStats(st reportStats, out string, simulate bool) {
	mode := "current DB state"
	if simulate {
		mode = "simulated with --input APPLY rows"
	}
	log.Printf("report (%s, read-only): %d products checked, %d with an FG candidate in the FG set, NEW=%d CHANGE=%d -> %s",
		mode, st.Products, st.Considered, st.New, st.Change, out)
	if simulate {
		log.Printf("simulation: %d APPLY rows overlaid", st.Overlay)
		logCounts("simulation skipped APPLY rows", st.OverlaySkp)
	}
}
