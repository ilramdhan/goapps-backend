package costbulkimport

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/rs/zerolog"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// ExportHandler generates a 6-sheet bulk export Excel file from database data.
// The output can be used as an import template pre-populated with existing data.
type ExportHandler struct {
	cpmRepo   costproductmaster.Repository
	cppRepo   costproductparameter.Repository
	typeRepo  costproducttype.Repository
	routeRepo costroute.Repository
	jobRepo   costimportjob.Repository
	storage   storage.Service
	logger    zerolog.Logger
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(
	cpmRepo costproductmaster.Repository,
	cppRepo costproductparameter.Repository,
	typeRepo costproducttype.Repository,
	routeRepo costroute.Repository,
	jobRepo costimportjob.Repository,
	storageSvc storage.Service,
	logger zerolog.Logger,
) *ExportHandler {
	return &ExportHandler{
		cpmRepo:   cpmRepo,
		cppRepo:   cppRepo,
		typeRepo:  typeRepo,
		routeRepo: routeRepo,
		jobRepo:   jobRepo,
		storage:   storageSvc,
		logger:    logger,
	}
}

// ExportRequest carries the parameters for a bulk export.
type ExportRequest struct {
	ProductTypeCodes []string `json:"product_type_codes"`
	IncludeRouting   bool     `json:"include_routing"`
	ActiveOnly       bool     `json:"active_only"`
	Actor            string   `json:"actor"`
}

// Handle executes the async export for the given job.
// Generates the 6-sheet Excel, uploads to MinIO, marks job DONE.
func (h *ExportHandler) Handle(ctx context.Context, jobID int64, req ExportRequest) error {
	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load export job %d: %w", jobID, err)
	}
	job.MarkRunning()
	h.updateExportJob(ctx, jobID, job)

	data, loadErr := h.loadExportData(ctx, req)
	if loadErr != nil {
		job.MarkFailed(loadErr.Error())
		h.updateExportJob(ctx, jobID, job)
		return loadErr
	}

	excelBytes, genErr := h.generateExcel(data)
	if genErr != nil {
		job.MarkFailed(genErr.Error())
		h.updateExportJob(ctx, jobID, job)
		return genErr
	}

	key := fmt.Sprintf("exports/bulk-product-routing/%s/export.xlsx", strconv.FormatInt(jobID, 10))
	putErr := h.storage.PutObject(
		ctx, key,
		bytes.NewReader(excelBytes),
		int64(len(excelBytes)),
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	)
	if putErr != nil {
		job.MarkFailed(putErr.Error())
		h.updateExportJob(ctx, jobID, job)
		return fmt.Errorf("upload export: %w", putErr)
	}

	job.MarkDone(key)
	h.updateExportJob(ctx, jobID, job)
	h.logger.Info().Int64("job_id", jobID).Str("key", key).Msg("bulk export: uploaded")
	return nil
}

// exportData bundles all queried data needed to build the export Excel.
type exportData struct {
	products     []*costproductmaster.CostProductMaster
	typeIDToCode map[int32]string
	cppRows      []costproductparameter.CPPRow
	cappRows     []costproductparameter.CAPPRow
	heads        []costroute.ExportRouteHead
	seqs         []costroute.ExportRouteSeq
	rms          []costroute.ExportRouteRM
}

// loadExportData fetches all six datasets from the database.
func (h *ExportHandler) loadExportData(ctx context.Context, req ExportRequest) (*exportData, error) {
	// Load product types to build ID↔code maps.
	productTypes, typeErr := h.typeRepo.ListAllActive(ctx)
	if typeErr != nil {
		return nil, fmt.Errorf("list product types: %w", typeErr)
	}
	typeCodeToID := make(map[string]int32, len(productTypes))
	typeIDToCode := make(map[int32]string, len(productTypes))
	for _, t := range productTypes {
		typeCodeToID[t.TypeCode()] = t.TypeID()
		typeIDToCode[t.TypeID()] = t.TypeCode()
	}

	filter := costproductmaster.Filter{}
	if req.ActiveOnly {
		filter.ActiveFilter = "active"
	}
	products, listErr := h.cpmRepo.ListAll(ctx, filter)
	if listErr != nil {
		return nil, fmt.Errorf("list products: %w", listErr)
	}

	// Apply ProductTypeCodes filter when requested.
	products = filterProductsByTypeCode(products, req.ProductTypeCodes, typeCodeToID)

	// Build sys_id set for routing queries.
	sysIDs := make([]int64, 0, len(products))
	for _, p := range products {
		sysIDs = append(sysIDs, p.ProductSysID())
	}

	cppRows, cppErr := h.cppRepo.ListAllValues(ctx)
	if cppErr != nil {
		return nil, fmt.Errorf("list cpp values: %w", cppErr)
	}

	cappRows, cappErr := h.cppRepo.ListAllApplicable(ctx)
	if cappErr != nil {
		return nil, fmt.Errorf("list capp rows: %w", cappErr)
	}

	data := &exportData{
		products:     products,
		typeIDToCode: typeIDToCode,
		cppRows:      cppRows,
		cappRows:     cappRows,
	}

	if !req.IncludeRouting {
		return data, nil
	}

	heads, headsErr := h.routeRepo.ListAllHeadsForExport(ctx, sysIDs)
	if headsErr != nil {
		return nil, fmt.Errorf("list route heads: %w", headsErr)
	}

	headIDs := make([]int64, 0, len(heads))
	for _, hd := range heads {
		headIDs = append(headIDs, hd.HeadID)
	}

	seqs, seqsErr := h.routeRepo.ListAllSeqsForExport(ctx, headIDs)
	if seqsErr != nil {
		return nil, fmt.Errorf("list route seqs: %w", seqsErr)
	}

	seqIDs := make([]int64, 0, len(seqs))
	seqToHead := make(map[int64]int64, len(seqs))
	for _, s := range seqs {
		seqIDs = append(seqIDs, s.SeqID)
		seqToHead[s.SeqID] = s.HeadID
	}

	rms, rmsErr := h.routeRepo.ListAllRMsForExport(ctx, seqIDs)
	if rmsErr != nil {
		return nil, fmt.Errorf("list route rms: %w", rmsErr)
	}

	// Populate HeadID on each RM from the seq→head map.
	for i := range rms {
		rms[i].HeadID = seqToHead[rms[i].SeqID]
	}

	data.heads = heads
	data.seqs = seqs
	data.rms = rms
	return data, nil
}

// filterProductsByTypeCode filters products to only those whose type code is in typeCodes.
// Returns the original slice unchanged when typeCodes is empty.
func filterProductsByTypeCode(
	products []*costproductmaster.CostProductMaster,
	typeCodes []string,
	typeCodeToID map[string]int32,
) []*costproductmaster.CostProductMaster {
	if len(typeCodes) == 0 {
		return products
	}
	allowed := make(map[int32]bool, len(typeCodes))
	for _, code := range typeCodes {
		if id, ok := typeCodeToID[code]; ok {
			allowed[id] = true
		}
	}
	filtered := products[:0]
	for _, p := range products {
		if allowed[p.ProductTypeID()] {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// generateExcel builds the 6-sheet export Excel in memory.
func (h *ExportHandler) generateExcel(data *exportData) ([]byte, error) {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			_ = err
		}
	}()

	if err := writeProductMasterSheet(f, data.products, data.typeIDToCode); err != nil {
		return nil, err
	}
	if err := writeCPPSheet(f, data.cppRows); err != nil {
		return nil, err
	}
	if err := writeCAPPSheet(f, data.cappRows); err != nil {
		return nil, err
	}
	if err := writeRouteHeadSheet(f, data.heads); err != nil {
		return nil, err
	}
	if err := writeRouteSeqSheet(f, data.seqs); err != nil {
		return nil, err
	}
	if err := writeRouteRMSheet(f, data.rms); err != nil {
		return nil, err
	}

	// Remove the default blank Sheet1 if it still exists.
	if err := f.DeleteSheet("Sheet1"); err != nil {
		_ = err
	}

	var buf bytes.Buffer
	if writeErr := f.Write(&buf); writeErr != nil {
		return nil, fmt.Errorf("write export excel: %w", writeErr)
	}
	return buf.Bytes(), nil
}

// updateExportJob persists job state and logs on failure without propagating.
func (h *ExportHandler) updateExportJob(ctx context.Context, jobID int64, job *costimportjob.CostImportJob) {
	if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
		h.logger.Warn().Err(updateErr).Int64("job_id", jobID).Msg("bulk export: failed to persist job state")
	}
}

// writeProductMasterSheet writes the product_master sheet.
func writeProductMasterSheet(f *excelize.File, products []*costproductmaster.CostProductMaster, typeIDToCode map[int32]string) error {
	const sheetName = "product_master"
	if _, err := f.NewSheet(sheetName); err != nil {
		return fmt.Errorf("create sheet %s: %w", sheetName, err)
	}
	headers := []string{
		"legacy_oracle_sys_id", "product_code", "product_name",
		"product_type_code", "shade_code", "shade_name",
		"grade_code", "erp_item_code", "is_active",
	}
	if err := writeSheetHeaders(f, sheetName, headers); err != nil {
		return err
	}
	for rowIdx, p := range products {
		vals := []any{
			p.Flex02(), p.ProductCode(), p.ProductName(),
			typeIDToCode[p.ProductTypeID()], p.ShadeCode(), p.ShadeName(),
			p.GradeCode(), p.ErpItemCode(), p.IsActive(),
		}
		if err := writeSheetRow(f, sheetName, rowIdx+2, vals); err != nil {
			return err
		}
	}
	return nil
}

// writeCPPSheet writes the product_parameters sheet.
func writeCPPSheet(f *excelize.File, rows []costproductparameter.CPPRow) error {
	const sheetName = "product_parameters"
	if _, err := f.NewSheet(sheetName); err != nil {
		return fmt.Errorf("create sheet %s: %w", sheetName, err)
	}
	headers := []string{"legacy_oracle_sys_id", "param_code", "value_numeric", "value_text", "value_flag"}
	if err := writeSheetHeaders(f, sheetName, headers); err != nil {
		return err
	}
	for rowIdx, r := range rows {
		vals := []any{r.ProductCode, r.ParamCode, ptrStringOrEmpty(r.ValueNumeric), ptrStringOrEmpty(r.ValueText), ptrBoolOrEmpty(r.ValueFlag)}
		if err := writeSheetRow(f, sheetName, rowIdx+2, vals); err != nil {
			return err
		}
	}
	return nil
}

// writeCAPPSheet writes the product_applicable_params sheet.
func writeCAPPSheet(f *excelize.File, rows []costproductparameter.CAPPRow) error {
	const sheetName = "product_applicable_params"
	if _, err := f.NewSheet(sheetName); err != nil {
		return fmt.Errorf("create sheet %s: %w", sheetName, err)
	}
	headers := []string{"legacy_oracle_sys_id", "param_code", "is_required", "display_order"}
	if err := writeSheetHeaders(f, sheetName, headers); err != nil {
		return err
	}
	for rowIdx, r := range rows {
		vals := []any{r.ProductCode, r.ParamCode, r.IsRequired, ptrInt32OrEmpty(r.DisplayOrder)}
		if err := writeSheetRow(f, sheetName, rowIdx+2, vals); err != nil {
			return err
		}
	}
	return nil
}

// writeRouteHeadSheet writes the route_head sheet.
func writeRouteHeadSheet(f *excelize.File, heads []costroute.ExportRouteHead) error {
	const sheetName = "route_head"
	if _, err := f.NewSheet(sheetName); err != nil {
		return fmt.Errorf("create sheet %s: %w", sheetName, err)
	}
	headers := []string{"legacy_oracle_sys_id", "routing_status", "notes"}
	if err := writeSheetHeaders(f, sheetName, headers); err != nil {
		return err
	}
	for rowIdx, h := range heads {
		vals := []any{strconv.FormatInt(h.ProductSysID, 10), h.RoutingStatus, h.Notes}
		if err := writeSheetRow(f, sheetName, rowIdx+2, vals); err != nil {
			return err
		}
	}
	return nil
}

// writeRouteSeqSheet writes the route_sequences sheet.
func writeRouteSeqSheet(f *excelize.File, seqs []costroute.ExportRouteSeq) error {
	const sheetName = "route_sequences"
	if _, err := f.NewSheet(sheetName); err != nil {
		return fmt.Errorf("create sheet %s: %w", sheetName, err)
	}
	headers := []string{
		"route_head_legacy_product_id", "node_product_legacy_id",
		"route_level", "route_seq", "route_name", "route_item_code",
		"route_shade_code", "route_shade_name",
	}
	if err := writeSheetHeaders(f, sheetName, headers); err != nil {
		return err
	}
	for rowIdx, s := range seqs {
		vals := []any{
			strconv.FormatInt(s.HeadID, 10), strconv.FormatInt(s.ProductSysID, 10),
			s.RouteLevel, s.RouteSeq, s.RouteName, s.RouteItemCode,
			s.RouteShadeCode, s.RouteShadeName,
		}
		if err := writeSheetRow(f, sheetName, rowIdx+2, vals); err != nil {
			return err
		}
	}
	return nil
}

// writeRouteRMSheet writes the route_rms sheet.
func writeRouteRMSheet(f *excelize.File, rms []costroute.ExportRouteRM) error {
	const sheetName = "route_rms"
	if _, err := f.NewSheet(sheetName); err != nil {
		return fmt.Errorf("create sheet %s: %w", sheetName, err)
	}
	headers := []string{
		"route_head_legacy_product_id", "route_level", "route_seq",
		"rm_type", "ratio", "rm_product_legacy_id",
		"rm_item_code", "rm_group_code", "rm_name",
		"rm_shade_code", "rm_shade_name", "sub_type", "notes",
	}
	if err := writeSheetHeaders(f, sheetName, headers); err != nil {
		return err
	}
	for rowIdx, rm := range rms {
		rmLegacyID := ""
		if rm.RmProductSysID != 0 {
			rmLegacyID = strconv.FormatInt(rm.RmProductSysID, 10)
		}
		vals := []any{
			strconv.FormatInt(rm.HeadID, 10), rm.RouteLevel, rm.RouteSeq,
			rm.RmType, rm.Ratio, rmLegacyID,
			rm.RmItemCode, rm.RmGroupCode, rm.RmName,
			rm.RmShadeCode, rm.RmShadeName, rm.SubType, rm.Notes,
		}
		if err := writeSheetRow(f, sheetName, rowIdx+2, vals); err != nil {
			return err
		}
	}
	return nil
}

// writeSheetHeaders writes a header row to the named sheet.
func writeSheetHeaders(f *excelize.File, sheetName string, headers []string) error {
	for col, h := range headers {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("header cell name: %w", cellErr)
		}
		if err := f.SetCellValue(sheetName, cell, h); err != nil {
			return fmt.Errorf("set cell %s: %w", cell, err)
		}
	}
	return nil
}

// writeSheetRow writes one data row at the given 1-based rowIdx.
func writeSheetRow(f *excelize.File, sheetName string, rowIdx int, vals []any) error {
	for col, v := range vals {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, rowIdx)
		if cellErr != nil {
			return fmt.Errorf("data cell name: %w", cellErr)
		}
		if err := f.SetCellValue(sheetName, cell, v); err != nil {
			return fmt.Errorf("set cell %s: %w", cell, err)
		}
	}
	return nil
}

// ptrStringOrEmpty returns the pointed-to string or empty string when nil.
func ptrStringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ptrBoolOrEmpty returns "true"/"false" or empty string when nil.
func ptrBoolOrEmpty(b *bool) string {
	if b == nil {
		return ""
	}
	if *b {
		return boolTrueStr
	}
	return "false"
}

// ptrInt32OrEmpty returns the int32 value or empty string when nil.
func ptrInt32OrEmpty(v *int32) any {
	if v == nil {
		return ""
	}
	return *v
}
