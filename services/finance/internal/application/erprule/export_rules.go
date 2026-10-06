package erprule

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// Export sheet names.
const (
	SheetVallossRules = "valloss_rules"
	SheetSellPrices   = "sell_prices"
	SheetGradeGroups  = "grade_groups"
	SheetRuleSet      = "rule_set"
)

// ExportRulesQuery selects what the export includes.
type ExportRulesQuery struct {
	IncludeInactive bool
}

// ExportRulesResult is the xlsx workbook.
type ExportRulesResult struct {
	FileContent []byte
	FileName    string
	// RuleHash is the hash of the current active RuleSet (design §6.6),
	// also written to the rule_set sheet.
	RuleHash string
}

// ExportRulesHandler builds the ExportErpRules workbook: one sheet each for
// valloss rules, sell prices and grade groups, plus a rule_set sheet with the
// current active rule hash and counts. It uses excelize directly, like the
// other finance master exports.
type ExportRulesHandler struct {
	rules  domain.VallossRuleRepository
	prices domain.SellPriceRepository
	grades domain.GradeGroupRepository
	loader domain.RuleSetLoader
	now    func() time.Time
}

// NewExportRulesHandler builds the handler.
func NewExportRulesHandler(
	rules domain.VallossRuleRepository, prices domain.SellPriceRepository,
	grades domain.GradeGroupRepository, loader domain.RuleSetLoader,
) *ExportRulesHandler {
	return &ExportRulesHandler{rules: rules, prices: prices, grades: grades, loader: loader, now: time.Now}
}

// Handle reads everything and renders the workbook. Money values are written
// as fixed 6-dp text so Excel never turns them into floats.
func (h *ExportRulesHandler) Handle(ctx context.Context, q ExportRulesQuery) (result *ExportRulesResult, err error) {
	rules, _, err := h.rules.List(ctx, domain.VallossRuleFilter{IncludeInactive: q.IncludeInactive})
	if err != nil {
		return nil, fmt.Errorf("erp rule export: list rules: %w", err)
	}
	prices, err := h.prices.List(ctx, q.IncludeInactive)
	if err != nil {
		return nil, fmt.Errorf("erp rule export: list sell prices: %w", err)
	}
	grades, _, err := h.grades.ListGrades(ctx, domain.GradeFilter{})
	if err != nil {
		return nil, fmt.Errorf("erp rule export: list grades: %w", err)
	}
	rs, err := h.loader.LoadRuleSet(ctx)
	if err != nil {
		return nil, fmt.Errorf("erp rule export: load rule set: %w", err)
	}

	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Warn().Err(cerr).Msg("erp rule export: close workbook")
			if err == nil {
				result, err = nil, fmt.Errorf("erp rule export: close workbook: %w", cerr)
			}
		}
	}()

	w := &sheetWriter{f: f}
	w.table(SheetVallossRules,
		[]string{"No", "cevr_id", "cevr_fg_type", "cevr_prod_type", "cevr_grade_group", "cevr_basis", "cevr_val_loss", "cevr_is_active", "created_at", "created_by", "updated_at", "updated_by"},
		len(rules), func(i int) []any {
			r := rules[i]
			k := r.Key()
			return []any{i + 1, r.ID(), k.FgType.String(), k.ProdType.String(), k.GradeGroup.String(),
				r.Basis().String(), r.ValLoss().StringFixed(domain.Scale), r.IsActive(),
				formatTime(r.CreatedAt()), r.CreatedBy(), formatTimePtr(r.UpdatedAt()), r.UpdatedBy()}
		})
	w.table(SheetSellPrices,
		[]string{"No", "cesp_basis", "cesp_price", "cesp_is_active", "created_at", "created_by", "updated_at", "updated_by"},
		len(prices), func(i int) []any {
			p := prices[i]
			return []any{i + 1, p.Basis().String(), p.Price().StringFixed(domain.Scale), p.IsActive(),
				formatTime(p.CreatedAt()), p.CreatedBy(), formatTimePtr(p.UpdatedAt()), p.UpdatedBy()}
		})
	w.table(SheetGradeGroups,
		[]string{"No", "ceg_grade_code", "ceg_grade_name", "ceg_is_active", "ceg_grade_group"},
		len(grades), func(i int) []any {
			g := grades[i]
			group := ""
			if gg := g.Group(); gg != nil {
				group = gg.String()
			}
			return []any{i + 1, g.Code(), g.Name(), g.IsActive(), group}
		})
	generated := h.now().UTC()
	summary := [][]any{
		{"rule_hash", rs.Hash()},
		{"active_rules", rs.RuleCount()},
		{"active_prices", rs.PriceCount()},
		{"assigned_grades", rs.GradeCount()},
		{"generated_at", formatTime(generated)},
	}
	w.table(SheetRuleSet, []string{"key", "value"}, len(summary), func(i int) []any { return summary[i] })
	if w.err != nil {
		return nil, w.err
	}

	if idx, ierr := f.GetSheetIndex(SheetVallossRules); ierr == nil {
		f.SetActiveSheet(idx)
	}
	if derr := f.DeleteSheet("Sheet1"); derr != nil {
		log.Debug().Err(derr).Msg("erp rule export: delete default sheet")
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("erp rule export: write workbook: %w", err)
	}
	return &ExportRulesResult{
		FileContent: buf.Bytes(),
		FileName:    fmt.Sprintf("erp_rules_%s.xlsx", generated.Format("20060102_150405")),
		RuleHash:    rs.Hash(),
	}, nil
}

// sheetWriter writes header + rows sheets and keeps the first error.
type sheetWriter struct {
	f   *excelize.File
	err error
}

func (w *sheetWriter) table(sheet string, headers []string, n int, row func(i int) []any) {
	if w.err != nil {
		return
	}
	if _, err := w.f.NewSheet(sheet); err != nil {
		w.err = fmt.Errorf("erp rule export: new sheet %s: %w", sheet, err)
		return
	}
	if err := w.f.SetSheetRow(sheet, "A1", &headers); err != nil {
		w.err = fmt.Errorf("erp rule export: header %s: %w", sheet, err)
		return
	}
	for i := 0; i < n; i++ {
		cell, err := excelize.CoordinatesToCellName(1, i+2)
		if err != nil {
			w.err = fmt.Errorf("erp rule export: cell %s: %w", sheet, err)
			return
		}
		values := row(i)
		if err := w.f.SetSheetRow(sheet, cell, &values); err != nil {
			w.err = fmt.Errorf("erp rule export: row %s/%d: %w", sheet, i+2, err)
			return
		}
	}
	w.styleHeader(sheet, len(headers))
}

func (w *sheetWriter) styleHeader(sheet string, cols int) {
	style, err := w.f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		log.Debug().Err(err).Msg("erp rule export: header style")
		return
	}
	last, err := excelize.CoordinatesToCellName(cols, 1)
	if err != nil {
		return
	}
	if err := w.f.SetCellStyle(sheet, "A1", last, style); err != nil {
		log.Debug().Err(err).Str("sheet", sheet).Msg("erp rule export: set header style")
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}
