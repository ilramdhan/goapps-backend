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
	// ProductSysIDs, when non-empty, restricts the export to the transitive closure
	// of the given products (each listed product plus every intermediate product
	// reachable via PRODUCT-type route RMs). Takes precedence over ProductTypeCodes.
	ProductSysIDs []int64 `json:"product_sys_ids"`
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

	// ProductSysIDs takes precedence: resolve transitive closure then filter.
	// Otherwise fall back to ProductTypeCodes filter.
	if len(req.ProductSysIDs) > 0 {
		closureIDs, closureErr := resolveProductClosure(ctx, req.ProductSysIDs, h.routeRepo)
		if closureErr != nil {
			return nil, fmt.Errorf("resolve product closure: %w", closureErr)
		}
		products = filterProductsBySysIDs(products, closureIDs)
	} else {
		products = filterProductsByTypeCode(products, req.ProductTypeCodes, typeCodeToID)
	}

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

	heads, seqs, rms, routeErr := h.loadRoutingData(ctx, sysIDs)
	if routeErr != nil {
		return nil, routeErr
	}

	data.heads = heads
	data.seqs = seqs
	data.rms = rms
	return data, nil
}

// loadRoutingData fetches heads, sequences, and RMs for the given product sys IDs.
func (h *ExportHandler) loadRoutingData(ctx context.Context, sysIDs []int64) (
	heads []costroute.ExportRouteHead,
	seqs []costroute.ExportRouteSeq,
	rms []costroute.ExportRouteRM,
	err error,
) {
	heads, err = h.routeRepo.ListAllHeadsForExport(ctx, sysIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list route heads: %w", err)
	}

	headIDs := make([]int64, 0, len(heads))
	for _, hd := range heads {
		headIDs = append(headIDs, hd.HeadID)
	}

	seqs, err = h.routeRepo.ListAllSeqsForExport(ctx, headIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list route seqs: %w", err)
	}

	seqIDs := make([]int64, 0, len(seqs))
	seqToHead := make(map[int64]int64, len(seqs))
	for _, s := range seqs {
		seqIDs = append(seqIDs, s.SeqID)
		seqToHead[s.SeqID] = s.HeadID
	}

	rms, err = h.routeRepo.ListAllRMsForExport(ctx, seqIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list route rms: %w", err)
	}

	// Populate HeadID on each RM from the seq→head map.
	for i := range rms {
		rms[i].HeadID = seqToHead[rms[i].SeqID]
	}
	return heads, seqs, rms, nil
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

// resolveProductClosure performs a BFS from seedIDs through PRODUCT-type route RMs
// and returns the full set of product sys IDs (seeds + all transitively reachable intermediates).
func resolveProductClosure(ctx context.Context, seedIDs []int64, repo costroute.Repository) ([]int64, error) { //nolint:gocognit // BFS traversal with layered early-exit is inherently nested
	seen := make(map[int64]bool, len(seedIDs))
	for _, id := range seedIDs {
		seen[id] = true
	}
	queue := append([]int64(nil), seedIDs...)

	for len(queue) > 0 {
		heads, headsErr := repo.ListAllHeadsForExport(ctx, queue)
		if headsErr != nil {
			return nil, fmt.Errorf("closure: list heads: %w", headsErr)
		}
		headIDs := make([]int64, 0, len(heads))
		for _, hd := range heads {
			headIDs = append(headIDs, hd.HeadID)
		}

		if len(headIDs) == 0 {
			break
		}

		seqs, seqsErr := repo.ListAllSeqsForExport(ctx, headIDs)
		if seqsErr != nil {
			return nil, fmt.Errorf("closure: list seqs: %w", seqsErr)
		}
		seqIDs := make([]int64, 0, len(seqs))
		for _, s := range seqs {
			seqIDs = append(seqIDs, s.SeqID)
		}

		if len(seqIDs) == 0 {
			break
		}

		rms, rmsErr := repo.ListAllRMsForExport(ctx, seqIDs)
		if rmsErr != nil {
			return nil, fmt.Errorf("closure: list rms: %w", rmsErr)
		}

		queue = queue[:0]
		for _, rm := range rms {
			if rm.RmType == costroute.RmTypeProduct && rm.RmProductSysID != 0 && !seen[rm.RmProductSysID] {
				seen[rm.RmProductSysID] = true
				queue = append(queue, rm.RmProductSysID)
			}
		}
	}

	result := make([]int64, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	return result, nil
}

// filterProductsBySysIDs filters products to only those whose sys ID is in the given set.
func filterProductsBySysIDs(
	products []*costproductmaster.CostProductMaster,
	sysIDs []int64,
) []*costproductmaster.CostProductMaster {
	if len(sysIDs) == 0 {
		return nil
	}
	allowed := make(map[int64]bool, len(sysIDs))
	for _, id := range sysIDs {
		allowed[id] = true
	}
	filtered := products[:0]
	for _, p := range products {
		if allowed[p.ProductSysID()] {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// generateExcel builds the 6-sheet export Excel in memory.
// All cross-sheet references use product_code as the universal key so the file
// can be re-imported without modification.
func (h *ExportHandler) generateExcel(data *exportData) ([]byte, error) {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			_ = err
		}
	}()

	sysIDToCode, headIDToCode, codeToLegacyID := buildExportCodeMaps(data)
	filteredCPP := filterCPPByProductCode(data.cppRows, codeToLegacyID)
	filteredCAPP := filterCAPPByProductCode(data.cappRows, codeToLegacyID)

	if err := writeProductMasterSheet(f, data.products, data.typeIDToCode); err != nil {
		return nil, err
	}
	if err := writeCPPSheets(f, filteredCPP, codeToLegacyID); err != nil {
		return nil, err
	}
	if err := writeCAPPSheets(f, filteredCAPP, codeToLegacyID); err != nil {
		return nil, err
	}
	if err := writeRouteHeadSheet(f, data.heads, sysIDToCode); err != nil {
		return nil, err
	}
	if err := writeRouteSeqSheet(f, data.seqs, headIDToCode, sysIDToCode); err != nil {
		return nil, err
	}
	if err := writeRouteRMSheet(f, data.rms, headIDToCode, sysIDToCode); err != nil {
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

// flex02OrCode returns the Oracle legacy sys_id if set, otherwise falls back to the
// auto-generated product code. Used as the consistent cross-sheet key in exports.
func flex02OrCode(p *costproductmaster.CostProductMaster) string {
	if v := p.Flex02(); v != "" {
		return v
	}
	return p.ProductCode()
}

// buildExportCodeMaps constructs product-code lookup maps from the export data set.
// codeToLegacyID maps each product's auto-generated ProductCode → its flex02OrCode key,
// so parameter sheets can translate stored product codes to the correct legacy_oracle_sys_id.
func buildExportCodeMaps(data *exportData) (sysIDToCode map[int64]string, headIDToCode map[int64]string, codeToLegacyID map[string]string) {
	sysIDToCode = make(map[int64]string, len(data.products))
	codeToLegacyID = make(map[string]string, len(data.products))
	for _, p := range data.products {
		legacyKey := flex02OrCode(p)
		sysIDToCode[p.ProductSysID()] = legacyKey
		codeToLegacyID[p.ProductCode()] = legacyKey
	}
	headIDToCode = make(map[int64]string, len(data.heads))
	for _, hd := range data.heads {
		headIDToCode[hd.HeadID] = sysIDToCode[hd.ProductSysID]
	}
	return sysIDToCode, headIDToCode, codeToLegacyID
}

// filterCPPByProductCode returns only the CPP rows whose product code is in codeToLegacyID.
func filterCPPByProductCode(rows []costproductparameter.CPPRow, codeToLegacyID map[string]string) []costproductparameter.CPPRow {
	out := make([]costproductparameter.CPPRow, 0, len(rows))
	for _, r := range rows {
		if _, ok := codeToLegacyID[r.ProductCode]; ok {
			out = append(out, r)
		}
	}
	return out
}

// filterCAPPByProductCode returns only the CAPP rows whose product code is in codeToLegacyID.
func filterCAPPByProductCode(rows []costproductparameter.CAPPRow, codeToLegacyID map[string]string) []costproductparameter.CAPPRow {
	out := make([]costproductparameter.CAPPRow, 0, len(rows))
	for _, r := range rows {
		if _, ok := codeToLegacyID[r.ProductCode]; ok {
			out = append(out, r)
		}
	}
	return out
}

// updateExportJob persists job state and logs on failure without propagating.
func (h *ExportHandler) updateExportJob(ctx context.Context, jobID int64, job *costimportjob.CostImportJob) {
	if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
		h.logger.Warn().Err(updateErr).Int64("job_id", jobID).Msg("bulk export: failed to persist job state")
	}
}

// writeProductMasterSheet writes the product_master sheet.
// Uses flex02OrCode as the legacy_oracle_sys_id so all sheets share the same key.
func writeProductMasterSheet(f *excelize.File, products []*costproductmaster.CostProductMaster, typeIDToCode map[int32]string) error {
	const sheetName = "product_master"
	if _, err := f.NewSheet(sheetName); err != nil {
		return fmt.Errorf("create sheet %s: %w", sheetName, err)
	}
	headers := []string{
		"legacy_oracle_sys_id", "product_type_code", "product_name",
		"shade_code", "shade_name", "grade_code",
		"description", "erp_item_code", "legacy_erp_compound_key", "legacy_type_label", "is_active",
	}
	if err := writeSheetHeaders(f, sheetName, headers); err != nil {
		return err
	}
	for rowIdx, p := range products {
		vals := []any{
			flex02OrCode(p), typeIDToCode[p.ProductTypeID()], p.ProductName(),
			p.ShadeCode(), p.ShadeName(), p.GradeCode(),
			p.Description(), p.ErpItemCode(), p.Flex01(), p.Flex03(), p.IsActive(),
		}
		if err := writeSheetRow(f, sheetName, rowIdx+2, vals); err != nil {
			return err
		}
	}
	return nil
}

// maxRowsPerSheet is the maximum data rows written to a single Excel sheet.
// Excel supports ~1,048,576 rows total; leaving 100k buffer for safety.
const maxRowsPerSheet = 900_000

// writeCPPSheets writes product_parameters data, splitting into multiple numbered
// part-sheets ("product_parameters_p1", "_p2", …) when rows exceed maxRowsPerSheet.
// The import ParseSheet fuzzy-matching merges them back automatically.
func writeCPPSheets(f *excelize.File, rows []costproductparameter.CPPRow, codeToLegacyID map[string]string) error {
	const base = "product_parameters"
	headers := []string{"legacy_oracle_sys_id", "param_code", "data_type", "value_numeric", "value_text", "value_flag"}

	parts := splitCPPRows(rows, maxRowsPerSheet)
	for i, part := range parts {
		name := partSheetName(base, i+1, len(parts))
		if _, err := f.NewSheet(name); err != nil {
			return fmt.Errorf("create sheet %s: %w", name, err)
		}
		if err := writeSheetHeaders(f, name, headers); err != nil {
			return err
		}
		for rowIdx, r := range part {
			legacyID := codeToLegacyID[r.ProductCode]
			if legacyID == "" {
				legacyID = r.ProductCode
			}
			vals := []any{legacyID, r.ParamCode, cppDataType(r), ptrStringOrEmpty(r.ValueNumeric), ptrStringOrEmpty(r.ValueText), ptrBoolOrEmpty(r.ValueFlag)}
			if err := writeSheetRow(f, name, rowIdx+2, vals); err != nil {
				return err
			}
		}
	}
	return nil
}

// cppDataType returns the data type label for a CPP row based on which value field is set.
func cppDataType(r costproductparameter.CPPRow) string {
	switch {
	case r.ValueNumeric != nil:
		return "NUMERIC"
	case r.ValueText != nil:
		return "TEXT"
	case r.ValueFlag != nil:
		return "FLAG"
	default:
		return ""
	}
}

// writeCAPPSheets writes product_applicable_params data, splitting into part-sheets
// when rows exceed maxRowsPerSheet.
func writeCAPPSheets(f *excelize.File, rows []costproductparameter.CAPPRow, codeToLegacyID map[string]string) error {
	const base = "product_applicable_params"
	headers := []string{"legacy_oracle_sys_id", "param_code", "is_required", "display_order"}

	parts := splitCAPPRows(rows, maxRowsPerSheet)
	for i, part := range parts {
		name := partSheetName(base, i+1, len(parts))
		if _, err := f.NewSheet(name); err != nil {
			return fmt.Errorf("create sheet %s: %w", name, err)
		}
		if err := writeSheetHeaders(f, name, headers); err != nil {
			return err
		}
		for rowIdx, r := range part {
			legacyID := codeToLegacyID[r.ProductCode]
			if legacyID == "" {
				legacyID = r.ProductCode
			}
			vals := []any{legacyID, r.ParamCode, r.IsRequired, ptrInt32OrEmpty(r.DisplayOrder)}
			if err := writeSheetRow(f, name, rowIdx+2, vals); err != nil {
				return err
			}
		}
	}
	return nil
}

// partSheetName returns the sheet name for part i of total.
// If total == 1, returns base (no suffix) so single-sheet exports keep their exact name.
func partSheetName(base string, i, total int) string {
	if total == 1 {
		return base
	}
	return fmt.Sprintf("%s_p%d", base, i)
}

func splitCPPRows(rows []costproductparameter.CPPRow, size int) [][]costproductparameter.CPPRow {
	if len(rows) <= size {
		return [][]costproductparameter.CPPRow{rows}
	}
	var parts [][]costproductparameter.CPPRow
	for start := 0; start < len(rows); start += size {
		end := min(start+size, len(rows))
		parts = append(parts, rows[start:end])
	}
	return parts
}

func splitCAPPRows(rows []costproductparameter.CAPPRow, size int) [][]costproductparameter.CAPPRow {
	if len(rows) <= size {
		return [][]costproductparameter.CAPPRow{rows}
	}
	var parts [][]costproductparameter.CAPPRow
	for start := 0; start < len(rows); start += size {
		end := min(start+size, len(rows))
		parts = append(parts, rows[start:end])
	}
	return parts
}

// writeRouteHeadSheet writes the route_head sheet.
// Uses product_code (from sysIDToCode) as legacy_oracle_sys_id.
func writeRouteHeadSheet(f *excelize.File, heads []costroute.ExportRouteHead, sysIDToCode map[int64]string) error {
	const sheetName = "route_head"
	if _, err := f.NewSheet(sheetName); err != nil {
		return fmt.Errorf("create sheet %s: %w", sheetName, err)
	}
	headers := []string{"legacy_oracle_sys_id", "routing_status", "notes"}
	if err := writeSheetHeaders(f, sheetName, headers); err != nil {
		return err
	}
	for rowIdx, h := range heads {
		productCode := sysIDToCode[h.ProductSysID]
		if productCode == "" {
			productCode = strconv.FormatInt(h.ProductSysID, 10)
		}
		vals := []any{productCode, h.RoutingStatus, h.Notes}
		if err := writeSheetRow(f, sheetName, rowIdx+2, vals); err != nil {
			return err
		}
	}
	return nil
}

// writeRouteSeqSheet writes the route_sequences sheet.
// Uses product_code for both route_head_legacy_product_id and node_product_legacy_id.
func writeRouteSeqSheet(f *excelize.File, seqs []costroute.ExportRouteSeq, headIDToCode, sysIDToCode map[int64]string) error {
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
		headCode := headIDToCode[s.HeadID]
		if headCode == "" {
			headCode = strconv.FormatInt(s.HeadID, 10)
		}
		nodeCode := sysIDToCode[s.ProductSysID]
		if nodeCode == "" {
			nodeCode = strconv.FormatInt(s.ProductSysID, 10)
		}
		vals := []any{
			headCode, nodeCode,
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
// Uses product_code for route_head_legacy_product_id and rm_product_legacy_id.
func writeRouteRMSheet(f *excelize.File, rms []costroute.ExportRouteRM, headIDToCode, sysIDToCode map[int64]string) error {
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
		headCode := headIDToCode[rm.HeadID]
		if headCode == "" {
			headCode = strconv.FormatInt(rm.HeadID, 10)
		}
		rmProductCode := ""
		if rm.RmProductSysID != 0 {
			rmProductCode = sysIDToCode[rm.RmProductSysID]
			if rmProductCode == "" {
				rmProductCode = strconv.FormatInt(rm.RmProductSysID, 10)
			}
		}
		vals := []any{
			headCode, rm.RouteLevel, rm.RouteSeq,
			rm.RmType, rm.Ratio, rmProductCode,
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
