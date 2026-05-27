// Package rmgroup — V2 Excel import handler. Parses a 2-sheet workbook
// produced by ExportHandler / TemplateHandler and upserts heads + details.
//
// Schema (mirrors export_handler.go):
//   - Groups columns: group_code, group_name, description, colourant, ci_name,
//     duty_pct (whole %), transport_rate, mkt_freight, mkt_anti_pct (whole %),
//     mkt_default_value, valuation_flag, marketing_flag, is_active.
//   - Items columns: group_code, item_code, item_name, item_type_code,
//     grade_code, item_grade, uom_code, sort_order, val_freight,
//     val_anti_pct (whole %), val_duty_pct (whole %), val_transport,
//     val_default_value, is_active.
//
// Conventions:
//   - Whole-percent values are converted to decimal before persistence.
//   - duplicate_action: "skip" leaves the existing head untouched; "update"
//     applies the row's fields. Items already in the SAME group are silently
//     skipped; items already active in a DIFFERENT group are reported as errors.
//   - itemLookup is optional: when present, it backfills missing item_name /
//     grade / uom from the sync feed. When absent (or item not in feed), the
//     row's own values are used and missing cells default to empty strings.
package rmgroup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

// ImportCommand is the command for importing RM groups from Excel.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip" or "update"
	CreatedBy       string
}

// ImportError is a per-row error report.
type ImportError struct {
	RowNumber int32
	Field     string
	Message   string
}

// ImportResult summarizes the outcome.
type ImportResult struct {
	GroupsCreated int32
	GroupsUpdated int32
	GroupsSkipped int32
	ItemsAdded    int32
	ItemsSkipped  int32
	FailedCount   int32
	Errors        []ImportError
}

// ImportItemLookup is the optional sync-feed contract used during import:
//
//   - GetItemByCodeGrade returns the exact (item_code, grade_code) row for
//     metadata autofill when grade_code is known.
//   - ListItemsByCode returns every distinct grade variant for ambiguity
//     detection so multi-variant items can be rejected when the user
//     omitted grade_code.
//
// Any nil lookup disables backfill but still allows imports — callers must
// then provide all metadata in the workbook themselves.
type ImportItemLookup interface {
	GetItemByCodeGrade(ctx context.Context, itemCode, gradeCode string) (*syncdata.ItemConsStockPO, error)
	ListItemsByCode(ctx context.Context, itemCode string) ([]*syncdata.ItemConsStockPO, error)
}

// ImportHandler parses a 2-sheet Excel and upserts heads + details.
type ImportHandler struct {
	repo       rmgroup.Repository
	itemLookup ImportItemLookup
}

// NewImportHandler builds an ImportHandler. lookup may be nil.
func NewImportHandler(repo rmgroup.Repository, lookup ImportItemLookup) *ImportHandler {
	return &ImportHandler{repo: repo, itemLookup: lookup}
}

const (
	dupSkip   = "skip"
	dupUpdate = "update"
)

// Column indices (Groups sheet) — must match groupsHeaders in export_handler.go.
const (
	colGroupCode     = 0
	colGroupName     = 1
	colDescription   = 2
	colColourant     = 3
	colCIName        = 4
	colDutyPct       = 5
	colTransportRate = 6
	colMktFreight    = 7
	colMktAntiPct    = 8
	colMktDefaultVal = 9
	colValFlag       = 10
	colMktFlag       = 11
	colGroupActive   = 12
)

// Column indices (Items sheet) — must match itemsHeaders in export_handler.go.
const (
	colItemGroupCode  = 0
	colItemCode       = 1
	colItemName       = 2
	colItemTypeCode   = 3
	colItemGradeCode  = 4
	colItemGradeName  = 5
	colItemUOMCode    = 6
	colItemSortOrder  = 7
	colItemValFreight = 8
	colItemValAntiPct = 9
	colItemValDutyPct = 10
	colItemValTrans   = 11
	colItemValDefault = 12
	colItemActive     = 13
)

// Handle executes the import.
func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (*ImportResult, error) {
	f, err := excelize.OpenReader(bytes.NewReader(cmd.FileContent))
	if err != nil {
		return nil, fmt.Errorf("open excel: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			_ = closeErr // best-effort close
		}
	}()

	result := &ImportResult{}

	sheets := f.GetSheetList()
	hasGroups := containsSheet(sheets, sheetGroups)
	hasItems := containsSheet(sheets, sheetItems)
	if !hasGroups && !hasItems {
		return nil, fmt.Errorf("expected at least one of sheets %q or %q", sheetGroups, sheetItems)
	}

	if hasGroups {
		if err := h.importGroupsSheet(ctx, f, cmd, result); err != nil {
			return nil, err
		}
	}
	if hasItems {
		if err := h.importItemsSheet(ctx, f, cmd, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// =============================================================================
// Groups sheet
// =============================================================================

func (h *ImportHandler) importGroupsSheet(
	ctx context.Context,
	f *excelize.File,
	cmd ImportCommand,
	result *ImportResult,
) error {
	rows, err := f.GetRows(sheetGroups)
	if err != nil {
		return fmt.Errorf("read %s: %w", sheetGroups, err)
	}
	if len(rows) < 2 {
		return nil
	}
	for i, row := range rows[1:] {
		rowNum := int32(i + 2) //nolint:gosec // bounded by Excel row count
		if isBlankRow(row) {
			continue
		}
		if err := h.importGroupRow(ctx, row, cmd, result); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "Groups",
				Message:   err.Error(),
			})
		}
	}
	return nil
}

func (h *ImportHandler) importGroupRow(
	ctx context.Context,
	row []string,
	cmd ImportCommand,
	result *ImportResult,
) error {
	groupCode := colStr(row, colGroupCode)
	if groupCode == "" {
		return errors.New("group_code is required")
	}
	code, err := rmgroup.NewCode(groupCode)
	if err != nil {
		return fmt.Errorf("group_code: %w", err)
	}
	existing, err := h.repo.GetHeadByCode(ctx, code)
	if err != nil && !errors.Is(err, rmgroup.ErrNotFound) {
		return fmt.Errorf("lookup head: %w", err)
	}
	if existing != nil {
		return h.updateOrSkipHead(ctx, existing, row, cmd, result)
	}
	return h.createHead(ctx, code, row, cmd.CreatedBy, result)
}

func (h *ImportHandler) updateOrSkipHead(
	ctx context.Context,
	existing *rmgroup.Head,
	row []string,
	cmd ImportCommand,
	result *ImportResult,
) error {
	if cmd.DuplicateAction == dupSkip {
		result.GroupsSkipped++
		return nil
	}
	in, perr := parseGroupUpdateInput(row)
	if perr != nil {
		return perr
	}
	if err := existing.Update(in, cmd.CreatedBy); err != nil {
		return fmt.Errorf("update head: %w", err)
	}
	if err := applyGroupV2Marketing(existing, row, cmd.CreatedBy); err != nil {
		return err
	}
	if err := h.repo.UpdateHead(ctx, existing); err != nil {
		return fmt.Errorf("persist update: %w", err)
	}
	result.GroupsUpdated++
	return nil
}

func (h *ImportHandler) createHead(
	ctx context.Context,
	code rmgroup.Code,
	row []string,
	createdBy string,
	result *ImportResult,
) error {
	name := colStr(row, colGroupName)
	description := colStr(row, colDescription)
	dutyPct, err := colPctOrZero(row, colDutyPct) // converts "4" -> 0.04
	if err != nil {
		return fmt.Errorf("duty_pct: %w", err)
	}
	transportRate, err := colFloat(row, colTransportRate)
	if err != nil {
		return fmt.Errorf("transport_rate: %w", err)
	}
	head, err := rmgroup.NewHead(code, name, description, dutyPct, transportRate, createdBy)
	if err != nil {
		return fmt.Errorf("new head: %w", err)
	}
	in, perr := parseGroupUpdateInput(row)
	if perr != nil {
		return perr
	}
	// Already set via constructor — clear so Update doesn't re-validate.
	in.Name = nil
	in.CostPercentage = nil
	in.CostPerKg = nil
	if err := head.Update(in, createdBy); err != nil {
		return fmt.Errorf("apply optional fields: %w", err)
	}
	if err := applyGroupV2Marketing(head, row, createdBy); err != nil {
		return err
	}
	if err := h.repo.CreateHead(ctx, head); err != nil {
		return fmt.Errorf("create head: %w", err)
	}
	result.GroupsCreated++
	return nil
}

// parseGroupUpdateInput maps the row's V1-equivalent fields onto an UpdateInput.
// V2 marketing fields are applied separately via applyGroupV2Marketing.
func parseGroupUpdateInput(row []string) (rmgroup.UpdateInput, error) {
	in := rmgroup.UpdateInput{}
	if v := colStr(row, colGroupName); v != "" {
		in.Name = &v
	}
	if v := colStr(row, colDescription); v != "" {
		in.Description = &v
	}
	if v := colStr(row, colColourant); v != "" {
		in.Colorant = &v
	}
	if v := colStr(row, colCIName); v != "" {
		in.CIName = &v
	}
	if v, ok, err := colOptPct(row, colDutyPct); err != nil {
		return in, fmt.Errorf("duty_pct: %w", err)
	} else if ok {
		in.CostPercentage = &v
	}
	if v, ok, err := colOptFloat(row, colTransportRate); err != nil {
		return in, fmt.Errorf("transport_rate: %w", err)
	} else if ok {
		in.CostPerKg = &v
	}
	if v, ok := colOptBool(row, colGroupActive); ok {
		in.IsActive = &v
	}
	return in, nil
}

// applyGroupV2Marketing pulls V2 marketing inputs from the row and re-attaches
// them to the head. Empty cells preserve the head's current values.
func applyGroupV2Marketing(head *rmgroup.Head, row []string, _ string) error {
	mi := head.MarketingInputs()
	if v, ok, err := colOptFloat(row, colMktFreight); err != nil {
		return fmt.Errorf("mkt_freight: %w", err)
	} else if ok {
		mi.FreightRate = &v
	}
	if v, ok, err := colOptPct(row, colMktAntiPct); err != nil {
		return fmt.Errorf("mkt_anti_pct: %w", err)
	} else if ok {
		mi.AntiDumpingPct = &v
	}
	if v, ok, err := colOptFloat(row, colMktDefaultVal); err != nil {
		return fmt.Errorf("mkt_default_value: %w", err)
	} else if ok {
		mi.DefaultValue = &v
	}
	if v := colStr(row, colValFlag); v != "" {
		vf, err := rmgroup.ParseValuationFlag(v)
		if err != nil {
			return fmt.Errorf("valuation_flag: %w", err)
		}
		mi.ValuationFlag = vf
	}
	if v := colStr(row, colMktFlag); v != "" {
		mf, err := rmgroup.ParseMarketingFlag(v)
		if err != nil {
			return fmt.Errorf("marketing_flag: %w", err)
		}
		mi.MarketingFlag = mf
	}
	return head.AttachMarketingInputs(mi)
}

// =============================================================================
// Items sheet
// =============================================================================

func (h *ImportHandler) importItemsSheet(
	ctx context.Context,
	f *excelize.File,
	cmd ImportCommand,
	result *ImportResult,
) error {
	rows, err := f.GetRows(sheetItems)
	if err != nil {
		return fmt.Errorf("read %s: %w", sheetItems, err)
	}
	if len(rows) < 2 {
		return nil
	}
	for i, row := range rows[1:] {
		rowNum := int32(i + 2) //nolint:gosec // bounded
		if isBlankRow(row) {
			continue
		}
		if err := h.importItemRow(ctx, row, cmd, result); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "Items",
				Message:   err.Error(),
			})
		}
	}
	return nil
}

func (h *ImportHandler) importItemRow(
	ctx context.Context,
	row []string,
	cmd ImportCommand,
	result *ImportResult,
) error {
	groupCode := colStr(row, colItemGroupCode)
	itemCodeStr := colStr(row, colItemCode)
	if groupCode == "" || itemCodeStr == "" {
		return errors.New("group_code and item_code are required")
	}
	head, err := h.lookupGroup(ctx, groupCode)
	if err != nil {
		return err
	}
	itemCode, err := rmgroup.NewItemCode(itemCodeStr)
	if err != nil {
		return fmt.Errorf("item_code: %w", err)
	}
	// Resolve grade up front so subsequent steps share one canonical key.
	// When the user omits grade_code we either pick the single variant from
	// the sync feed or refuse the row to prevent ghost rows whose engine
	// lookup will silently return zero quantities.
	gradeKey, err := h.resolveGradeCode(ctx, itemCodeStr, colStr(row, colItemGradeCode))
	if err != nil {
		return err
	}
	skipped, err := h.skipIfAlreadyAssigned(ctx, head, itemCode, gradeKey, result)
	if err != nil {
		return err
	}
	if skipped {
		return nil
	}
	detail, err := h.buildDetail(ctx, head, itemCode, gradeKey, row, cmd.CreatedBy)
	if err != nil {
		return err
	}
	if err := h.repo.AddDetail(ctx, detail); err != nil {
		return fmt.Errorf("persist detail: %w", err)
	}
	result.ItemsAdded++
	return nil
}

// resolveGradeCode returns the canonical grade_code for the row.
//   - Explicit grade_code in the workbook always wins.
//   - When omitted and lookup is unavailable, returns empty string and lets
//     downstream steps proceed (caller is responsible for providing data).
//   - When omitted with lookup available, lists variants from the sync feed:
//     0 variants → keep empty (item not synced yet, allow as-is);
//     1 variant  → autofill that single grade_code;
//     >1 variants → reject the row with a helpful error listing the choices.
func (h *ImportHandler) resolveGradeCode(ctx context.Context, itemCode, providedGrade string) (string, error) {
	if providedGrade != "" {
		return providedGrade, nil
	}
	if h.itemLookup == nil {
		return "", nil
	}
	variants, err := h.itemLookup.ListItemsByCode(ctx, itemCode)
	if err != nil {
		return "", fmt.Errorf("lookup variants for %s: %w", itemCode, err)
	}
	switch len(variants) {
	case 0:
		return "", nil
	case 1:
		return variants[0].GradeCode, nil
	default:
		grades := make([]string, 0, len(variants))
		for _, v := range variants {
			if v.GradeCode == "" {
				grades = append(grades, "(empty)")
			} else {
				grades = append(grades, v.GradeCode)
			}
		}
		return "", fmt.Errorf(
			"item_code %q has %d grade variants — please specify grade_code (one of: %s)",
			itemCode, len(variants), strings.Join(grades, ", "),
		)
	}
}

func (h *ImportHandler) lookupGroup(ctx context.Context, groupCode string) (*rmgroup.Head, error) {
	code, err := rmgroup.NewCode(groupCode)
	if err != nil {
		return nil, fmt.Errorf("group_code: %w", err)
	}
	head, err := h.repo.GetHeadByCode(ctx, code)
	if err != nil {
		if errors.Is(err, rmgroup.ErrNotFound) {
			return nil, fmt.Errorf("group_code %q not found", groupCode)
		}
		return nil, fmt.Errorf("lookup group: %w", err)
	}
	return head, nil
}

// skipIfAlreadyAssigned checks the natural-key constraint and returns true
// when the row should be silently skipped (item already in the same group).
// Returns an error when the item is active in a different group.
func (h *ImportHandler) skipIfAlreadyAssigned(
	ctx context.Context,
	head *rmgroup.Head,
	itemCode rmgroup.ItemCode,
	gradeKey string,
	result *ImportResult,
) (bool, error) {
	existingDetail, err := h.repo.GetActiveDetailByItemCodeGrade(ctx, itemCode, gradeKey)
	if err != nil && !errors.Is(err, rmgroup.ErrDetailNotFound) {
		return false, fmt.Errorf("lookup active detail: %w", err)
	}
	if existingDetail == nil {
		return false, nil
	}
	if existingDetail.HeadID() == head.ID() {
		result.ItemsSkipped++
		return true, nil
	}
	return false, fmt.Errorf("item_code %q (grade %q) is already active in another group", itemCode.String(), gradeKey)
}

// buildDetail assembles a new Detail from the row, optionally enriched with
// sync-feed metadata keyed on the resolved (item_code, grade_code) so the
// autofill picks the exact variant the operator selected. The row's explicit
// values always win over feed values.
func (h *ImportHandler) buildDetail(
	ctx context.Context,
	head *rmgroup.Head,
	itemCode rmgroup.ItemCode,
	gradeKey string,
	row []string,
	createdBy string,
) (*rmgroup.Detail, error) {
	detail, err := rmgroup.NewDetail(head.ID(), itemCode, createdBy)
	if err != nil {
		return nil, fmt.Errorf("new detail: %w", err)
	}
	in := h.buildDetailMetadata(ctx, itemCode.String(), gradeKey, row)
	if err := detail.Update(in, createdBy); err != nil {
		return nil, fmt.Errorf("apply detail fields: %w", err)
	}
	if err := applyItemRowValuationInputs(detail, row); err != nil {
		return nil, err
	}
	return detail, nil
}

// buildDetailMetadata seeds detail metadata from the optional sync feed,
// keyed on the resolved (item_code, grade_code) variant, and then overrides
// with any non-empty cells in the row.
func (h *ImportHandler) buildDetailMetadata(ctx context.Context, itemCode, gradeKey string, row []string) rmgroup.DetailUpdateInput {
	in := rmgroup.DetailUpdateInput{}
	if h.itemLookup != nil {
		if synced, err := h.itemLookup.GetItemByCodeGrade(ctx, itemCode, gradeKey); err == nil && synced != nil {
			in.ItemName = stringPtrIfNotEmpty(synced.ItemName)
			in.GradeCode = stringPtrIfNotEmpty(synced.GradeCode)
			in.ItemGrade = stringPtrIfNotEmpty(synced.GradeName)
			in.UOMCode = stringPtrIfNotEmpty(synced.UOM)
		}
	}
	// Fallback: ensure grade_code is set from the resolved key when the sync
	// feed didn't have a row to autofill from. This is the canonical key
	// downstream calculations will use, so persisting it on the detail keeps
	// the import → calc round-trip lossless.
	if in.GradeCode == nil && gradeKey != "" {
		in.GradeCode = stringPtrIfNotEmpty(gradeKey)
	}
	overlayItemRowOverrides(row, &in)
	return in
}

// overlayItemRowOverrides applies the row's V2 metadata + sort_order + active
// onto the input, overwriting any sync-feed seeds.
func overlayItemRowOverrides(row []string, in *rmgroup.DetailUpdateInput) {
	if v := colStr(row, colItemName); v != "" {
		in.ItemName = &v
	}
	if v := colStr(row, colItemTypeCode); v != "" {
		in.ItemTypeCode = &v
	}
	if v := colStr(row, colItemGradeCode); v != "" {
		in.GradeCode = &v
	}
	if v := colStr(row, colItemGradeName); v != "" {
		in.ItemGrade = &v
	}
	if v := colStr(row, colItemUOMCode); v != "" {
		in.UOMCode = &v
	}
	if s := colStr(row, colItemSortOrder); s != "" {
		if n, perr := strconv.ParseInt(s, 10, 32); perr == nil {
			n32 := int32(n) //nolint:gosec // ParseInt bitsize 32
			in.SortOrder = &n32
		}
	}
	if v, ok := colOptBool(row, colItemActive); ok {
		in.IsActive = &v
	}
}

// applyItemRowValuationInputs applies the V2 valuation inputs on the detail
// (freight, anti %, duty %, transport, default value). Pct columns are
// stored as decimal after dividing by 100.
func applyItemRowValuationInputs(detail *rmgroup.Detail, row []string) error {
	vi := detail.ValuationInputs()
	if v, ok, err := colOptFloat(row, colItemValFreight); err != nil {
		return fmt.Errorf("val_freight: %w", err)
	} else if ok {
		vi.FreightRate = &v
	}
	if v, ok, err := colOptPct(row, colItemValAntiPct); err != nil {
		return fmt.Errorf("val_anti_pct: %w", err)
	} else if ok {
		vi.AntiDumpingPct = &v
	}
	if v, ok, err := colOptPct(row, colItemValDutyPct); err != nil {
		return fmt.Errorf("val_duty_pct: %w", err)
	} else if ok {
		vi.DutyPct = &v
	}
	if v, ok, err := colOptFloat(row, colItemValTrans); err != nil {
		return fmt.Errorf("val_transport: %w", err)
	} else if ok {
		vi.TransportRate = &v
	}
	if v, ok, err := colOptFloat(row, colItemValDefault); err != nil {
		return fmt.Errorf("val_default_value: %w", err)
	} else if ok {
		vi.DefaultValue = &v
	}
	return detail.AttachValuationInputs(vi)
}

// =============================================================================
// Cell helpers
// =============================================================================

func containsSheet(sheets []string, name string) bool {
	for _, s := range sheets {
		if s == name {
			return true
		}
	}
	return false
}

func isBlankRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func colStr(row []string, idx int) string {
	if idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func colFloat(row []string, idx int) (float64, error) {
	s := colStr(row, idx)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}

func colOptFloat(row []string, idx int) (float64, bool, error) {
	s := colStr(row, idx)
	if s == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, err
	}
	return v, true, nil
}

// colPctOrZero parses a whole-percent cell ("4") and returns the decimal
// equivalent (0.04). Empty cell returns 0.
func colPctOrZero(row []string, idx int) (float64, error) {
	v, err := colFloat(row, idx)
	if err != nil {
		return 0, err
	}
	return v / 100.0, nil
}

// colOptPct parses an optional whole-percent cell. ok=false when empty.
// The returned value is the decimal equivalent (e.g. "4" -> 0.04).
func colOptPct(row []string, idx int) (float64, bool, error) {
	v, ok, err := colOptFloat(row, idx)
	if err != nil || !ok {
		return 0, ok, err
	}
	return v / 100.0, true, nil
}

func colOptBool(row []string, idx int) (bool, bool) {
	s := strings.ToLower(colStr(row, idx))
	switch s {
	case "true", "1", "yes", "y":
		return true, true
	case "false", "0", "no", "n":
		return false, true
	default:
		return false, false
	}
}

func stringPtrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
