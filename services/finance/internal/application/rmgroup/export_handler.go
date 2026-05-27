// Package rmgroup — V2 Excel export handler. Produces a 2-sheet workbook
// (Groups + Items) using the V2 schema:
//
//   - Decimal-stored percent fields (cost_percentage, anti-dumping) are
//     converted to whole-percent on export so the round-trip with the form UI
//     is consistent.
//   - V2 selection flags are emitted as string codes (AUTO / SL / FP / etc.)
//     instead of legacy integer enums.
//   - Three export modes share one entry point: All (no filter), Filtered
//     (active_filter and/or search), Selected (explicit group_head_ids).
package rmgroup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// ExportQuery selects which groups to export. All three filters are optional;
// when GroupHeadIDs is non-empty it overrides the others.
type ExportQuery struct {
	IsActive     *bool
	Search       string
	GroupHeadIDs []uuid.UUID
}

// ExportResult is the export bytes + filename.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler produces a 2-sheet Excel (Groups + Items) for selected RM groups.
type ExportHandler struct {
	repo rmgroup.Repository
}

// NewExportHandler builds an ExportHandler.
func NewExportHandler(repo rmgroup.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

// Sheet names + column orderings shared with import + template handlers.
const (
	sheetGroups = "Groups"
	sheetItems  = "Items"
	sheetNotes  = "Notes"
)

// groupsHeaders defines the V2 Groups sheet column order.
var groupsHeaders = []string{
	"group_code", "group_name", "description", "colourant", "ci_name",
	"duty_pct", "transport_rate",
	"mkt_freight", "mkt_anti_pct", "mkt_default_value",
	"valuation_flag", "marketing_flag", "is_active",
}

// itemsHeaders defines the V2 Items sheet column order.
var itemsHeaders = []string{
	"group_code", "item_code", "item_name", "item_type_code",
	"grade_code", "item_grade", "uom_code", "sort_order",
	"val_freight", "val_anti_pct", "val_duty_pct", "val_transport", "val_default_value",
	"is_active",
}

// Handle executes the export.
func (h *ExportHandler) Handle(ctx context.Context, q ExportQuery) (result *ExportResult, err error) {
	heads, err := h.fetchHeads(ctx, q)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Warn().Err(cerr).Msg("close excel")
			if err == nil {
				err = fmt.Errorf("close file: %w", cerr)
			}
		}
	}()

	if err := buildGroupsSheet(f, heads); err != nil {
		return nil, err
	}
	if err := buildItemsSheet(ctx, f, h.repo, heads); err != nil {
		return nil, err
	}
	if err := buildNotesSheet(f); err != nil {
		log.Debug().Err(err).Msg("notes sheet (non-critical)")
	}

	if delErr := f.DeleteSheet("Sheet1"); delErr != nil {
		log.Debug().Err(delErr).Msg("delete default sheet")
	}
	if idx, idxErr := f.GetSheetIndex(sheetGroups); idxErr == nil {
		f.SetActiveSheet(idx)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	return &ExportResult{FileContent: buf.Bytes(), FileName: "rm_groups_export.xlsx"}, nil
}

// fetchHeads honors the three modes: Selected (by ID list, ignores other
// filters), Filtered (active + search), or All.
func (h *ExportHandler) fetchHeads(ctx context.Context, q ExportQuery) ([]*rmgroup.Head, error) {
	if len(q.GroupHeadIDs) > 0 {
		return h.fetchHeadsByIDs(ctx, q.GroupHeadIDs)
	}
	heads, err := h.repo.ListAllHeads(ctx, q.IsActive)
	if err != nil {
		return nil, fmt.Errorf("list heads: %w", err)
	}
	if q.Search == "" {
		return heads, nil
	}
	return filterHeadsBySearch(heads, q.Search), nil
}

func (h *ExportHandler) fetchHeadsByIDs(ctx context.Context, ids []uuid.UUID) ([]*rmgroup.Head, error) {
	out := make([]*rmgroup.Head, 0, len(ids))
	for _, id := range ids {
		head, err := h.repo.GetHeadByID(ctx, id)
		if err != nil {
			if errors.Is(err, rmgroup.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("get head %s: %w", id, err)
		}
		out = append(out, head)
	}
	return out, nil
}

func filterHeadsBySearch(heads []*rmgroup.Head, search string) []*rmgroup.Head {
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return heads
	}
	out := make([]*rmgroup.Head, 0, len(heads))
	for _, h := range heads {
		hay := strings.ToLower(h.Code().String() + " " + h.Name() + " " + h.Description())
		if strings.Contains(hay, needle) {
			out = append(out, h)
		}
	}
	return out
}

// buildGroupsSheet writes the Groups sheet with V2 columns.
func buildGroupsSheet(f *excelize.File, heads []*rmgroup.Head) error {
	if _, err := f.NewSheet(sheetGroups); err != nil {
		return fmt.Errorf("new sheet %s: %w", sheetGroups, err)
	}
	if err := writeHeaderRow(f, sheetGroups, groupsHeaders); err != nil {
		return err
	}
	var errs []error
	for i, head := range heads {
		row := i + 2
		mi := head.MarketingInputs()
		vals := []any{
			head.Code().String(),
			head.Name(),
			head.Description(),
			head.Colorant(),
			head.CIName(),
			pctFromDecimal(head.CostPercentage()),
			head.CostPerKg(),
			optFloatToCell(mi.FreightRate),
			optFloatToPctCell(mi.AntiDumpingPct),
			optFloatToCell(mi.DefaultValue),
			mi.ValuationFlag.String(),
			mi.MarketingFlag.String(),
			head.IsActive(),
		}
		if err := writeRow(f, sheetGroups, row, vals); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		log.Warn().Err(errors.Join(errs...)).Msg("groups sheet partial errors")
	}
	return nil
}

// buildItemsSheet writes the Items sheet with V2 columns. Detail valuation
// pct fields are converted from decimal to whole-percent for round-trip with
// the per-item edit dialog.
func buildItemsSheet(
	ctx context.Context,
	f *excelize.File,
	repo rmgroup.Repository,
	heads []*rmgroup.Head,
) error {
	if _, err := f.NewSheet(sheetItems); err != nil {
		return fmt.Errorf("new sheet %s: %w", sheetItems, err)
	}
	if err := writeHeaderRow(f, sheetItems, itemsHeaders); err != nil {
		return err
	}
	rowIdx := 2
	var errs []error
	for _, head := range heads {
		details, err := repo.ListDetailsByHeadID(ctx, head.ID())
		if err != nil {
			return fmt.Errorf("list details for %s: %w", head.Code().String(), err)
		}
		for _, d := range details {
			if d.IsDeleted() {
				continue
			}
			vi := d.ValuationInputs()
			vals := []any{
				head.Code().String(),
				d.ItemCode().String(),
				d.ItemName(),
				d.ItemTypeCode(),
				d.GradeCode(),
				d.ItemGrade(),
				d.UOMCode(),
				d.SortOrder(),
				optFloatToCell(vi.FreightRate),
				optFloatToPctCell(vi.AntiDumpingPct),
				optFloatToPctCell(vi.DutyPct),
				optFloatToCell(vi.TransportRate),
				optFloatToCell(vi.DefaultValue),
				d.IsActive(),
			}
			if werr := writeRow(f, sheetItems, rowIdx, vals); werr != nil {
				errs = append(errs, werr)
			}
			rowIdx++
		}
	}
	if len(errs) > 0 {
		log.Warn().Err(errors.Join(errs...)).Msg("items sheet partial errors")
	}
	return nil
}

// buildNotesSheet emits a small reference page documenting the conventions
// the import handler enforces.
func buildNotesSheet(f *excelize.File) error {
	if _, err := f.NewSheet(sheetNotes); err != nil {
		return err
	}
	notes := [][2]string{
		{"A1", "RM Group Export / Import — Notes"},
		{"A3", "• duty_pct, mkt_anti_pct, val_anti_pct, val_duty_pct are WHOLE PERCENT (4 means 4%)."},
		{"A4", "• Other rate fields (freight, transport, default_value) are absolute decimal numbers."},
		{"A5", "• valuation_flag accepts: AUTO, CR, SR, PR, CL, SL, FL"},
		{"A6", "• marketing_flag accepts: AUTO, SP, PP, FP"},
		{"A7", "• is_active accepts: TRUE / FALSE (case-insensitive)."},
		{"A8", "• On import, group_code in the Items sheet must already exist in the Groups sheet OR in the database."},
		{"A9", "• Items already active in another group are reported as errors; same group/item is silently skipped."},
		{"A10", "• Empty cells = leave existing value unchanged when updating, or use defaults when creating."},
		{"A12", "Items sheet — item & grade resolution:"},
		{"A13", "• item_code is required; grade_code is required when an item has multiple variants in the sync feed."},
		{"A14", "  Backend will reject the row with the list of valid grade_code choices."},
		{"A15", "• When the item has a single variant, grade_code may be omitted — backend autofills from the sync feed."},
		{"A16", "• item_name, item_grade (grade name), and uom_code auto-fill from the sync feed when grade_code matches."},
		{"A17", "• item_type_code is a snapshot field — not used by the calculation engine, safe to leave empty."},
	}
	for _, kv := range notes {
		if err := f.SetCellValue(sheetNotes, kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// pctFromDecimal converts a decimal-stored percent (0.04) to whole percent (4).
func pctFromDecimal(v float64) float64 {
	return v * 100.0
}

// optFloatToCell renders a *float64 as either the value or empty string.
func optFloatToCell(v *float64) any {
	if v == nil {
		return ""
	}
	return *v
}

// optFloatToPctCell renders a decimal-stored percent *float64 as whole percent
// (4 instead of 0.04). Empty when nil.
func optFloatToPctCell(v *float64) any {
	if v == nil {
		return ""
	}
	return *v * 100.0
}

func writeHeaderRow(f *excelize.File, sheet string, headers []string) error {
	for col, h := range headers {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return fmt.Errorf("cell name: %w", err)
		}
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return fmt.Errorf("set header: %w", err)
		}
	}
	style, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
	})
	if err == nil {
		lastCol, colErr := excelize.CoordinatesToCellName(len(headers), 1)
		if colErr == nil {
			_ = f.SetCellStyle(sheet, "A1", lastCol, style) //nolint:errcheck // best-effort styling
		}
	}
	return nil
}

func writeRow(f *excelize.File, sheet string, row int, values []any) error {
	for col, v := range values {
		cell, err := excelize.CoordinatesToCellName(col+1, row)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(sheet, cell, v); err != nil {
			return err
		}
	}
	return nil
}
