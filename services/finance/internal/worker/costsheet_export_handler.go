package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/xuri/excelize/v2"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	appcostcalc "github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// costSheetSourceType is the notification source_type for product cost sheet exports.
const costSheetSourceType = "finance.product_cost_sheet_export"

// xlsxContentType is the MIME type stored on the uploaded workbook.
const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// fallbackSheetName is used when a product's finished-good stage has no item code.
const fallbackSheetName = "Product"

// RouteCostSheetProvider resolves the full route cost sheet of a single product.
// Implemented by application/costcalc.GetRouteCostSheetHandler; declared as an
// interface here so the worker is testable without the full calc service graph.
type RouteCostSheetProvider interface {
	Handle(ctx context.Context, q appcostcalc.GetRouteCostSheetQuery) ([]appcostcalc.RouteCostSheetStage, error)
}

// CostSheetExportHandler renders one product cost sheet per requested product
// into a single xlsx workbook, uploads it to MinIO, emits a notification, and
// updates the job_execution row.
type CostSheetExportHandler struct {
	jobRepo  job.Repository
	sheets   RouteCostSheetProvider
	storage  storage.Service
	notif    iamclient.NotificationClient
	logger   zerolog.Logger
	calcType costcalcdom.CalculationType
}

// NewCostSheetExportHandler constructs the handler. defaultCalcType is used when
// the job params carry no calculation_type; pass "" for ACTUAL.
func NewCostSheetExportHandler(
	jobRepo job.Repository,
	sheets RouteCostSheetProvider,
	storageSvc storage.Service,
	notif iamclient.NotificationClient,
	logger zerolog.Logger,
	defaultCalcType costcalcdom.CalculationType,
) *CostSheetExportHandler {
	if defaultCalcType == "" {
		defaultCalcType = costcalcdom.CalcTypeActual
	}
	return &CostSheetExportHandler{
		jobRepo:  jobRepo,
		sheets:   sheets,
		storage:  storageSvc,
		notif:    notif,
		logger:   logger,
		calcType: defaultCalcType,
	}
}

// costSheetResult summarizes a successful export run. Persisted as the job
// result_summary and partially embedded into the EXPORT_READY action_payload.
type costSheetResult struct {
	FilePath     string   `json:"file_path"`
	FileName     string   `json:"file_name"`
	SizeBytes    int      `json:"size_bytes"`
	Period       string   `json:"period"`
	ProductCount int      `json:"product_count"`
	SheetsWrit   int      `json:"sheets_written"`
	Skipped      []int64  `json:"skipped_product_sys_ids,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

// Handle is the entry point bound to the rabbitmq consumer in cmd/worker.
//
// Lifecycle: PROCESSING -> (success: COMPLETED + notif SUCCESS) | (failure: FAILED + notif ERROR).
func (h *CostSheetExportHandler) Handle(ctx context.Context, msg rabbitmq.JobMessage) error {
	jobID, err := uuid.Parse(msg.JobID)
	if err != nil {
		return fmt.Errorf("invalid job id: %w", err)
	}

	exec, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}
	if err := exec.Start(); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("cost sheet export: job state transition failed; continuing")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("cost sheet export: persist PROCESSING failed")
	}

	result, runErr := h.runExport(ctx, msg, h.resolveCalcType(exec))
	if runErr != nil {
		h.markFailedAndNotify(ctx, exec, msg, runErr)
		// Handled via job_execution + notification; returning nil keeps the
		// message off the dead-letter queue.
		return nil
	}

	h.completeJob(ctx, exec, msg, result)
	// A batch child never gets its own EXPORT_READY notification — only the
	// parent fires one, exactly once, when every child has finished (see
	// handleChildCompletion). A standalone (non-batch) job notifies as before.
	if exec.IsChild() {
		h.handleChildCompletion(ctx, exec, msg, true)
	} else {
		h.emitCostSheetReadyNotification(ctx, msg, result)
	}
	h.logger.Info().
		Str("job_id", msg.JobID).
		Str("file_path", result.FilePath).
		Int("product_count", result.ProductCount).
		Int("sheets_written", result.SheetsWrit).
		Msg("product cost sheet export completed")
	return nil
}

// completeJob persists the COMPLETED status with the serialized summary.
func (h *CostSheetExportHandler) completeJob(
	ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, result *costSheetResult,
) {
	summaryJSON, mErr := json.Marshal(result)
	if mErr != nil {
		h.logger.Warn().Err(mErr).Str("job_id", msg.JobID).Msg("cost sheet export: marshal result_summary failed")
		summaryJSON = []byte("{}")
	}
	if err := exec.Complete(summaryJSON); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("cost sheet export: complete state transition failed")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Msg("cost sheet export: persist COMPLETED failed")
	}
}

// resolveCalcType reads calculation_type out of the job params. The publish
// message carries no calc type (the ExportJobPublisher contract is fixed), so
// the params JSON written by the request handler is the source of truth.
func (h *CostSheetExportHandler) resolveCalcType(exec *job.Execution) costcalcdom.CalculationType {
	raw := exec.Params()
	if len(raw) == 0 {
		return h.calcType
	}
	var params struct {
		CalculationType string `json:"calculation_type"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		h.logger.Warn().Err(err).Msg("cost sheet export: decode params failed; using default calculation type")
		return h.calcType
	}
	if params.CalculationType == "" {
		return h.calcType
	}
	return costcalcdom.CalculationType(params.CalculationType)
}

// runExport builds the workbook and uploads it, returning the run summary.
func (h *CostSheetExportHandler) runExport(
	ctx context.Context, msg rabbitmq.JobMessage, calcType costcalcdom.CalculationType,
) (*costSheetResult, error) {
	if h.storage == nil {
		return nil, fmt.Errorf("storage unavailable")
	}
	if len(msg.ProductSysIDs) == 0 {
		return nil, fmt.Errorf("no products requested")
	}

	book, stats, err := h.buildWorkbook(ctx, msg, calcType)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cErr := book.Close(); cErr != nil {
			h.logger.Warn().Err(cErr).Str("job_id", msg.JobID).Msg("cost sheet export: close workbook")
		}
	}()

	// Written to a temp file on local disk rather than a bytes.Buffer: the
	// excelize.File already holds the workbook's decompressed representation in
	// memory, so serializing a second full copy into a buffer would double peak
	// memory for bulk exports (one sheet per product, up to maxExportProducts —
	// see costsheet.maxExportProducts). Spilling to disk keeps peak memory to
	// roughly one workbook's worth and gives PutObject a known size via Stat, so
	// it can do a single atomic PUT instead of a size-unknown (memory-heavy)
	// multipart upload.
	sizeBytes, tmpPath, err := writeWorkbookToTemp(book)
	if err != nil {
		return nil, err
	}
	defer func() {
		if rmErr := os.Remove(tmpPath); rmErr != nil && !os.IsNotExist(rmErr) {
			h.logger.Warn().Err(rmErr).Str("job_id", msg.JobID).Str("tmp_path", tmpPath).
				Msg("cost sheet export: remove temp workbook file")
		}
	}()

	objectKey, fileName := costSheetObjectKey(msg, stats.fgLabels)
	if err := h.uploadTempFile(ctx, tmpPath, objectKey); err != nil {
		return nil, err
	}

	return &costSheetResult{
		FilePath:     objectKey,
		FileName:     fileName,
		SizeBytes:    int(sizeBytes),
		Period:       msg.Period,
		ProductCount: len(msg.ProductSysIDs),
		SheetsWrit:   stats.written,
		Skipped:      stats.skipped,
		Warnings:     stats.warnings,
	}, nil
}

// writeWorkbookToTemp serializes the workbook straight to a temp file on local
// disk instead of an in-memory buffer, avoiding a second full in-memory copy of
// the xlsx alongside excelize's own internal representation. Returns the final
// file size (needed by PutObject) and the temp file path; the caller owns
// removing it.
func writeWorkbookToTemp(book *excelize.File) (sizeBytes int64, tmpPath string, err error) {
	tmp, cErr := os.CreateTemp("", "cost-sheet-export-*.xlsx")
	if cErr != nil {
		return 0, "", fmt.Errorf("create temp workbook file: %w", cErr)
	}
	tmpPath = tmp.Name()

	writeErr := book.Write(tmp)
	closeErr := tmp.Close()
	if writeErr != nil {
		return 0, tmpPath, fmt.Errorf("write workbook: %w", writeErr)
	}
	if closeErr != nil {
		return 0, tmpPath, fmt.Errorf("close temp workbook file: %w", closeErr)
	}

	info, statErr := os.Stat(tmpPath)
	if statErr != nil {
		return 0, tmpPath, fmt.Errorf("stat temp workbook file: %w", statErr)
	}
	return info.Size(), tmpPath, nil
}

// uploadTempFile opens the spilled workbook and streams it to MinIO with a
// known size, so PutObject performs a single atomic (or size-bounded
// multipart) upload rather than a size-unknown streaming one.
func (h *CostSheetExportHandler) uploadTempFile(ctx context.Context, tmpPath, objectKey string) error {
	f, err := os.Open(tmpPath) //nolint:gosec // path is our own os.CreateTemp output, not user input
	if err != nil {
		return fmt.Errorf("open temp workbook file: %w", err)
	}
	defer func() {
		if cErr := f.Close(); cErr != nil {
			h.logger.Warn().Err(cErr).Str("tmp_path", tmpPath).Msg("cost sheet export: close temp workbook file")
		}
	}()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat temp workbook file: %w", err)
	}
	if err := h.storage.PutObject(ctx, objectKey, f, info.Size(), xlsxContentType); err != nil {
		return fmt.Errorf("upload xlsx: %w", err)
	}
	return nil
}

// costSheetObjectKey builds the MinIO key and the suggested download filename.
// fgLabels carries each written product's resolved finished-good label
// (product_code, or product_name as fallback — never sys_id), in write order,
// as computed by finishedGoodLabel during buildWorkbook. For a single-product
// job the filename is qualified with that product's code; for a batch, a
// filename cannot hold every product code (up to 200), so it stays scoped by
// period + count instead. The object key (internal storage path) is
// unaffected — it never carries product identity today and continues not to.
func costSheetObjectKey(msg rabbitmq.JobMessage, fgLabels []string) (objectKey, fileName string) {
	yyyymm := msg.Period
	ts := time.Now().UTC().Format("20060102-150405")
	// The job ID is a UUID; its first hyphen-delimited group is short enough for
	// a filename while staying unique within a period. Cut returns the whole
	// string when no hyphen is present, which is the desired fallback.
	shortID, _, _ := strings.Cut(msg.JobID, "-")
	userIDDir := msg.RequestingUserID
	if userIDDir == "" {
		userIDDir = "unknown"
	}
	objectKey = fmt.Sprintf("exports/finance/product-cost-sheet/%s/%s/%s-%s.xlsx", yyyymm, userIDDir, ts, shortID)

	if len(msg.ProductSysIDs) == 1 && len(fgLabels) == 1 {
		fileName = fmt.Sprintf("product-cost-sheet-%s-%s.xlsx", fgLabels[0], msg.Period)
	} else {
		fileName = fmt.Sprintf("product-cost-sheet-%s-%dproducts-%s.xlsx", msg.Period, len(msg.ProductSysIDs), ts)
	}
	return objectKey, fileName
}

// exportStats accumulates the per-product outcomes surfaced in result_summary.
type exportStats struct {
	written  int
	skipped  []int64
	warnings []string
	taken    map[string]bool
	// fgLabels collects each written product's finished-good label (product_code,
	// or product_name as a fallback — never sys_id), in write order. Used to name
	// the download filename after the actual FG rather than a generic name.
	fgLabels []string
}

// buildWorkbook renders one sheet per product into a single workbook. Products
// whose route resolves to zero stages are skipped with a warning rather than
// failing the whole job.
func (h *CostSheetExportHandler) buildWorkbook(
	ctx context.Context, msg rabbitmq.JobMessage, calcType costcalcdom.CalculationType,
) (*excelize.File, *exportStats, error) {
	book := excelize.NewFile()
	stats := &exportStats{taken: map[string]bool{}}

	for _, productSysID := range msg.ProductSysIDs {
		if err := h.addProductSheet(ctx, book, stats, productSysID, msg.Period, calcType); err != nil {
			closeWorkbook(h.logger, book)
			return nil, nil, err
		}
	}

	if stats.written == 0 {
		closeWorkbook(h.logger, book)
		return nil, nil, fmt.Errorf("no product produced a cost sheet (all %d products had no route stages)", len(msg.ProductSysIDs))
	}
	if err := book.DeleteSheet(book.GetSheetName(0)); err != nil {
		h.logger.Warn().Err(err).Msg("cost sheet export: delete placeholder sheet")
	}
	book.SetActiveSheet(0)
	return book, stats, nil
}

// addProductSheet resolves, renders, and appends one product's sheet. A fatal
// error (route load or render failure) aborts the whole job; an empty route is
// recorded as a skip.
func (h *CostSheetExportHandler) addProductSheet(
	ctx context.Context,
	book *excelize.File,
	stats *exportStats,
	productSysID int64,
	period string,
	calcType costcalcdom.CalculationType,
) error {
	raw, err := h.sheets.Handle(ctx, appcostcalc.GetRouteCostSheetQuery{
		ProductSysID: productSysID,
		Period:       period,
		CalcType:     calcType,
	})
	if err != nil {
		return fmt.Errorf("load route cost sheet for product %d: %w", productSysID, err)
	}
	if len(raw) == 0 {
		stats.skipped = append(stats.skipped, productSysID)
		stats.warnings = append(stats.warnings,
			fmt.Sprintf("product %s has no route stages and was skipped", strconv.FormatInt(productSysID, 10)))
		return nil
	}

	stages := toStages(raw)
	if len(stages) > MaxStagesPerPage {
		stats.warnings = append(stats.warnings, fmt.Sprintf(
			"product %s has %d stages (over %d); the sheet will not fit one A4 page width in print",
			finishedGoodLabel(stages), len(stages), MaxStagesPerPage))
	}

	rendered, err := BuildProductCostSheet(stages)
	if err != nil {
		return fmt.Errorf("build cost sheet for product %d: %w", productSysID, err)
	}
	defer closeWorkbook(h.logger, rendered)

	fgLabel := finishedGoodLabel(stages)
	sheetName := sanitizeSheetName(fgLabel, stats.taken)
	if err := copySheet(rendered, book, sheetName); err != nil {
		return fmt.Errorf("append sheet for product %d: %w", productSysID, err)
	}
	stats.written++
	stats.fgLabels = append(stats.fgLabels, fgLabel)
	return nil
}

// toStages converts the application-layer stage DTOs into the Excel builder's
// input shape. The two structs are deliberately decoupled: the builder must not
// import the application layer.
func toStages(in []appcostcalc.RouteCostSheetStage) []Stage {
	out := make([]Stage, 0, len(in))
	for _, s := range in {
		out = append(out, Stage{
			RouteLevel:    s.RouteLevel,
			RouteSeq:      s.RouteSeq,
			RouteName:     s.RouteName,
			ItemCode:      s.ItemCode,
			ProductName:   s.ProductName,
			ShadeCode:     s.ShadeCode,
			ShadeName:     s.ShadeName,
			ProductSysID:  s.ProductSysID,
			HasCost:       s.HasCost,
			ParamSnapshot: s.ParamSnapshot,
		})
	}
	return out
}

// finishedGoodLabel returns the item code of the route-level-1 stage — the
// finished good itself (see costroute/graph.go's ValidateLevels: level 1 is
// always the single stage producing the head product; higher levels move
// upstream toward raw materials). Falls back to the first stage with an item
// code, and finally to a generic label, rather than exposing the internal
// product_sys_id to the user.
func finishedGoodLabel(stages []Stage) string {
	for _, s := range stages {
		if s.RouteLevel == 1 && s.ItemCode != "" {
			return s.ItemCode
		}
	}
	for _, s := range stages {
		if s.ItemCode != "" {
			return s.ItemCode
		}
	}
	return fallbackSheetName
}

// =============================================================================
// Workbook merging
// =============================================================================

// copySheet transplants the single sheet of a rendered per-product workbook into
// the combined workbook under dstName, preserving values, styles, column widths,
// row heights, merges, freeze panes, and page setup.
func copySheet(src, dst *excelize.File, dstName string) error {
	srcName := src.GetSheetName(0)
	if srcName == "" {
		return fmt.Errorf("source workbook has no sheet")
	}
	if _, err := dst.NewSheet(dstName); err != nil {
		return fmt.Errorf("create sheet %q: %w", dstName, err)
	}

	rows, err := src.GetRows(srcName, excelize.Options{RawCellValue: true})
	if err != nil {
		return fmt.Errorf("read source rows: %w", err)
	}
	styleCache := map[int]int{}
	maxCol := 0
	for rowIdx, row := range rows {
		if len(row) > maxCol {
			maxCol = len(row)
		}
		if err := copyRow(src, dst, srcName, dstName, rowIdx+1, len(row), styleCache); err != nil {
			return err
		}
	}

	return copyLayout(src, dst, srcName, dstName, len(rows), maxCol)
}

// copyRow copies one row's cell values and styles.
func copyRow(src, dst *excelize.File, srcName, dstName string, row, cols int, styleCache map[int]int) error {
	for col := 1; col <= cols; col++ {
		cell, err := excelize.CoordinatesToCellName(col, row)
		if err != nil {
			return fmt.Errorf("cell name (%d,%d): %w", col, row, err)
		}
		if err := copyCellValue(src, dst, srcName, dstName, cell); err != nil {
			return err
		}
		if err := copyCellStyle(src, dst, srcName, dstName, cell, styleCache); err != nil {
			return err
		}
	}
	return nil
}

// copyCellValue preserves numeric cells as numbers so the recipient can re-total
// the sheet in Excel; everything else is copied as its raw string.
func copyCellValue(src, dst *excelize.File, srcName, dstName, cell string) error {
	raw, err := src.GetCellValue(srcName, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		return fmt.Errorf("read %s: %w", cell, err)
	}
	if raw == "" {
		return nil
	}
	numeric, err := isNumericCell(src, srcName, cell)
	if err != nil {
		return err
	}
	if numeric {
		num, pErr := strconv.ParseFloat(raw, 64)
		if pErr != nil {
			return fmt.Errorf("parse numeric cell %s (%q): %w", cell, raw, pErr)
		}
		if err := dst.SetCellValue(dstName, cell, num); err != nil {
			return fmt.Errorf("write %s: %w", cell, err)
		}
		return nil
	}
	if err := dst.SetCellValue(dstName, cell, raw); err != nil {
		return fmt.Errorf("write %s: %w", cell, err)
	}
	return nil
}

// isNumericCell reports whether a cell holds a real number.
//
// OOXML omits the cell's `t` attribute for numeric cells, so excelize reports
// them as CellTypeUnset rather than CellTypeNumber — CellTypeNumber is only
// returned when the writer emitted an explicit t="n". Both must therefore be
// treated as numeric, and the raw value re-parsed to confirm. Matching on
// CellTypeNumber alone silently degrades every number in the merged workbook
// into a string, which is exactly what the export must not do.
//
// Text-shaped source cells are excluded on type alone, not on the parsed
// content. The source workbook is built by BuildProductCostSheet
// (costsheet_export_excel.go): kindSnapshot rows write real numbers via
// numericCell/SetCellFloat (t="n" or unset -> eligible above), while kindText
// rows always write through textOrDash -> excelize.SetCellStr, which stamps
// t="s" (CellTypeSharedString). CellTypeSharedString fails the type check
// above and short-circuits before ParseFloat ever runs, so a kindText cell
// cannot become numeric here no matter what its value looks like — a param
// code like "007" or "00123" is copied as a string even though ParseFloat
// would happily accept it. This holds for every current kindText manifest row
// (MB_SP_DYE, MC_NAME, OPU, CAPTIVE_PACK_CODE, DELIVERY_PACK_CODE,
// SPECIAL_COST_FLAG, NS_LOSS_TYPE, BC_LOSS_TYPE in costsheet_rows.go) and for
// any future kindText row, as long as it keeps going through textOrDash. The
// risk this function's own doc comment used to warn about only materializes
// if some future code path writes a kindText (or otherwise text-semantic)
// value into the source sheet via SetCellValue with a numeric Go type, or
// via a raw XML/streaming writer that sets t="n"/leaves t unset for a string
// payload — i.e. only a regression in the writer side, not in this reader.
func isNumericCell(f *excelize.File, sheet, cell string) (bool, error) {
	cellType, err := f.GetCellType(sheet, cell)
	if err != nil {
		return false, fmt.Errorf("read type of %s: %w", cell, err)
	}
	if cellType != excelize.CellTypeNumber && cellType != excelize.CellTypeUnset {
		return false, nil
	}
	raw, err := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		return false, fmt.Errorf("read %s: %w", cell, err)
	}
	if _, pErr := strconv.ParseFloat(raw, 64); pErr != nil {
		// A non-parsable raw value simply is not a number (e.g. the "-"
		// placeholder). That is a classification result, not a failure.
		return false, nil //nolint:nilerr // unparsable means "not numeric", not an error
	}
	return true, nil
}

// copyCellStyle re-registers the source style in the destination workbook once
// per distinct source style index, then applies it.
func copyCellStyle(src, dst *excelize.File, srcName, dstName, cell string, cache map[int]int) error {
	srcStyleID, err := src.GetCellStyle(srcName, cell)
	if err != nil {
		return fmt.Errorf("read style of %s: %w", cell, err)
	}
	if srcStyleID == 0 {
		return nil
	}
	dstStyleID, ok := cache[srcStyleID]
	if !ok {
		style, sErr := src.GetStyle(srcStyleID)
		if sErr != nil {
			return fmt.Errorf("resolve style %d: %w", srcStyleID, sErr)
		}
		dstStyleID, err = dst.NewStyle(style)
		if err != nil {
			return fmt.Errorf("register style %d: %w", srcStyleID, err)
		}
		cache[srcStyleID] = dstStyleID
	}
	if err := dst.SetCellStyle(dstName, cell, cell, dstStyleID); err != nil {
		return fmt.Errorf("apply style to %s: %w", cell, err)
	}
	return nil
}

// copyLayout carries over the print/visual setup of the source sheet.
func copyLayout(src, dst *excelize.File, srcName, dstName string, rowCount, colCount int) error {
	if err := copyDimensions(src, dst, srcName, dstName, rowCount, colCount); err != nil {
		return err
	}
	if err := copyMerges(src, dst, srcName, dstName); err != nil {
		return err
	}
	return copyPageSetup(src, dst, srcName, dstName)
}

// copyDimensions copies per-column widths and per-row heights.
func copyDimensions(src, dst *excelize.File, srcName, dstName string, rowCount, colCount int) error {
	for col := 1; col <= colCount; col++ {
		name, err := excelize.ColumnNumberToName(col)
		if err != nil {
			return fmt.Errorf("column name %d: %w", col, err)
		}
		width, err := src.GetColWidth(srcName, name)
		if err != nil {
			return fmt.Errorf("read width of %s: %w", name, err)
		}
		if err := dst.SetColWidth(dstName, name, name, width); err != nil {
			return fmt.Errorf("set width of %s: %w", name, err)
		}
	}
	for row := 1; row <= rowCount; row++ {
		height, err := src.GetRowHeight(srcName, row)
		if err != nil {
			return fmt.Errorf("read height of row %d: %w", row, err)
		}
		if err := dst.SetRowHeight(dstName, row, height); err != nil {
			return fmt.Errorf("set height of row %d: %w", row, err)
		}
	}
	return nil
}

// copyMerges replays the source sheet's merged ranges.
func copyMerges(src, dst *excelize.File, srcName, dstName string) error {
	merges, err := src.GetMergeCells(srcName)
	if err != nil {
		return fmt.Errorf("read merges: %w", err)
	}
	for _, m := range merges {
		if err := dst.MergeCell(dstName, m.GetStartAxis(), m.GetEndAxis()); err != nil {
			return fmt.Errorf("merge %s:%s: %w", m.GetStartAxis(), m.GetEndAxis(), err)
		}
	}
	return nil
}

// copyPageSetup copies page layout, margins, and freeze panes.
func copyPageSetup(src, dst *excelize.File, srcName, dstName string) error {
	layout, err := src.GetPageLayout(srcName)
	if err != nil {
		return fmt.Errorf("read page layout: %w", err)
	}
	if err := dst.SetPageLayout(dstName, &layout); err != nil {
		return fmt.Errorf("set page layout: %w", err)
	}
	margins, err := src.GetPageMargins(srcName)
	if err != nil {
		return fmt.Errorf("read page margins: %w", err)
	}
	if err := dst.SetPageMargins(dstName, &margins); err != nil {
		return fmt.Errorf("set page margins: %w", err)
	}
	panes, err := src.GetPanes(srcName)
	if err != nil {
		return fmt.Errorf("read panes: %w", err)
	}
	if !panes.Freeze && !panes.Split {
		return nil
	}
	if err := dst.SetPanes(dstName, &panes); err != nil {
		return fmt.Errorf("set panes: %w", err)
	}
	return nil
}

// closeWorkbook releases an excelize file's temp resources, logging any failure.
func closeWorkbook(logger zerolog.Logger, f *excelize.File) {
	if f == nil {
		return
	}
	if err := f.Close(); err != nil {
		logger.Warn().Err(err).Msg("cost sheet export: close workbook")
	}
}

// =============================================================================
// Job failure + notifications
// =============================================================================

// markFailedAndNotify updates the job_execution row to FAILED and emits an ERROR
// notification to the requester. Best-effort: internal errors are logged, not
// propagated, so the rabbitmq message is still ACKed.
func (h *CostSheetExportHandler) markFailedAndNotify(
	ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, runErr error,
) {
	if err := exec.Fail(runErr.Error()); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("cost sheet export: fail state transition")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Msg("cost sheet export: persist FAILED")
	}
	if exec.IsChild() {
		h.handleChildCompletion(ctx, exec, msg, false)
	} else {
		h.emitCostSheetFailureNotification(ctx, msg, runErr)
	}
	h.logger.Error().Err(runErr).Str("job_id", msg.JobID).Msg("product cost sheet export failed")
}

// handleChildCompletion atomically increments the parent batch job's
// completed/failed counter for one finished child and, when that increment
// reports the batch is now fully done, fires exactly one EXPORT_READY (or
// failure) notification targeting the parent job — never per child. Uses the
// repository's single UPDATE ... RETURNING so concurrent children finishing
// at the same moment cannot race the completion transition (double-fire or
// never-fire).
func (h *CostSheetExportHandler) handleChildCompletion(
	ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, success bool,
) {
	parentID := exec.ParentJobID()
	if parentID == nil {
		return
	}
	batchComplete, err := h.jobRepo.IncrementChildProgress(ctx, *parentID, success)
	if err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Str("parent_job_id", parentID.String()).
			Msg("cost sheet export: increment parent batch progress failed")
		return
	}
	if !batchComplete {
		return
	}
	h.notifyBatchComplete(ctx, *parentID, msg)
}

// notifyBatchComplete loads the now-fully-finished parent job and fires its
// single completion notification, reflecting how many of its children
// succeeded vs. failed. Marks the parent job_execution row itself
// COMPLETED/FAILED so it stops showing as PROCESSING in the job list.
func (h *CostSheetExportHandler) notifyBatchComplete(ctx context.Context, parentID uuid.UUID, msg rabbitmq.JobMessage) {
	parent, err := h.jobRepo.GetByID(ctx, parentID)
	if err != nil {
		h.logger.Error().Err(err).Str("parent_job_id", parentID.String()).
			Msg("cost sheet export: load parent job for batch-complete notification failed")
		return
	}

	allFailed := parent.CompletedChildren() == 0 && parent.FailedChildren() > 0
	if err := h.completeParentJob(ctx, parent, allFailed); err != nil {
		h.logger.Warn().Err(err).Str("parent_job_id", parentID.String()).
			Msg("cost sheet export: persist parent batch completion status failed")
	}

	parentMsg := msg
	parentMsg.JobID = parentID.String()
	if allFailed {
		h.emitCostSheetFailureNotification(ctx, parentMsg, fmt.Errorf("all %d export files failed", parent.FailedChildren()))
		return
	}
	h.emitBatchReadyNotification(ctx, parentMsg, parent)
}

// completeParentJob transitions the parent job to a terminal status once its
// batch is complete. A parent has no workbook of its own to summarize, so
// result_summary just records the final child tallies.
func (h *CostSheetExportHandler) completeParentJob(ctx context.Context, parent *job.Execution, allFailed bool) error {
	summary, mErr := json.Marshal(map[string]any{
		"total_children":     parent.TotalChildren(),
		"completed_children": parent.CompletedChildren(),
		"failed_children":    parent.FailedChildren(),
	})
	if mErr != nil {
		summary = []byte("{}")
	}
	if allFailed {
		if err := parent.Fail(fmt.Sprintf("all %d child export jobs failed", parent.FailedChildren())); err != nil {
			return err
		}
	} else if err := parent.Complete(summary); err != nil {
		return err
	}
	return h.jobRepo.UpdateStatus(ctx, parent)
}

// emitBatchReadyNotification fires the single batch-completion notification,
// reusing the same EXPORT_READY notification type as a standalone export but
// with a body summarizing N files across the whole batch instead of one
// file's stats (a batch parent has no single file to link to — each child's
// file is downloaded individually from the Cost Results page's batch-files
// popover). Uses NAVIGATE (not DOWNLOAD, unlike the standalone case) so the
// click-through takes the user to the Cost Results page with the parent job
// id set in the URL, which surfaces that popover instead of attempting a
// direct download of a job that has no single file.
func (h *CostSheetExportHandler) emitBatchReadyNotification(ctx context.Context, msg rabbitmq.JobMessage, parent *job.Execution) {
	if h.notif == nil || msg.RequestingUserID == "" {
		return
	}
	body := fmt.Sprintf("Period %s • %d file selesai", msg.Period, parent.CompletedChildren())
	if parent.FailedChildren() > 0 {
		body += fmt.Sprintf(" • %d file gagal", parent.FailedChildren())
	}
	payload, mErr := json.Marshal(map[string]any{
		"path": "/finance/cost-results?exportJobId=" + url.QueryEscape(msg.JobID),
	})
	if mErr != nil {
		h.logger.Warn().Err(mErr).Str("job_id", msg.JobID).Msg("cost sheet export: marshal batch action_payload failed")
		payload = []byte("{}")
	}
	if err := h.notif.Create(ctx, iamclient.CreateNotificationParams{
		RecipientUserID: msg.RequestingUserID,
		Type:            iamv1.NotificationType_NOTIFICATION_TYPE_EXPORT_READY,
		Severity:        iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_SUCCESS,
		Title:           "Export Product Cost Sheet (batch) selesai",
		Body:            body,
		ActionType:      iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NAVIGATE,
		ActionPayload:   string(payload),
		SourceType:      costSheetSourceType,
		SourceID:        msg.JobID,
	}); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("cost sheet export: create batch EXPORT_READY notification failed")
	}
}

func (h *CostSheetExportHandler) emitCostSheetReadyNotification(
	ctx context.Context, msg rabbitmq.JobMessage, r *costSheetResult,
) {
	if h.notif == nil || msg.RequestingUserID == "" {
		return
	}
	expiresAt := time.Now().UTC().Add(ExportNotificationExpiry).Format(time.RFC3339)
	payload, mErr := json.Marshal(map[string]any{
		"file_path":  r.FilePath,
		"file_name":  r.FileName,
		"size_bytes": r.SizeBytes,
		"expires_at": expiresAt,
	})
	if mErr != nil {
		h.logger.Warn().Err(mErr).Str("job_id", msg.JobID).Msg("cost sheet export: marshal action_payload failed")
		payload = []byte("{}")
	}
	body := fmt.Sprintf("Period %s • %d produk • %d sheet • %d KB",
		r.Period, r.ProductCount, r.SheetsWrit, r.SizeBytes/1024)
	if len(r.Skipped) > 0 {
		body += fmt.Sprintf(" • %d produk dilewati (tanpa route)", len(r.Skipped))
	}
	if err := h.notif.Create(ctx, iamclient.CreateNotificationParams{
		RecipientUserID: msg.RequestingUserID,
		Type:            iamv1.NotificationType_NOTIFICATION_TYPE_EXPORT_READY,
		Severity:        iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_SUCCESS,
		Title:           "Export Product Cost Sheet selesai",
		Body:            body,
		ActionType:      iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_DOWNLOAD,
		ActionPayload:   string(payload),
		SourceType:      costSheetSourceType,
		SourceID:        msg.JobID,
		ExpiresAt:       expiresAt,
	}); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("cost sheet export: create EXPORT_READY notification failed")
	}
}

func (h *CostSheetExportHandler) emitCostSheetFailureNotification(
	ctx context.Context, msg rabbitmq.JobMessage, runErr error,
) {
	if h.notif == nil || msg.RequestingUserID == "" {
		return
	}
	if err := h.notif.Create(ctx, iamclient.CreateNotificationParams{
		RecipientUserID: msg.RequestingUserID,
		Type:            iamv1.NotificationType_NOTIFICATION_TYPE_EXPORT_READY,
		Severity:        iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_ERROR,
		Title:           "Export Product Cost Sheet gagal",
		Body:            truncate(runErr.Error(), 500),
		ActionType:      iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NONE,
		ActionPayload:   "",
		SourceType:      costSheetSourceType,
		SourceID:        msg.JobID,
	}); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("cost sheet export: create failure notification failed")
	}
}
