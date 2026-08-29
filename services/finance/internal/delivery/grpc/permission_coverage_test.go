package grpc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// ---------------------------------------------------------------------------
// K-34 (c) — service-wide permission-coverage ratchet.
//
// WHAT THIS IS. PermissionInterceptor treats an empty required permission as
// "any authenticated user may call this" (auth_interceptor.go, the
// `if required == "" { return handler(ctx, req) }` branch). getRequiredPermission
// returns "" both for methods deliberately mapped to "" AND for methods missing
// from the map entirely — so every RPC nobody remembered to map is silently
// reachable by any logged-in user. That is a lot of RPCs today.
//
// WHAT THIS IS NOT. This file fixes NOTHING. It does not add a single permission
// entry. It is a brake, not a patch: the current holes are frozen into a
// baseline, and the test fails the moment the hole count grows. Closing the
// holes is separate, reviewed production work.
//
// WHY ENUMERATED, NOT HAND-LISTED. The RPC list is derived from the GENERATED
// *_ServiceDesc values in gen/finance/v1. A new RPC added tomorrow is seen by
// this test automatically — nobody has to remember to update a list here. A
// hand-maintained list would defeat the entire point.
// ---------------------------------------------------------------------------

// registeredServiceDescs returns the generated descriptors for exactly those
// services actually wired into the running server in cmd/server/main.go
// (the RegisterXxxServiceServer block). Keep this in sync with that block.
//
// ⛔ DELIBERATELY EXCLUDED — CstRoutingService, ProductService and
// ProductTypeService. They exist in gen/finance/v1 but are NEVER registered in
// cmd/server/main.go, so their 17 RPCs are unreachable dead code: no request can
// ever reach the interceptor for them. Including them would manufacture 17
// bogus failures about code that cannot be called. If any of the three is ever
// registered in main.go, add its descriptor here — the test will then correctly
// demand permissions for its RPCs.
func registeredServiceDescs() []grpc.ServiceDesc {
	return []grpc.ServiceDesc{
		financev1.UOMService_ServiceDesc,
		financev1.RMCategoryService_ServiceDesc,
		financev1.ParameterService_ServiceDesc,
		financev1.FormulaService_ServiceDesc,
		financev1.UOMCategoryService_ServiceDesc,
		financev1.BoxBobbinCostService_ServiceDesc,
		financev1.MBHeadService_ServiceDesc,
		financev1.MBSpinService_ServiceDesc,
		financev1.MbCompositionService_ServiceDesc,
		financev1.MbParamService_ServiceDesc,
		financev1.MbLustureService_ServiceDesc,
		financev1.MbCrossSectionService_ServiceDesc,
		financev1.MbCrossSectionFactorService_ServiceDesc,
		financev1.MBDozingService_ServiceDesc,
		financev1.MbWorkflowLogService_ServiceDesc,
		financev1.MbPushService_ServiceDesc,
		financev1.MbBatchService_ServiceDesc,
		financev1.MachineService_ServiceDesc,
		financev1.InterminglingService_ServiceDesc,
		financev1.SpinFixedCostService_ServiceDesc,
		financev1.ProductGradeService_ServiceDesc,
		financev1.LookupMasterService_ServiceDesc,
		financev1.YarnLookupFillService_ServiceDesc,
		financev1.OracleSyncService_ServiceDesc,
		financev1.RMGroupService_ServiceDesc,
		financev1.RMCostService_ServiceDesc,
		financev1.CostProductTypeService_ServiceDesc,
		financev1.CostRmTypeService_ServiceDesc,
		financev1.CostErpLookupService_ServiceDesc,
		financev1.CostProductMasterService_ServiceDesc,
		financev1.CostRouteService_ServiceDesc,
		financev1.CostMasterLookupService_ServiceDesc,
		financev1.CostRequestTypeService_ServiceDesc,
		financev1.CostPaperTubeTypeService_ServiceDesc,
		financev1.CostProductRequestService_ServiceDesc,
		financev1.CostRequestCommentService_ServiceDesc,
		financev1.CostAttachmentService_ServiceDesc,
		financev1.CostRoutingRuleService_ServiceDesc,
		financev1.CostAuditLogService_ServiceDesc,
		financev1.CostNotificationService_ServiceDesc,
		financev1.CostProductParameterService_ServiceDesc,
		financev1.CostDataImportService_ServiceDesc,
		financev1.CostCalcService_ServiceDesc,
		financev1.CostLevelAssignmentConfigService_ServiceDesc,
		financev1.CostFillTaskService_ServiceDesc,
		financev1.DashboardService_ServiceDesc,
		financev1.ChartDataService_ServiceDesc,
		financev1.DataSourceService_ServiceDesc,
		financev1.BiJobService_ServiceDesc,
		financev1.BiUploadService_ServiceDesc,
		// ShadeService was already registered in cmd/server/main.go
		// (RegisterShadeServiceServer) but was missing from this list, so its 6
		// RPCs were invisible to every test in this file — they neither counted
		// as reachable nor were flagged as unmapped/fail-open. Added when the
		// Shade permission mapping was wired into getRequiredPermission.
		financev1.ShadeService_ServiceDesc,
	}
}

// permissionMapLiteral parses the `permissions := map[string]string{...}` literal
// inside getRequiredPermission directly from auth_interceptor.go source.
//
// ⚠ WHY SOURCE PARSING IS NECESSARY. getRequiredPermission alone CANNOT tell the
// two fail-open causes apart: a key mapped to "" and a key that is absent both
// return "" from a map index. Calling the function therefore cannot distinguish
// "deliberately authenticated-only" from "somebody forgot". Since the map is an
// unexported local literal with no accessor, the only way to see the difference
// without touching production code is to read the literal from source — which is
// what this does. It is the least-bad option available to a test that is not
// permitted to modify auth_interceptor.go.
//
// If the map is ever refactored (e.g. hoisted to a package-level var), this
// helper fails loudly rather than silently returning an empty map.
func permissionMapLiteral(t *testing.T) map[string]string {
	t.Helper()

	const srcFile = "auth_interceptor.go"

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, srcFile, nil, 0)
	require.NoError(t, err, "cannot parse %s", srcFile)

	out := map[string]string{}

	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		mapType, ok := lit.Type.(*ast.MapType)
		if !ok {
			return true
		}
		keyIdent, ok := mapType.Key.(*ast.Ident)
		if !ok || keyIdent.Name != "string" {
			return true
		}
		valIdent, ok := mapType.Value.(*ast.Ident)
		if !ok || valIdent.Name != "string" {
			return true
		}

		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			k, kok := kv.Key.(*ast.BasicLit)
			v, vok := kv.Value.(*ast.BasicLit)
			if !kok || !vok || k.Kind != token.STRING || v.Kind != token.STRING {
				continue
			}
			key := unquoteGoString(k.Value)
			if len(key) == 0 || key[0] != '/' {
				continue
			}
			out[key] = unquoteGoString(v.Value)
		}
		return true
	})

	require.NotEmpty(t, out,
		"no map[string]string literal with %q-style keys found in %s — was getRequiredPermission refactored? "+
			"This test must be updated to read the permission map from its new home.", "/finance.v1.X/Y", srcFile)

	return out
}

// unquoteGoString strips the surrounding quotes from a Go string literal token.
// The permission map contains only plain, escape-free literals, so trimming the
// delimiters is sufficient and avoids pulling in strconv error handling.
func unquoteGoString(lit string) string {
	if len(lit) >= 2 {
		return lit[1 : len(lit)-1]
	}
	return lit
}

// genMethods returns every FullMethod reachable through a registered service,
// derived from the generated descriptors.
func genMethods(t *testing.T) map[string]struct{} {
	t.Helper()

	out := map[string]struct{}{}
	for _, desc := range registeredServiceDescs() {
		require.NotEmpty(t, desc.Methods, "descriptor %s exposes no methods", desc.ServiceName)
		for _, m := range desc.Methods {
			out["/"+desc.ServiceName+"/"+m.MethodName] = struct{}{}
		}
		for _, s := range desc.Streams {
			out["/"+desc.ServiceName+"/"+s.StreamName] = struct{}{}
		}
	}
	return out
}

// allGenMethodsIncludingUnregistered returns every FullMethod defined in
// gen/finance/v1, registered or not. Used only by the stale-key assertion: a map
// key pointing at an unregistered service is a different (milder) problem than a
// key pointing at a method that does not exist at all.
func allGenMethodsIncludingUnregistered() map[string]struct{} {
	descs := append(registeredServiceDescs(),
		financev1.CstRoutingService_ServiceDesc,
		financev1.ProductService_ServiceDesc,
		financev1.ProductTypeService_ServiceDesc,
	)

	out := map[string]struct{}{}
	for _, desc := range descs {
		for _, m := range desc.Methods {
			out["/"+desc.ServiceName+"/"+m.MethodName] = struct{}{}
		}
		for _, s := range desc.Streams {
			out["/"+desc.ServiceName+"/"+s.StreamName] = struct{}{}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// intentionallyAuthenticatedOnly — the 7 RPCs mapped to "" ON PURPOSE.
//
// These are NOT oversights and must NOT be counted as debt. They are mapped to
// the empty string explicitly in auth_interceptor.go, right next to comments
// recording the design intent: authentication is the gate, and the finer-grained
// decision (may THIS user cancel/close this request, may THIS user claim this
// fill task) is made downstream by the handler against the request's own
// state/ownership — not by a static permission code. Encoding them as a coarse
// permission would either over-restrict the legitimate actor or under-restrict
// everyone else.
//
// Removing an entry here without also removing the "" mapping in
// auth_interceptor.go (or vice-versa) makes TestIntentionalFailOpenSetIsAccurate
// fail — the two lists are cross-checked against the real source.
// ---------------------------------------------------------------------------
var intentionallyAuthenticatedOnly = map[string]struct{}{
	// CostProductRequestService — the request's own workflow state and the
	// caller's relationship to it decide these, not a permission code.
	"/finance.v1.CostProductRequestService/CancelCostProductRequest": {},
	"/finance.v1.CostProductRequestService/CloseCostProductRequest":  {},

	// CostFillTaskService — fill tasks are assignment-scoped: the handler checks
	// that the caller is the assignee/approver for the specific task.
	"/finance.v1.CostFillTaskService/ListFillTasks":   {},
	"/finance.v1.CostFillTaskService/ClaimFillTask":   {},
	"/finance.v1.CostFillTaskService/SubmitFillTask":  {},
	"/finance.v1.CostFillTaskService/ApproveFillTask": {},
	"/finance.v1.CostFillTaskService/RejectFillTask":  {},
}

// ---------------------------------------------------------------------------
// knownFailOpen — ⚠ DEBT THAT MUST SHRINK. NOT A PERMANENT ALLOW-LIST. ⚠
//
// Every entry below is an RPC on a REGISTERED service that has NO entry in the
// permission map, and therefore fails OPEN: any authenticated user, with zero
// permissions, can call it today. 237 entries as of this file's creation
// (391 RPCs in gen − 17 on unregistered services = 374 reachable;
//
//	374 − 130 properly guarded − 7 intentional "" = 237).
//
// RULES:
//   - Nothing may be ADDED here. A new fail-open RPC must turn this test red;
//     that is the whole point of K-34 (c).
//   - Entries must be DELETED as holes are closed. TestKnownFailOpenBaselineIsStillAccurate
//     turns red when an entry here has since been given a real permission, forcing
//     the line to be removed. Without that second direction the baseline would rot
//     into a list of comfortable lies.
//   - The correct long-term size of this map is ZERO.
//
// ---------------------------------------------------------------------------
var knownFailOpen = map[string]struct{}{
	"/finance.v1.BiJobService/CreateBiJob":                                      {},
	"/finance.v1.BiJobService/DeleteBiJob":                                      {},
	"/finance.v1.BiJobService/ListJobLogs":                                      {},
	"/finance.v1.BiJobService/ListJobs":                                         {},
	"/finance.v1.BiJobService/TriggerJob":                                       {},
	"/finance.v1.BiJobService/UpdateBiJob":                                      {},
	"/finance.v1.BiUploadService/CancelUpload":                                  {},
	"/finance.v1.BiUploadService/CommitUpload":                                  {},
	"/finance.v1.BiUploadService/DownloadUploadTemplate":                        {},
	"/finance.v1.BiUploadService/ListUploads":                                   {},
	"/finance.v1.BiUploadService/ParseUpload":                                   {},
	"/finance.v1.BoxBobbinCostService/CreateBoxBobbinCost":                      {},
	"/finance.v1.BoxBobbinCostService/CreateBoxBobbinCostRate":                  {},
	"/finance.v1.BoxBobbinCostService/DeleteBoxBobbinCost":                      {},
	"/finance.v1.BoxBobbinCostService/DeleteBoxBobbinCostRate":                  {},
	"/finance.v1.BoxBobbinCostService/DownloadBoxBobbinCostTemplate":            {},
	"/finance.v1.BoxBobbinCostService/ExportBoxBobbinCosts":                     {},
	"/finance.v1.BoxBobbinCostService/GetBoxBobbinCost":                         {},
	"/finance.v1.BoxBobbinCostService/ImportBoxBobbinCosts":                     {},
	"/finance.v1.BoxBobbinCostService/ListBoxBobbinCosts":                       {},
	"/finance.v1.BoxBobbinCostService/UpdateBoxBobbinCost":                      {},
	"/finance.v1.ChartDataService/GetDashboardData":                             {},
	"/finance.v1.ChartDataService/PreviewDashboard":                             {},
	"/finance.v1.CostAttachmentService/DeleteCostAttachment":                    {},
	"/finance.v1.CostAttachmentService/GetCostAttachmentDownloadURL":            {},
	"/finance.v1.CostAttachmentService/ListCostAttachmentsByComment":            {},
	"/finance.v1.CostAttachmentService/ListCostAttachmentsByRequest":            {},
	"/finance.v1.CostAttachmentService/UploadCostAttachment":                    {},
	"/finance.v1.CostAuditLogService/ListCostAuditLogs":                         {},
	"/finance.v1.CostCalcService/DownloadExportBatchZip":                        {},
	"/finance.v1.CostCalcService/GetBatchChildDownloadUrl":                      {},
	"/finance.v1.CostCalcService/GetProductCostSheetDownloadURL":                {},
	"/finance.v1.CostCalcService/GetProductCostSheetExportJobStatus":            {},
	"/finance.v1.CostCalcService/GetRouteCostSheet":                             {},
	"/finance.v1.CostCalcService/ListCostResultPeriods":                         {},
	"/finance.v1.CostCalcService/ListCostResults":                               {},
	"/finance.v1.CostCalcService/ListCostSheetExportBatchChildren":              {},
	"/finance.v1.CostCalcService/ListExportJobs":                                {},
	"/finance.v1.CostCalcService/RequestProductCostSheetExport":                 {},
	"/finance.v1.CostDataImportService/DownloadCostApplicableParamTemplate":     {},
	"/finance.v1.CostDataImportService/DownloadCostProductParameterTemplate":    {},
	"/finance.v1.CostDataImportService/ExportBulkProductRouting":                {},
	"/finance.v1.CostDataImportService/ExportCostApplicableParams":              {},
	"/finance.v1.CostDataImportService/ExportCostProductParameters":             {},
	"/finance.v1.CostDataImportService/GetCostImportJob":                        {},
	"/finance.v1.CostDataImportService/GetImportUploadURL":                      {},
	"/finance.v1.CostDataImportService/ImportBulkParamsOnly":                    {},
	"/finance.v1.CostDataImportService/ImportBulkProductRouting":                {},
	"/finance.v1.CostDataImportService/ImportCostApplicableParams":              {},
	"/finance.v1.CostDataImportService/ImportCostProductParameters":             {},
	"/finance.v1.CostDataImportService/ListCostImportJobs":                      {},
	"/finance.v1.CostDataImportService/StartCostingImport":                      {},
	"/finance.v1.CostDataImportService/ValidateBulkProductRoutingFile":          {},
	"/finance.v1.CostErpLookupService/GetCostErpItem":                           {},
	"/finance.v1.CostErpLookupService/ListCostErpGrades":                        {},
	"/finance.v1.CostErpLookupService/ListCostErpItems":                         {},
	"/finance.v1.CostErpLookupService/ListCostErpShades":                        {},
	"/finance.v1.CostMasterLookupService/BatchGetCostProductMaster":             {},
	"/finance.v1.CostMasterLookupService/BatchGetProductParameterValues":        {},
	"/finance.v1.CostMasterLookupService/GetCostProductMasterForPPC":            {},
	"/finance.v1.CostMasterLookupService/GetProductRouteForPPC":                 {},
	"/finance.v1.CostMasterLookupService/ListCostProductMasterForPPC":           {},
	"/finance.v1.CostMasterLookupService/ListProductGradesForPPC":               {},
	"/finance.v1.CostMasterLookupService/ListProductParametersForPPC":           {},
	"/finance.v1.CostMasterLookupService/ResolveCostProductMasterByErpCode":     {},
	"/finance.v1.CostNotificationService/GetMyCostNotificationUnreadCount":      {},
	"/finance.v1.CostNotificationService/ListMyCostNotifications":               {},
	"/finance.v1.CostNotificationService/MarkAllMyCostNotificationsRead":        {},
	"/finance.v1.CostNotificationService/MarkCostNotificationRead":              {},
	"/finance.v1.CostPaperTubeTypeService/ListCostPaperTubeTypes":               {},
	"/finance.v1.CostProductParameterService/AddApplicableParam":                {},
	"/finance.v1.CostProductParameterService/AddApplicableParamWithChildren":    {},
	"/finance.v1.CostProductParameterService/CheckMissingRequiredParams":        {},
	"/finance.v1.CostProductParameterService/DeleteProductParamValue":           {},
	"/finance.v1.CostProductParameterService/GetRemoveApplicablePreview":        {},
	"/finance.v1.CostProductParameterService/ListAvailableParams":               {},
	"/finance.v1.CostProductParameterService/ListParamEditLog":                  {},
	"/finance.v1.CostProductParameterService/ListProductRequiredParams":         {},
	"/finance.v1.CostProductParameterService/OverrideParamValues":               {},
	"/finance.v1.CostProductParameterService/RemoveApplicableParam":             {},
	"/finance.v1.CostProductParameterService/RemoveApplicableParamWithChildren": {},
	"/finance.v1.CostProductParameterService/UpdateApplicableParam":             {},
	"/finance.v1.CostProductParameterService/UpsertProductParamValue":           {},
	"/finance.v1.CostProductParameterService/UpsertProductParamValuesBatch":     {},
	"/finance.v1.CostProductRequestService/GetParamSummary":                     {},
	"/finance.v1.CostProductRequestService/MarkParameterPending":                {},
	"/finance.v1.CostProductTypeService/CreateCostProductType":                  {},
	"/finance.v1.CostProductTypeService/DownloadCostProductTypeTemplate":        {},
	"/finance.v1.CostProductTypeService/ExportCostProductTypes":                 {},
	"/finance.v1.CostProductTypeService/GetCostProductType":                     {},
	"/finance.v1.CostProductTypeService/ImportCostProductTypes":                 {},
	"/finance.v1.CostProductTypeService/ListCostProductTypes":                   {},
	"/finance.v1.CostProductTypeService/UpdateCostProductType":                  {},
	"/finance.v1.CostRequestCommentService/CreateCostRequestComment":            {},
	"/finance.v1.CostRequestCommentService/DeleteCostRequestComment":            {},
	"/finance.v1.CostRequestCommentService/HideCostRequestComment":              {},
	"/finance.v1.CostRequestCommentService/ListCostCommentEditHistory":          {},
	"/finance.v1.CostRequestCommentService/ListCostRequestComments":             {},
	"/finance.v1.CostRequestCommentService/UnhideCostRequestComment":            {},
	"/finance.v1.CostRequestCommentService/UpdateCostRequestComment":            {},
	"/finance.v1.CostRequestTypeService/ListCostRequestTypes":                   {},
	"/finance.v1.CostRmTypeService/CreateCostRmType":                            {},
	"/finance.v1.CostRmTypeService/GetCostRmType":                               {},
	"/finance.v1.CostRmTypeService/ListCostRmTypes":                             {},
	"/finance.v1.CostRmTypeService/UpdateCostRmType":                            {},
	"/finance.v1.CostRoutingRuleService/CreateCostRoutingRule":                  {},
	"/finance.v1.CostRoutingRuleService/DeleteCostRoutingRule":                  {},
	"/finance.v1.CostRoutingRuleService/GetCostRoutingRule":                     {},
	"/finance.v1.CostRoutingRuleService/ListCostRoutingRules":                   {},
	"/finance.v1.CostRoutingRuleService/UpdateCostRoutingRule":                  {},
	"/finance.v1.DashboardService/CreateDashboard":                              {},
	"/finance.v1.DashboardService/CreateDashboardGroup":                         {},
	"/finance.v1.DashboardService/DeleteDashboard":                              {},
	"/finance.v1.DashboardService/DeleteDashboardGroup":                         {},
	"/finance.v1.DashboardService/DuplicateDashboard":                           {},
	"/finance.v1.DashboardService/GetDashboard":                                 {},
	"/finance.v1.DashboardService/GetDashboardByCode":                           {},
	"/finance.v1.DashboardService/ListAccessibleDashboards":                     {},
	"/finance.v1.DashboardService/ListConfigAudit":                              {},
	"/finance.v1.DashboardService/ListDashboardGroups":                          {},
	"/finance.v1.DashboardService/ListDashboards":                               {},
	"/finance.v1.DashboardService/ListFeaturedDashboards":                       {},
	"/finance.v1.DashboardService/SetDashboardRoles":                            {},
	"/finance.v1.DashboardService/UpdateDashboard":                              {},
	"/finance.v1.DashboardService/UpdateDashboardGroup":                         {},
	"/finance.v1.DataSourceService/GetFactDistincts":                            {},
	"/finance.v1.DataSourceService/ListDataSources":                             {},
	"/finance.v1.FormulaService/CreateFormula":                                  {},
	"/finance.v1.FormulaService/DownloadFormulaTemplate":                        {},
	"/finance.v1.FormulaService/ExportFormulas":                                 {},
	"/finance.v1.FormulaService/GetFormula":                                     {},
	"/finance.v1.FormulaService/ImportFormulas":                                 {},
	"/finance.v1.FormulaService/ListFormulas":                                   {},
	"/finance.v1.InterminglingService/CreateIntermingling":                      {},
	"/finance.v1.InterminglingService/DeleteIntermingling":                      {},
	"/finance.v1.InterminglingService/DownloadInterminglingTemplate":            {},
	"/finance.v1.InterminglingService/ExportInterminglings":                     {},
	"/finance.v1.InterminglingService/GetIntermingling":                         {},
	"/finance.v1.InterminglingService/ImportInterminglings":                     {},
	"/finance.v1.InterminglingService/ListInterminglings":                       {},
	"/finance.v1.InterminglingService/UpdateIntermingling":                      {},
	"/finance.v1.LookupMasterService/CreateLookupMaster":                        {},
	"/finance.v1.LookupMasterService/CreateLookupMasterColumn":                  {},
	"/finance.v1.LookupMasterService/DeleteLookupMasterColumn":                  {},
	"/finance.v1.LookupMasterService/ExportLookupMasters":                       {},
	"/finance.v1.LookupMasterService/ImportLookupMasters":                       {},
	"/finance.v1.LookupMasterService/ListLookupMasterColumns":                   {},
	"/finance.v1.LookupMasterService/ListLookupMasters":                         {},
	"/finance.v1.LookupMasterService/ListMasterOptions":                         {},
	"/finance.v1.LookupMasterService/ListTableColumns":                          {},
	"/finance.v1.MachineService/CreateMachine":                                  {},
	"/finance.v1.MachineService/DownloadMachineTemplate":                        {},
	"/finance.v1.MachineService/ExportMachines":                                 {},
	"/finance.v1.MachineService/GetMachine":                                     {},
	"/finance.v1.MachineService/ImportMachines":                                 {},
	"/finance.v1.MachineService/ListMachines":                                   {},
	"/finance.v1.MbLustureService/DownloadMbLustureTemplate":                    {},
	"/finance.v1.MbLustureService/ExportMbLusture":                              {},
	"/finance.v1.MbLustureService/ImportMbLusture":                              {},
	"/finance.v1.MbParamService/DownloadMbParamTemplate":                        {},
	"/finance.v1.MbParamService/ExportMbParams":                                 {},
	"/finance.v1.MbParamService/ImportMbParams":                                 {},
	"/finance.v1.OracleSyncService/CancelSyncJob":                               {},
	"/finance.v1.OracleSyncService/GetSyncJob":                                  {},
	"/finance.v1.OracleSyncService/ListItemConsStockPO":                         {},
	"/finance.v1.OracleSyncService/ListSyncJobs":                                {},
	"/finance.v1.OracleSyncService/ListSyncPeriods":                             {},
	"/finance.v1.OracleSyncService/TriggerSync":                                 {},
	"/finance.v1.ParameterService/CreateParameter":                              {},
	"/finance.v1.ParameterService/DownloadParameterTemplate":                    {},
	"/finance.v1.ParameterService/ExportParameters":                             {},
	"/finance.v1.ParameterService/GetParameter":                                 {},
	"/finance.v1.ParameterService/ImportParameters":                             {},
	"/finance.v1.ParameterService/ListParameters":                               {},
	"/finance.v1.ProductGradeService/CreateProductGrade":                        {},
	"/finance.v1.ProductGradeService/DownloadProductGradeTemplate":              {},
	"/finance.v1.ProductGradeService/ExportProductGrades":                       {},
	"/finance.v1.ProductGradeService/GetProductGrade":                           {},
	"/finance.v1.ProductGradeService/ImportProductGrades":                       {},
	"/finance.v1.ProductGradeService/ListProductGrades":                         {},
	"/finance.v1.RMCostService/ExportRMCosts":                                   {},
	"/finance.v1.RMCostService/GetExportDownloadURL":                            {},
	"/finance.v1.RMCostService/GetRMCost":                                       {},
	"/finance.v1.RMCostService/ListCostDetails":                                 {},
	"/finance.v1.RMCostService/ListRMCostHistory":                               {},
	"/finance.v1.RMCostService/ListRMCostPeriods":                               {},
	"/finance.v1.RMCostService/ListRMCosts":                                     {},
	"/finance.v1.RMCostService/RequestRMCostExport":                             {},
	"/finance.v1.RMCostService/TriggerRMCostCalculation":                        {},
	"/finance.v1.RMCostService/UpdateCostDetailFixRate":                         {},
	"/finance.v1.RMCostService/UpdateRMCostInputs":                              {},
	"/finance.v1.RMGroupService/AddItems":                                       {},
	"/finance.v1.RMGroupService/CreateRMGroup":                                  {},
	"/finance.v1.RMGroupService/DownloadGroupItemsTemplate":                     {},
	"/finance.v1.RMGroupService/DownloadRMGroupTemplate":                        {},
	"/finance.v1.RMGroupService/ExportRMGroups":                                 {},
	"/finance.v1.RMGroupService/ExportUngroupedItems":                           {},
	"/finance.v1.RMGroupService/GetRMGroup":                                     {},
	"/finance.v1.RMGroupService/GetRMGroupItemRates":                            {},
	"/finance.v1.RMGroupService/ImportGroupItems":                               {},
	"/finance.v1.RMGroupService/ImportRMGroups":                                 {},
	"/finance.v1.RMGroupService/ListRMGroups":                                   {},
	"/finance.v1.RMGroupService/ListUngroupedItems":                             {},
	"/finance.v1.RMGroupService/RemoveItems":                                    {},
	"/finance.v1.RMGroupService/UpdateGroupItem":                                {},
	"/finance.v1.RMGroupService/UpdateRMGroup":                                  {},
	// SpinFixedCostService (5 baris di bawah) — SENGAJA dibiarkan fail-open, gerbang K-42 (b).
	// Meski namanya "Spin", ini pool biaya tetap POY (HOY) bulanan (spin_fixed_cost.proto:202-204),
	// BUKAN MB Spin; MB Spin sejati = MBSpinService, sudah 8/8 terjaga (auth_interceptor.go:413-421).
	// Konsekuensi yang diterima: kelima RPC ini dapat dipanggil pengguna terautentikasi mana pun
	// tanpa cek izin. Tidak ditambal karena iam 000082 hanya memberi finance.master.spinfixedcost.*
	// ke SUPER_ADMIN, sehingga menambal = mencabut seluruh layar dari non-SUPER_ADMIN (regresi
	// fungsional langsung). Ini UTANG YANG DITERIMA, bukan lubang yang belum ditemukan — penutupan
	// menunggu keputusan lingkup + seed izin peran non-SUPER_ADMIN, di luar lingkup MB.
	"/finance.v1.SpinFixedCostService/CreateSpinFixedCost":       {},
	"/finance.v1.SpinFixedCostService/DeleteSpinFixedCost":       {},
	"/finance.v1.SpinFixedCostService/GetSpinFixedCost":          {},
	"/finance.v1.SpinFixedCostService/ListSpinFixedCosts":        {},
	"/finance.v1.SpinFixedCostService/UpdateSpinFixedCost":       {},
	"/finance.v1.UOMCategoryService/CreateUOMCategory":           {},
	"/finance.v1.UOMCategoryService/DownloadUOMCategoryTemplate": {},
	"/finance.v1.UOMCategoryService/ExportUOMCategories":         {},
	"/finance.v1.UOMCategoryService/GetUOMCategory":              {},
	"/finance.v1.UOMCategoryService/ImportUOMCategories":         {},
	"/finance.v1.UOMCategoryService/ListUOMCategories":           {},
	"/finance.v1.UOMService/DownloadTemplate":                    {},
	"/finance.v1.YarnLookupFillService/GetLookupFillValues":      {},
}

// TestNoNewFailOpenRPCs is the ratchet. Every reachable RPC must either carry a
// non-empty permission, be one of the 7 deliberate authenticated-only RPCs, or be
// a pre-existing hole recorded in knownFailOpen. Anything else is a NEW hole.
func TestNoNewFailOpenRPCs(t *testing.T) {
	var offenders []string

	for method := range genMethods(t) {
		if getRequiredPermission(method) != "" {
			continue
		}
		if _, ok := intentionallyAuthenticatedOnly[method]; ok {
			continue
		}
		if _, ok := knownFailOpen[method]; ok {
			continue
		}
		offenders = append(offenders, method)
	}

	sort.Strings(offenders)

	assert.Emptyf(t, offenders,
		"%d RPC(s) fail OPEN and are not in the K-34 baseline — any authenticated user can call them:\n  %v\n\n"+
			"FIX: add a permission entry for each in getRequiredPermission (auth_interceptor.go).\n"+
			"Do NOT add them to knownFailOpen — that map only records pre-existing debt and must never grow.\n"+
			"If a method is deliberately authenticated-only, map it to \"\" AND list it in "+
			"intentionallyAuthenticatedOnly with the reason.",
		len(offenders), offenders)
}

// TestKnownFailOpenBaselineIsStillAccurate is the OTHER direction of the ratchet.
// Once a hole is patched, its baseline line becomes a lie — so this test goes red
// and demands the line be deleted. This is what keeps the debt count honest.
func TestKnownFailOpenBaselineIsStillAccurate(t *testing.T) {
	reachable := genMethods(t)

	var patched, unknown []string

	for method := range knownFailOpen {
		if _, ok := reachable[method]; !ok {
			unknown = append(unknown, method)
			continue
		}
		if getRequiredPermission(method) != "" {
			patched = append(patched, method)
		}
	}

	sort.Strings(patched)
	sort.Strings(unknown)

	assert.Emptyf(t, patched,
		"%d baseline entr(ies) now HAVE a permission — the hole is closed:\n  %v\n\n"+
			"FIX: delete these lines from knownFailOpen. The baseline must shrink as holes close; "+
			"leaving stale entries turns it into a list of lies.",
		len(patched), patched)

	assert.Emptyf(t, unknown,
		"%d baseline entr(ies) no longer correspond to any reachable RPC:\n  %v\n\n"+
			"FIX: the RPC was renamed, removed, or its service was unregistered — delete these lines "+
			"from knownFailOpen.",
		len(unknown), unknown)
}

// TestIntentionalFailOpenSetIsAccurate cross-checks the 7-entry intentional set
// against the real map literal, so the two cannot drift apart. It catches both
// "listed here but no longer mapped to \"\"" and "mapped to \"\" in source but
// undocumented here" — the latter being how a genuine oversight would disguise
// itself as intent.
func TestIntentionalFailOpenSetIsAccurate(t *testing.T) {
	literal := permissionMapLiteral(t)

	sourceEmpty := map[string]struct{}{}
	for k, v := range literal {
		if v == "" {
			sourceEmpty[k] = struct{}{}
		}
	}

	var missingFromSource, undocumented []string

	for method := range intentionallyAuthenticatedOnly {
		if _, ok := sourceEmpty[method]; !ok {
			missingFromSource = append(missingFromSource, method)
		}
	}
	for method := range sourceEmpty {
		if _, ok := intentionallyAuthenticatedOnly[method]; !ok {
			undocumented = append(undocumented, method)
		}
	}

	sort.Strings(missingFromSource)
	sort.Strings(undocumented)

	assert.Emptyf(t, missingFromSource,
		"listed as deliberately authenticated-only but NOT mapped to \"\" in auth_interceptor.go:\n  %v\n\n"+
			"FIX: either the mapping changed (drop the entry from intentionallyAuthenticatedOnly) "+
			"or the mapping was lost (restore it).",
		missingFromSource)

	assert.Emptyf(t, undocumented,
		"mapped to \"\" in auth_interceptor.go but NOT documented in intentionallyAuthenticatedOnly:\n  %v\n\n"+
			"FIX: an explicit \"\" is a deliberate fail-open and needs its reason recorded here — "+
			"otherwise a plain oversight is indistinguishable from a design decision.",
		undocumented)
}

// TestPermissionMapHasNoStaleKeys asserts every key in the permission map names a
// method that actually exists in gen/finance/v1.
//
// ⭐ STATUS 2026-08-24 (K-36): GREEN. The finding below has been FIXED; the history
// is kept verbatim so the decision trail stays traceable.
//
// ── RIWAYAT (kondisi saat K-34 memasang rem, sudah TIDAK berlaku lagi) ──────────
// [USANG] ⚠ THIS TEST IS EXPECTED TO FAIL TODAY, AND THE FAILURE IS A REAL FINDING,
// [USANG] NOT A BROKEN TEST. Two keys are stale:
// [USANG]
// [USANG]	"/finance.v1.UOMService/ExportUOM"  — the real method is ExportUOMs (plural)
// [USANG]	"/finance.v1.UOMService/ImportUOM"  — the real method is ImportUOMs (plural)
// [USANG]
// [USANG] Consequence: the guards do nothing, and the ACTUAL ExportUOMs/ImportUOMs
// [USANG] RPCs match no key and therefore fail OPEN — UOM import/export is reachable
// [USANG] by any authenticated user despite looking guarded. (Both plural methods are
// [USANG] in the knownFailOpen baseline above, which is why they do not also trip
// [USANG] TestNoNewFailOpenRPCs.)
// [USANG]
// [USANG] Fixing it means editing the permission map — production code, out of scope
// [USANG] for K-34 (c), which only installs the brake. Left red on purpose:
// [USANG] suppressing it would hide the finding.
// ───────────────────────────────────────────────────────────────────────────────
//
// K-36 corrected both keys to ImportUOMs/ExportUOMs (auth_interceptor.go) and removed
// the two plural methods from knownFailOpen (237 → 235). The permission VALUES were
// deliberately left as finance.master.uom.create/.view — whether they should move to
// the dedicated finance.master.uom.import/.export codes that IAM already seeds is a
// SEPARATE open user decision (K-37), not part of the stale-name fix.
//
// It remains a STANDALONE test so that, if it ever goes red again, it fails alone
// without masking the ratchet tests.
func TestPermissionMapHasNoStaleKeys(t *testing.T) {
	existing := allGenMethodsIncludingUnregistered()

	var stale []string
	for method := range permissionMapLiteral(t) {
		if _, ok := existing[method]; !ok {
			stale = append(stale, method)
		}
	}
	sort.Strings(stale)

	assert.Emptyf(t, stale,
		"%d permission-map key(s) name a method that does not exist in gen/finance/v1:\n  %v\n\n"+
			"A stale key guards nothing, and the REAL method it was meant to guard fails OPEN. "+
			"FIX: correct the key to the generated method name (auth_interceptor.go) and remove the "+
			"real method from knownFailOpen.",
		len(stale), stale)
}

// TestPermissionCoverageCountsAreStable pins the shape of the problem so that a
// large, unreviewed swing in any direction is visible in the diff rather than
// silently absorbed. The debt number is expected to go DOWN over time; when it
// does, update it here in the same commit that removes the baseline entries.
func TestPermissionCoverageCountsAreStable(t *testing.T) {
	reachable := genMethods(t)

	guarded := 0
	for method := range reachable {
		if getRequiredPermission(method) != "" {
			guarded++
		}
	}

	assert.Len(t, registeredServiceDescs(), 51,
		"service count changed — reconcile registeredServiceDescs() with cmd/server/main.go")
	// 374 -> 375: RPC BARU DuplicateMBSpin (P8, gerbang G16 opsi (a)), 392 in gen - 17
	// 375 -> 378: TIGA RPC BARU unlock MBHeadService (P10-b): RequestUnlockMBHead,
	// GrantUnlockMBHead, RejectUnlockMBHead. 395 in gen - 17.
	// 378 -> 384: ShadeService (6 RPCs) was registered in main.go all along but
	// missing from registeredServiceDescs() — added, not a new RPC surface.
	assert.Len(t, reachable, 384,
		"reachable RPC count changed (395 in gen − 17 on the 3 unregistered services + 6 for the "+
			"previously-omitted-from-this-list ShadeService)")
	// 130 -> 132: dua kunci basi UOM (ImportUOM/ExportUOM) dibetulkan jadi ImportUOMs/ExportUOMs, K-36
	// 132 -> 135: tiga bulk RPC CostProductMasterService (Export/Import/DownloadTemplate) dijaga, K-43
	// 135 -> 136: DuplicateMBSpin dijaga finance.yarnmaster.mbspin.create (di-seed iam 000057:47), P8
	// 136 -> 139: tiga RPC unlock P10-b dijaga. Sejak keputusan user memisahkan izin,
	// RequestUnlockMBHead dijaga finance.mb.recipe.unlockrequest (kode BARU, WAJIB
	// di-seed migrasi IAM), sedangkan GrantUnlockMBHead/RejectUnlockMBHead tetap
	// finance.mb.recipe.unlock (sudah di-seed iam 000083:27, sudah dipetakan ke role
	// :107, :160). Pemecahan kode TIDAK mengubah angka: RPC-nya tetap tiga dan tetap
	// terjaga. knownFailOpen TIDAK bertambah: ketiganya terjaga sejak lahir.
	// 139 -> 145: enam RPC ShadeService kini dipetakan ke finance.master.shade.*
	// (di-seed iam migrasi 000089); ShadeService juga baru ditambahkan ke
	// registeredServiceDescs() di atas, jadi ini bukan sekadar pemetaan baru
	// tapi juga RPC yang baru pertama kali terlihat oleh test ini.
	// 145 -> 158: tiga belas RPC Update/Delete dipetakan ke kode izin yang SUDAH
	// di-seed di IAM: UOMCategoryService (Update/Delete), ParameterService
	// (Update/Delete), FormulaService (Update/Delete), MachineService
	// (Update/Delete), LookupMasterService (Update/Delete), ProductGradeService
	// (Update/Delete), RMGroupService (DeleteRMGroup) — 2+2+2+2+2+2+1 = 13.
	assert.Equal(t, 158, guarded, "number of properly guarded RPCs changed")
	assert.Len(t, intentionallyAuthenticatedOnly, 7, "the deliberate authenticated-only set changed")
	// 237 -> 235: dua kunci basi UOM diperbaiki sehingga ImportUOMs/ExportUOMs keluar dari baseline, K-36
	// 235 -> 232: tiga bulk RPC CostProductMasterService keluar dari baseline karena kini terjaga, K-43
	// 232 -> 219: tiga belas RPC di atas keluar dari baseline karena kini terjaga.
	assert.Lenf(t, knownFailOpen, 219,
		"fail-open debt changed. If it went DOWN, great — update this number. "+
			"If it went UP, revert: knownFailOpen must never grow.")
}
