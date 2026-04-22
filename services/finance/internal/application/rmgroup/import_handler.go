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

// ImportItemLookup is the minimum contract needed to resolve item metadata
// during import. Implementations are provided by the sync repository.
type ImportItemLookup interface {
	GetItemByCode(ctx context.Context, itemCode string) (*syncdata.ItemConsStockPO, error)
}

// ImportHandler parses a 2-sheet Excel and upserts heads + details.
type ImportHandler struct {
	repo       rmgroup.Repository
	itemLookup ImportItemLookup
}

// NewImportHandler builds an ImportHandler.
func NewImportHandler(repo rmgroup.Repository, lookup ImportItemLookup) *ImportHandler {
	return &ImportHandler{repo: repo, itemLookup: lookup}
}

const (
	dupSkip   = "skip"
	dupUpdate = "update"
)

// Handle executes the import.
func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (*ImportResult, error) {
	f, err := excelize.OpenReader(bytes.NewReader(cmd.FileContent))
	if err != nil {
		return nil, fmt.Errorf("open excel: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			_ = closeErr // best-effort
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
		rowNum := int32(i + 2) //nolint:gosec // i is bounded by Excel row count
		if isBlankRow(row) {
			continue
		}
		if err := h.importGroupRow(ctx, row, rowNum, cmd, result); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "",
				Message:   err.Error(),
			})
		}
	}
	return nil
}

func (h *ImportHandler) importGroupRow(
	ctx context.Context,
	row []string,
	rowNum int32,
	cmd ImportCommand,
	result *ImportResult,
) error {
	groupCode := colStr(row, 0)
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

	return h.createHead(ctx, code, row, cmd.CreatedBy, rowNum, result)
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
	_ int32,
	result *ImportResult,
) error {
	name := colStr(row, 1)
	description := colStr(row, 2)
	costPct, err := colFloat(row, 5)
	if err != nil {
		return fmt.Errorf("cost_percentage: %w", err)
	}
	costPerKg, err := colFloat(row, 6)
	if err != nil {
		return fmt.Errorf("cost_per_kg: %w", err)
	}

	head, err := rmgroup.NewHead(code, name, description, costPct, costPerKg, createdBy)
	if err != nil {
		return fmt.Errorf("new head: %w", err)
	}

	// Apply optional fields via Update (colorant, flags, inits, is_active).
	in, perr := parseGroupUpdateInput(row)
	if perr != nil {
		return perr
	}
	// Name already set, unset to avoid re-validation.
	in.Name = nil
	in.CostPercentage = nil
	in.CostPerKg = nil
	if err := head.Update(in, createdBy); err != nil {
		return fmt.Errorf("apply optional fields: %w", err)
	}

	if err := h.repo.CreateHead(ctx, head); err != nil {
		return fmt.Errorf("create head: %w", err)
	}
	result.GroupsCreated++
	return nil
}

func parseGroupUpdateInput(row []string) (rmgroup.UpdateInput, error) {
	in := rmgroup.UpdateInput{}

	if v := colStr(row, 1); v != "" {
		in.Name = &v
	}
	if v := colStr(row, 2); v != "" {
		in.Description = &v
	}
	if v := colStr(row, 3); v != "" {
		in.Colorant = &v
	}
	if v := colStr(row, 4); v != "" {
		in.CIName = &v
	}
	if v, ok, err := colOptFloat(row, 5); err != nil {
		return in, fmt.Errorf("cost_percentage: %w", err)
	} else if ok {
		in.CostPercentage = &v
	}
	if v, ok, err := colOptFloat(row, 6); err != nil {
		return in, fmt.Errorf("cost_per_kg: %w", err)
	} else if ok {
		in.CostPerKg = &v
	}

	if err := parseFlagFields(row, &in); err != nil {
		return in, err
	}
	if err := parseInitFields(row, &in); err != nil {
		return in, err
	}

	if v, ok := colOptBool(row, 13); ok {
		in.IsActive = &v
	}
	return in, nil
}

func parseFlagFields(row []string, in *rmgroup.UpdateInput) error {
	if v := colStr(row, 7); v != "" {
		f, err := rmgroup.ParseFlag(v)
		if err != nil {
			return fmt.Errorf("flag_valuation: %w", err)
		}
		in.FlagValuation = &f
	}
	if v := colStr(row, 8); v != "" {
		f, err := rmgroup.ParseFlag(v)
		if err != nil {
			return fmt.Errorf("flag_marketing: %w", err)
		}
		in.FlagMarketing = &f
	}
	if v := colStr(row, 9); v != "" {
		f, err := rmgroup.ParseFlag(v)
		if err != nil {
			return fmt.Errorf("flag_simulation: %w", err)
		}
		in.FlagSimulation = &f
	}
	return nil
}

func parseInitFields(row []string, in *rmgroup.UpdateInput) error {
	if v, ok, err := colOptFloat(row, 10); err != nil {
		return fmt.Errorf("init_val_valuation: %w", err)
	} else if ok {
		in.InitValValuation = &v
	}
	if v, ok, err := colOptFloat(row, 11); err != nil {
		return fmt.Errorf("init_val_marketing: %w", err)
	} else if ok {
		in.InitValMarketing = &v
	}
	if v, ok, err := colOptFloat(row, 12); err != nil {
		return fmt.Errorf("init_val_simulation: %w", err)
	} else if ok {
		in.InitValSimulation = &v
	}
	return nil
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
		rowNum := int32(i + 2) //nolint:gosec // i is bounded by Excel row count
		if isBlankRow(row) {
			continue
		}
		if err := h.importItemRow(ctx, row, cmd, result); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "",
				Message:   err.Error(),
			})
		}
	}
	return nil
}

func (h *ImportHandler) importItemRow( //nolint:gocyclo,gocognit // sequential validation steps
	ctx context.Context,
	row []string,
	cmd ImportCommand,
	result *ImportResult,
) error {
	groupCode := colStr(row, 0)
	itemCodeStr := colStr(row, 1)
	if groupCode == "" || itemCodeStr == "" {
		return errors.New("group_code and item_code are required")
	}

	code, err := rmgroup.NewCode(groupCode)
	if err != nil {
		return fmt.Errorf("group_code: %w", err)
	}
	head, err := h.repo.GetHeadByCode(ctx, code)
	if err != nil {
		if errors.Is(err, rmgroup.ErrNotFound) {
			return fmt.Errorf("group_code %q not found", groupCode)
		}
		return fmt.Errorf("lookup group: %w", err)
	}

	itemCode, err := rmgroup.NewItemCode(itemCodeStr)
	if err != nil {
		return fmt.Errorf("item_code: %w", err)
	}

	// Skip if already active in the same group for the same (item, grade)
	// variant. Use the grade from the row if provided, otherwise the synced
	// grade. Items with different grade_codes are treated as independent
	// variants per migration 000018.
	gradeKey := colStr(row, 2)
	if gradeKey == "" {
		if synced, lookupErr := h.itemLookup.GetItemByCode(ctx, itemCodeStr); lookupErr == nil && synced != nil {
			gradeKey = synced.GradeCode
		}
	}
	existingDetail, err := h.repo.GetActiveDetailByItemCodeGrade(ctx, itemCode, gradeKey)
	if err != nil && !errors.Is(err, rmgroup.ErrDetailNotFound) {
		return fmt.Errorf("lookup active detail: %w", err)
	}
	if existingDetail != nil {
		if existingDetail.HeadID() == head.ID() {
			result.ItemsSkipped++
			return nil
		}
		return fmt.Errorf("item_code %q is already active in another group", itemCodeStr)
	}

	// Look up metadata from sync feed. Missing items are rejected.
	synced, err := h.itemLookup.GetItemByCode(ctx, itemCodeStr)
	if err != nil {
		return fmt.Errorf("lookup sync item: %w", err)
	}
	if synced == nil {
		return fmt.Errorf("item_code %q not present in sync feed", itemCodeStr)
	}

	detail, err := rmgroup.NewDetail(head.ID(), itemCode, cmd.CreatedBy)
	if err != nil {
		return fmt.Errorf("new detail: %w", err)
	}

	// Apply sync metadata (name/grade/uom) via Update.
	detailIn := rmgroup.DetailUpdateInput{
		ItemName:  stringPtrIfNotEmpty(synced.ItemName),
		GradeCode: stringPtrIfNotEmpty(synced.GradeCode),
		ItemGrade: stringPtrIfNotEmpty(synced.GradeName),
		UOMCode:   stringPtrIfNotEmpty(synced.UOM),
	}
	applyItemRowOverrides(row, &detailIn)

	if err := detail.Update(detailIn, cmd.CreatedBy); err != nil {
		return fmt.Errorf("apply detail fields: %w", err)
	}

	if err := h.repo.AddDetail(ctx, detail); err != nil {
		return fmt.Errorf("persist detail: %w", err)
	}
	result.ItemsAdded++
	return nil
}

func applyItemRowOverrides(row []string, in *rmgroup.DetailUpdateInput) {
	if v := colStr(row, 2); v != "" {
		in.GradeCode = &v
	}
	if s := colStr(row, 3); s != "" {
		if n, perr := strconv.ParseInt(s, 10, 32); perr == nil {
			n32 := int32(n) //nolint:gosec // ParseInt bitsize 32
			in.SortOrder = &n32
		}
	}
	if v, ok := colOptBool(row, 4); ok {
		in.IsActive = &v
	}
	if v, ok := colOptBool(row, 5); ok {
		in.IsDummy = &v
	}
}

// =============================================================================
// Helpers
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
