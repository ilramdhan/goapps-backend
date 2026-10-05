package erpintegration

// link_readiness_export.go renders the P3-T9 link-readiness report as an
// .xlsx workbook: one sheet per section plus a summary. Quantities are
// written as fixed-scale text (never float).

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// Link-readiness workbook sheet names.
const (
	SheetReadinessSummary   = "Summary"
	SheetReadinessNoProduct = "9b-9d No Product"
	SheetReadinessAttrGaps  = "9c Attribute Gaps"
	SheetReadinessDups      = "9e Duplicates"
	SheetReadinessNoShade   = "Linked No Shade"
	SheetReadinessTypes     = "9f Type Mismatch"
)

// readinessQtyScale is the text scale of kg quantities (ced_qty_kg scale).
const readinessQtyScale = 7

// LinkReadinessExport is the rendered workbook.
type LinkReadinessExport struct {
	FileContent []byte
	FileName    string
	Report      *LinkReadinessReport
}

// Export builds the report and renders it as a workbook.
func (h *LinkReadinessHandler) Export(ctx context.Context, q LinkReadinessQuery) (*LinkReadinessExport, error) {
	rep, err := h.Handle(ctx, q)
	if err != nil {
		return nil, err
	}
	content, err := RenderLinkReadinessXLSX(rep)
	if err != nil {
		return nil, err
	}
	return &LinkReadinessExport{
		FileContent: content,
		FileName:    fmt.Sprintf("erp_link_readiness_%s_%s.xlsx", rep.Period, rep.GeneratedAt.Format("20060102_150405")),
		Report:      rep,
	}, nil
}

// RenderLinkReadinessXLSX renders a report as .xlsx bytes.
func RenderLinkReadinessXLSX(rep *LinkReadinessReport) (content []byte, err error) {
	if rep == nil {
		return nil, fmt.Errorf("link readiness export: %w", ErrInvalidLinkReadinessQuery)
	}
	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			content, err = nil, fmt.Errorf("link readiness export: close workbook: %w", cerr)
		}
	}()
	w := &readinessSheetWriter{f: f}
	writeReadinessSummary(w, rep)
	writeReadinessSections(w, rep)
	if w.err != nil {
		return nil, w.err
	}
	if idx, ierr := f.GetSheetIndex(SheetReadinessSummary); ierr == nil {
		f.SetActiveSheet(idx)
	}
	if derr := f.DeleteSheet("Sheet1"); derr != nil {
		log.Debug().Err(derr).Msg("link readiness export: delete default sheet")
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("link readiness export: write workbook: %w", err)
	}
	return buf.Bytes(), nil
}

func writeReadinessSummary(w *readinessSheetWriter, rep *LinkReadinessReport) {
	s := rep.Summary
	rows := [][]any{
		{"period", rep.Period},
		{"batch_id", rep.BatchID},
		{"demand_available", rep.DemandAvailable},
		{"generated_at", rep.GeneratedAt.UTC().Format("2006-01-02 15:04:05")},
		{"linked_products", s.LinkedProducts},
		{"replica_items", s.ReplicaItems},
		{"demand_combos", s.DemandCombos},
		{"demand_qty_kg", s.DemandQtyKg.StringFixed(readinessQtyScale)},
		{"mapped_combos", s.MappedCombos},
		{"s9b_no_product", s.NoProduct},
		{"s9b_no_product_with_qty", s.NoProductWithQty},
		{"s9d_link_candidates", s.LinkCandidates},
		{"s9d_create_new", s.CreateNew},
		{"s9c_gap_scope", s.GapScope},
		{"s9c_attribute_gap_products", s.AttributeGapProducts},
		{"s9c_blocking_gap_products", s.BlockingGapProducts},
		{"s9c_missing_fg_type", s.MissingFgType},
		{"s9c_not_in_replica", s.NotInReplica},
		{"s9e_duplicate_groups", s.DuplicateGroups},
		{"s9e_duplicate_products", s.DuplicateProducts},
		{"s9e_trial_groups", s.TrialDuplicateGroups},
		{"s9e_demand_groups", s.DemandDuplicates},
		{"linked_no_shade", s.LinkedNoShade},
		{"linked_no_shade_with_actual", s.LinkedNoShadeActual},
		{"s9f_type_mismatches", s.TypeMismatches},
		{"pass_s9b", s.Pass.NoProductWithQty},
		{"pass_s9c", s.Pass.AttributeGaps},
		{"pass_s9e", s.Pass.Duplicates},
		{"pass_s9f", s.Pass.TypeMismatches},
		{"ready", s.Ready},
		{"unique_index_ready", s.UniqueIndexReady},
	}
	w.table(SheetReadinessSummary, []string{"key", "value"}, len(rows), func(i int) []any { return rows[i] })
}

func writeReadinessSections(w *readinessSheetWriter, rep *LinkReadinessReport) {
	w.table(SheetReadinessNoProduct,
		[]string{"No", "kind", "item_code", "item_name", "shade_code", "grade_codes", "qty_kg", "suggestion", "candidates", "same_item_other_shade"},
		len(rep.NoProduct), func(i int) []any {
			l := rep.NoProduct[i]
			return []any{i + 1, string(l.Kind), l.ItemCode, l.ItemName, l.ShadeCode, strings.Join(l.GradeCodes, ","),
				l.QtyKg.StringFixed(readinessQtyScale), string(l.Suggestion), formatCandidates(l.Candidates), formatRefs(l.SameItemOtherShade)}
		})
	w.table(SheetReadinessAttrGaps,
		[]string{"No", "product_sys_id", "product_code", "item_code", "shade_code", "missing", "missing_fg_type", "not_in_replica", "blocking", "qty_kg"},
		len(rep.AttributeGaps), func(i int) []any {
			g := rep.AttributeGaps[i]
			missing := make([]string, len(g.Missing))
			for j, m := range g.Missing {
				missing[j] = string(m)
			}
			return []any{i + 1, g.SysID, g.ProductCode, g.ItemCode, g.ShadeCode, strings.Join(missing, ","),
				g.MissingFgType, g.NotInReplica, g.Blocking, g.QtyKg.StringFixed(readinessQtyScale)}
		})
	w.table(SheetReadinessDups,
		[]string{"No", "item_code", "shade_code", "product_count", "product_sys_ids", "product_codes", "trial", "in_demand", "demand_qty_kg"},
		len(rep.Duplicates), func(i int) []any {
			g := rep.Duplicates[i]
			ids := make([]string, len(g.Products))
			codes := make([]string, len(g.Products))
			for j, p := range g.Products {
				ids[j], codes[j] = strconv.FormatInt(p.SysID, 10), p.ProductCode
			}
			return []any{i + 1, g.ItemCode, g.ShadeCode, len(g.Products), strings.Join(ids, ","), strings.Join(codes, ","),
				g.Trial, g.InDemand, g.DemandQtyKg.StringFixed(readinessQtyScale)}
		})
	w.table(SheetReadinessNoShade,
		[]string{"No", "product_sys_id", "product_code", "item_code", "type_code", "actual_rows", "same_item_no_shade", "item_in_demand"},
		len(rep.LinkedNoShade), func(i int) []any {
			l := rep.LinkedNoShade[i]
			return []any{i + 1, l.SysID, l.ProductCode, l.ItemCode, l.TypeCode, l.ActualRows, l.SameItemNoShade, l.ItemInDemand}
		})
	w.table(SheetReadinessTypes,
		[]string{"No", "product_sys_id", "product_code", "item_code", "shade_code", "type_code", "direction", "in_demand"},
		len(rep.TypeMismatches), func(i int) []any {
			l := rep.TypeMismatches[i]
			return []any{i + 1, l.SysID, l.ProductCode, l.ItemCode, l.ShadeCode, l.TypeCode, string(l.Direction), l.InDemand}
		})
}

func formatCandidates(cs []LinkCandidate) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = fmt.Sprintf("%d:%s(%s,%d%%)", c.SysID, c.ProductCode, c.TypeCode, c.Score)
	}
	return strings.Join(parts, "; ")
}

func formatRefs(rs []ProductRef) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = fmt.Sprintf("%d:%s[%s]", r.SysID, r.ProductCode, r.ShadeCode)
	}
	return strings.Join(parts, "; ")
}

// readinessSheetWriter writes header + rows sheets and keeps the first error.
type readinessSheetWriter struct {
	f   *excelize.File
	err error
}

func (w *readinessSheetWriter) table(sheet string, headers []string, n int, row func(i int) []any) {
	if w.err != nil {
		return
	}
	if _, err := w.f.NewSheet(sheet); err != nil {
		w.err = fmt.Errorf("link readiness export: new sheet %s: %w", sheet, err)
		return
	}
	if err := w.f.SetSheetRow(sheet, "A1", &headers); err != nil {
		w.err = fmt.Errorf("link readiness export: header %s: %w", sheet, err)
		return
	}
	for i := 0; i < n; i++ {
		cell, err := excelize.CoordinatesToCellName(1, i+2)
		if err != nil {
			w.err = fmt.Errorf("link readiness export: cell %s: %w", sheet, err)
			return
		}
		values := row(i)
		if err := w.f.SetSheetRow(sheet, cell, &values); err != nil {
			w.err = fmt.Errorf("link readiness export: row %s/%d: %w", sheet, i+2, err)
			return
		}
	}
	style, err := w.f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
	})
	if err != nil {
		log.Debug().Err(err).Msg("link readiness export: header style")
		return
	}
	last, err := excelize.CoordinatesToCellName(len(headers), 1)
	if err != nil {
		return
	}
	if err := w.f.SetCellStyle(sheet, "A1", last, style); err != nil {
		log.Debug().Err(err).Str("sheet", sheet).Msg("link readiness export: set header style")
	}
}
