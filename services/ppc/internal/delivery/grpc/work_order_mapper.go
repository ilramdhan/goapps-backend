package grpc

import (
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// workOrderToProto maps a WO aggregate (+ optional production actuals) to proto.
func workOrderToProto(e *workorderdomain.WorkOrder, actuals []*workorderdomain.ProductionActual) *ppcv1.WorkOrder {
	proto := &ppcv1.WorkOrder{
		WoId:                e.ID(),
		WoNo:                e.WoNo(),
		LotNo:               e.LotNo(),
		Area:                stringToAreaCode(e.AreaCode()),
		MachineId:           e.MachineID(),
		CrhHeadId:           e.CrhHeadID(),
		CrhVersion:          e.CrhVersion(),
		PlanItemId:          e.PlanItemID(),
		RefType:             stringToRefType(e.RefType()),
		QtyTarget:           formatDecimal(e.QtyTarget()),
		GradeRequirement:    e.GradeRequirement(),
		Deadline:            formatDate(e.Deadline()),
		ProdCategory:        stringToProdCategory(e.ProdCategory()),
		SpecSnapshot:        snapshotToStruct(e.SpecSnapshot()),
		PackingSnapshot:     snapshotToStruct(e.PackingSnapshot()),
		RevisionNo:          e.RevisionNo(),
		RevisionReason:      e.RevisionReason(),
		Status:              stringToWOStatus(e.Status()),
		AutoApproveDisabled: e.AutoApproveDisabled(),
		Audit:               &commonv1.AuditInfo{CreatedBy: formatInt64(e.CreatedBy()), CreatedAt: e.CreatedAt().Format(time.RFC3339)},
	}
	if e.DemandID() != nil {
		proto.DemandId = *e.DemandID()
	}
	if e.RefWoID() != nil {
		proto.RefWoId = *e.RefWoID()
	}
	if e.PCApprovedBy() != nil {
		proto.PcApprovedBy = *e.PCApprovedBy()
	}
	if e.PCApprovedAt() != nil {
		proto.PcApprovedAt = e.PCApprovedAt().Format(time.RFC3339)
	}
	if e.PMApprovedBy() != nil {
		proto.PmApprovedBy = *e.PMApprovedBy()
	}
	if e.PMApprovedAt() != nil {
		proto.PmApprovedAt = e.PMApprovedAt().Format(time.RFC3339)
	}
	for _, p := range e.Parameters() {
		proto.Parameters = append(proto.Parameters, parameterToProto(p))
	}
	for _, a := range e.RmAllocations() {
		proto.RmAllocations = append(proto.RmAllocations, rmAllocationToProto(a))
	}
	for _, a := range actuals {
		proto.ProductionActuals = append(proto.ProductionActuals, productionActualToProto(a))
	}
	for _, l := range e.PlanItemLinks() {
		proto.LinkedPlanItems = append(proto.LinkedPlanItems, planItemLinkToProto(l, e.PlanItemID()))
	}
	return proto
}

// planItemLinkToProto maps one wo_plan_item_link row. The product/shade/deadline
// labels live on plan items in a separate lookup and are filled by the handler.
func planItemLinkToProto(l workorderdomain.PlanItemLink, anchorID int64) *ppcv1.WOPlanItemLink {
	return &ppcv1.WOPlanItemLink{
		WplId:           l.ID,
		WoId:            l.WOID,
		PlanItemId:      l.PlanItemID,
		QtyContribution: formatDecimal(l.QtyContribution),
		IsAnchor:        l.PlanItemID == anchorID,
	}
}

func parameterToProto(p *workorderdomain.Parameter) *ppcv1.WOParameter {
	return &ppcv1.WOParameter{
		WopId:        p.ID,
		WoId:         p.WOID,
		ParamId:      p.ParamID,
		ParamCode:    p.ParamCode,
		ParamName:    p.ParamName,
		DataType:     p.DataType,
		DisplayGroup: p.DisplayGroup,
		DisplayOrder: p.DisplayOrder,
		IsDual:       p.IsDual,
		ValuePpcNum:  formatOptionalDecimal(p.ValuePPCNum),
		ValuePpcText: stringPtrValue(p.ValuePPCText),
		ValuePpcFlag: boolPtrValue(p.ValuePPCFlag),
		ValuePcNum:   formatOptionalDecimal(p.ValuePCNum),
		ValuePcText:  stringPtrValue(p.ValuePCText),
		ValuePcFlag:  boolPtrValue(p.ValuePCFlag),
	}
}

func executionToProto(e *workorderdomain.Execution) *ppcv1.WOExecution {
	return &ppcv1.WOExecution{
		WoeId:     e.ID,
		WoId:      e.WOID,
		Date:      formatDate(e.Date),
		Shift:     e.Shift,
		ParamId:   e.ParamID,
		ParamCode: e.ParamCode,
		ValueNum:  formatOptionalDecimal(e.ValueNum),
		ValueText: stringPtrValue(e.ValueText),
		ValueFlag: boolPtrValue(e.ValueFlag),
		InputBy:   e.InputBy,
		InputAt:   e.InputAt.Format(time.RFC3339),
	}
}

func rmAllocationToProto(a *workorderdomain.RmAllocation) *ppcv1.WORmAllocation {
	return &ppcv1.WORmAllocation{
		WraId:        a.ID,
		WoId:         a.WOID,
		CrmRmId:      a.CrmRmID,
		RmType:       a.RmType,
		LotNo:        a.LotNo,
		RmSource:     stringToRMSource(a.RmSource),
		FreshBox:     a.FreshBox,
		ShadeCode:    a.ShadeCode,
		QtyAllocated: formatDecimal(a.QtyAllocated),
		Notes:        a.Notes,
	}
}

func resolvedParamToProto(rp workorderdomain.ResolvedParam) *ppcv1.ResolvedParam {
	return &ppcv1.ResolvedParam{
		ParamId:      rp.ParamID,
		ParamCode:    rp.ParamCode,
		ParamName:    rp.ParamName,
		DataType:     rp.DataType,
		DisplayGroup: rp.DisplayGroup,
		IsDual:       rp.IsDual,
		DisplayOrder: rp.DisplayOrder,
		ValueNum:     formatOptionalDecimal(rp.Num),
		ValueText:    stringPtrValue(rp.Text),
		ValueFlag:    boolPtrValue(rp.Flag),
		Source:       stringToParamResolutionSource(rp.Source),
	}
}

func productionActualToProto(a *workorderdomain.ProductionActual) *ppcv1.WOProductionActual {
	proto := &ppcv1.WOProductionActual{
		ActualId:         a.ID,
		WoId:             a.WOID,
		Date:             formatDate(a.Date),
		Shift:            a.Shift,
		Area:             stringToAreaCode(a.Area),
		TotalBobbins:     a.TotalBobbins,
		FullBobbins:      a.FullBobbins,
		UnfullBobbins:    a.UnfullBobbins,
		NormalBobs:       a.NormalBobs,
		DowngradeBobs:    a.DowngradeBobs,
		PendingBobs:      a.PendingBobs,
		PackCekBobs:      a.PackCekBobs,
		GrossBobbins:     a.GrossBobbins,
		TransferredBobs:  a.TransferredBobs,
		CutBobbins:       a.CutBobbins,
		NotTransfer:      a.NotTransfer,
		NormalBobsSpg:    a.NormalBobsSpg,
		DowngradeBobsSpg: a.DowngradeBobsSpg,
		NotCheckedBobs:   a.NotCheckedBobs,
		WeightPerBob:     formatDecimal(a.WeightPerBob),
		QtyBobbin:        formatDecimal(a.QtyBobbin),
		QtyActual:        formatDecimal(a.QtyActual),
		QtySource:        stringToQtyAxisSource(a.QtySource),
		AdjustReason:     a.AdjustReason,
		QtyDoffedKg:      formatDecimal(a.QtyDoffedKg),
		QtyTransferredKg: formatDecimal(a.QtyTransferredKg),
		BreaksShift1:     a.BreaksShift1,
		BreaksShift2:     a.BreaksShift2,
		BreaksShift3:     a.BreaksShift3,
		DoffFullCount:    a.DoffFullCount,
		DoffManualCount:  a.DoffManualCount,
		CoFailureCount:   a.CoFailureCount,
		SyncStatus:       a.SyncStatus,
	}
	if a.SyncedAt != nil {
		proto.SyncedAt = a.SyncedAt.Format(time.RFC3339)
	}
	if a.LastEditedBy != nil {
		proto.LastEditedBy = *a.LastEditedBy
	}
	if a.LastEditedAt != nil {
		proto.LastEditedAt = a.LastEditedAt.Format(time.RFC3339)
	}
	return proto
}

// snapshotToStruct converts a snapshot map to a protobuf Struct (nil-safe).
func snapshotToStruct(m map[string]any) *structpb.Struct {
	if m == nil {
		return nil
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		return nil
	}
	return s
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func boolPtrValue(v *bool) bool {
	return v != nil && *v
}

// ── enum mappers ─────────────────────────────────────────────────────────────

func prodCategoryToString(c ppcv1.ProdCategory) string {
	switch c {
	case ppcv1.ProdCategory_PROD_CATEGORY_NORMAL:
		return workorderdomain.ProdCategoryNormal
	case ppcv1.ProdCategory_PROD_CATEGORY_B_TO_B:
		return workorderdomain.ProdCategoryBToB
	case ppcv1.ProdCategory_PROD_CATEGORY_APQ:
		return workorderdomain.ProdCategoryAPQ
	case ppcv1.ProdCategory_PROD_CATEGORY_TRIAL:
		return workorderdomain.ProdCategoryTrial
	case ppcv1.ProdCategory_PROD_CATEGORY_SMALL_LOT:
		return workorderdomain.ProdCategorySmallLot
	case ppcv1.ProdCategory_PROD_CATEGORY_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToProdCategory(s string) ppcv1.ProdCategory {
	switch s {
	case workorderdomain.ProdCategoryNormal:
		return ppcv1.ProdCategory_PROD_CATEGORY_NORMAL
	case workorderdomain.ProdCategoryBToB:
		return ppcv1.ProdCategory_PROD_CATEGORY_B_TO_B
	case workorderdomain.ProdCategoryAPQ:
		return ppcv1.ProdCategory_PROD_CATEGORY_APQ
	case workorderdomain.ProdCategoryTrial:
		return ppcv1.ProdCategory_PROD_CATEGORY_TRIAL
	case workorderdomain.ProdCategorySmallLot:
		return ppcv1.ProdCategory_PROD_CATEGORY_SMALL_LOT
	default:
		return ppcv1.ProdCategory_PROD_CATEGORY_UNSPECIFIED
	}
}

func refTypeToString(t ppcv1.WORefType) string {
	switch t {
	case ppcv1.WORefType_WO_REF_TYPE_TEMPLATE:
		return workorderdomain.RefTypeTemplate
	case ppcv1.WORefType_WO_REF_TYPE_CONTINUATION:
		return workorderdomain.RefTypeContinuation
	case ppcv1.WORefType_WO_REF_TYPE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToRefType(s string) ppcv1.WORefType {
	switch s {
	case workorderdomain.RefTypeTemplate:
		return ppcv1.WORefType_WO_REF_TYPE_TEMPLATE
	case workorderdomain.RefTypeContinuation:
		return ppcv1.WORefType_WO_REF_TYPE_CONTINUATION
	default:
		return ppcv1.WORefType_WO_REF_TYPE_UNSPECIFIED
	}
}

func stringToQtyAxisSource(s string) ppcv1.QtyAxisSource {
	switch s {
	case "BOBBIN":
		return ppcv1.QtyAxisSource_QTY_AXIS_SOURCE_BOBBIN
	case "ADJUSTED":
		return ppcv1.QtyAxisSource_QTY_AXIS_SOURCE_ADJUSTED
	default:
		return ppcv1.QtyAxisSource_QTY_AXIS_SOURCE_UNSPECIFIED
	}
}

func stringToParamResolutionSource(s string) ppcv1.ParamResolutionSource {
	switch s {
	case workorderdomain.SourceWORef:
		return ppcv1.ParamResolutionSource_PARAM_RESOLUTION_SOURCE_WO_REF
	case workorderdomain.SourceProductMachine:
		return ppcv1.ParamResolutionSource_PARAM_RESOLUTION_SOURCE_PRODUCT_MACHINE
	case workorderdomain.SourceProduct:
		return ppcv1.ParamResolutionSource_PARAM_RESOLUTION_SOURCE_PRODUCT
	case workorderdomain.SourceDefault:
		return ppcv1.ParamResolutionSource_PARAM_RESOLUTION_SOURCE_DEFAULT
	default:
		return ppcv1.ParamResolutionSource_PARAM_RESOLUTION_SOURCE_UNSPECIFIED
	}
}
