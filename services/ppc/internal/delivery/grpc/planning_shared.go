// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"strconv"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
)

// planningDateLayout is the ISO-8601 date layout used across planning boundaries.
const planningDateLayout = "2006-01-02"

// Repeated status/type string literals shared across the enum mappers, hoisted
// to satisfy goconst (each appears in both to-string and from-string switches).
const (
	strMTS                = "MTS"
	strConfirmed          = "CONFIRMED"
	strSplit              = "SPLIT"
	strPendingProductLink = "PENDING_PRODUCT_LINK"
	strDraft              = "DRAFT"
	strCompleted          = "COMPLETED"
	strClosed             = "CLOSED"
)

// actorID resolves the acting user id from context as an int64. Non-numeric or
// missing identities (e.g. the internal service identity) resolve to 0, which
// the domain treats as the system actor where a numeric id is required.
func actorID(ctx context.Context) int64 {
	if v, ok := ctx.Value(AuthUserIDKey).(string); ok {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			return id
		}
	}
	return 0
}

// formatInt64 renders an int64 id as a string for audit fields.
func formatInt64(v int64) string { return strconv.FormatInt(v, 10) }

// formatDate renders a time as an ISO-8601 date (empty for the zero time).
func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(planningDateLayout)
}

// formatTimePtr renders an optional timestamp as RFC-3339 (empty when nil).
func formatTimePtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// dateField parses a required ISO-8601 date field. On failure it returns a 400
// BaseResponse (second return); callers return it directly.
func dateField(field, value string) (time.Time, *commonv1.BaseResponse) {
	t, err := time.Parse(planningDateLayout, value)
	if err != nil {
		return time.Time{}, errorResponse("400", "invalid "+field)
	}
	return t, nil
}

// optionalDateField parses an optional ISO-8601 date field. Empty yields the
// zero time and no error.
func optionalDateField(field, value string) (*time.Time, *commonv1.BaseResponse) {
	if value == "" {
		return nil, nil //nolint:nilnil // empty date legitimately maps to no value
	}
	t, err := time.Parse(planningDateLayout, value)
	if err != nil {
		return nil, errorResponse("400", "invalid "+field)
	}
	return &t, nil
}

// paginationProto builds the common pagination envelope.
func paginationProto(currentPage, pageSize int32, totalItems int64, totalPages int32) *commonv1.PaginationResponse {
	return &commonv1.PaginationResponse{
		CurrentPage: currentPage,
		PageSize:    pageSize,
		TotalItems:  totalItems,
		TotalPages:  totalPages,
	}
}

// ── Demand enum mappers ──────────────────────────────────────────────────────

func demandTypeToString(t ppcv1.DemandType) string {
	switch t {
	case ppcv1.DemandType_DEMAND_TYPE_CONTRACT:
		return "CONTRACT"
	case ppcv1.DemandType_DEMAND_TYPE_MTS:
		return strMTS
	case ppcv1.DemandType_DEMAND_TYPE_SAMPLE:
		return "SAMPLE"
	case ppcv1.DemandType_DEMAND_TYPE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToDemandType(s string) ppcv1.DemandType {
	switch s {
	case "CONTRACT":
		return ppcv1.DemandType_DEMAND_TYPE_CONTRACT
	case strMTS:
		return ppcv1.DemandType_DEMAND_TYPE_MTS
	case "SAMPLE":
		return ppcv1.DemandType_DEMAND_TYPE_SAMPLE
	default:
		return ppcv1.DemandType_DEMAND_TYPE_UNSPECIFIED
	}
}

func demandSubTypeToString(t ppcv1.DemandSubType) string {
	switch t {
	case ppcv1.DemandSubType_DEMAND_SUB_TYPE_CF_EXPORT:
		return "CF_EXPORT"
	case ppcv1.DemandSubType_DEMAND_SUB_TYPE_NEW_EXPORT:
		return "NEW_EXPORT"
	case ppcv1.DemandSubType_DEMAND_SUB_TYPE_LOCAL:
		return "LOCAL"
	case ppcv1.DemandSubType_DEMAND_SUB_TYPE_INTERNAL:
		return "INTERNAL"
	case ppcv1.DemandSubType_DEMAND_SUB_TYPE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToDemandSubType(s string) ppcv1.DemandSubType {
	switch s {
	case "CF_EXPORT":
		return ppcv1.DemandSubType_DEMAND_SUB_TYPE_CF_EXPORT
	case "NEW_EXPORT":
		return ppcv1.DemandSubType_DEMAND_SUB_TYPE_NEW_EXPORT
	case "LOCAL":
		return ppcv1.DemandSubType_DEMAND_SUB_TYPE_LOCAL
	case "INTERNAL":
		return ppcv1.DemandSubType_DEMAND_SUB_TYPE_INTERNAL
	default:
		return ppcv1.DemandSubType_DEMAND_SUB_TYPE_UNSPECIFIED
	}
}

func demandSourceToString(t ppcv1.DemandSource) string {
	switch t {
	case ppcv1.DemandSource_DEMAND_SOURCE_ORION_PULL:
		return "ORION_PULL"
	case ppcv1.DemandSource_DEMAND_SOURCE_MANUAL:
		return "MANUAL"
	case ppcv1.DemandSource_DEMAND_SOURCE_MTS_APPROVED:
		return "MTS_APPROVED"
	case ppcv1.DemandSource_DEMAND_SOURCE_CARRY_FORWARD:
		return "CARRY_FORWARD"
	case ppcv1.DemandSource_DEMAND_SOURCE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToDemandSource(s string) ppcv1.DemandSource {
	switch s {
	case "ORION_PULL":
		return ppcv1.DemandSource_DEMAND_SOURCE_ORION_PULL
	case "MANUAL":
		return ppcv1.DemandSource_DEMAND_SOURCE_MANUAL
	case "MTS_APPROVED":
		return ppcv1.DemandSource_DEMAND_SOURCE_MTS_APPROVED
	case "CARRY_FORWARD":
		return ppcv1.DemandSource_DEMAND_SOURCE_CARRY_FORWARD
	default:
		return ppcv1.DemandSource_DEMAND_SOURCE_UNSPECIFIED
	}
}

func demandStatusToString(t ppcv1.DemandStatus) string {
	switch t {
	case ppcv1.DemandStatus_DEMAND_STATUS_PENDING_CONFIRMATION:
		return "PENDING_CONFIRMATION"
	case ppcv1.DemandStatus_DEMAND_STATUS_CONFIRMED:
		return strConfirmed
	case ppcv1.DemandStatus_DEMAND_STATUS_IN_PRODUCTION:
		return "IN_PRODUCTION"
	case ppcv1.DemandStatus_DEMAND_STATUS_PARTIAL:
		return "PARTIAL"
	case ppcv1.DemandStatus_DEMAND_STATUS_FULFILLED:
		return "FULFILLED"
	case ppcv1.DemandStatus_DEMAND_STATUS_CANCELLED:
		return "CANCELLED"
	case ppcv1.DemandStatus_DEMAND_STATUS_CARRIED_OVER:
		return "CARRIED_OVER"
	case ppcv1.DemandStatus_DEMAND_STATUS_DEFERRED:
		return "DEFERRED"
	case ppcv1.DemandStatus_DEMAND_STATUS_SPLIT:
		return strSplit
	case ppcv1.DemandStatus_DEMAND_STATUS_PENDING_PRODUCT_LINK:
		return strPendingProductLink
	case ppcv1.DemandStatus_DEMAND_STATUS_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToDemandStatus(s string) ppcv1.DemandStatus {
	switch s {
	case "PENDING_CONFIRMATION":
		return ppcv1.DemandStatus_DEMAND_STATUS_PENDING_CONFIRMATION
	case strConfirmed:
		return ppcv1.DemandStatus_DEMAND_STATUS_CONFIRMED
	case "IN_PRODUCTION":
		return ppcv1.DemandStatus_DEMAND_STATUS_IN_PRODUCTION
	case "PARTIAL":
		return ppcv1.DemandStatus_DEMAND_STATUS_PARTIAL
	case "FULFILLED":
		return ppcv1.DemandStatus_DEMAND_STATUS_FULFILLED
	case "CANCELLED":
		return ppcv1.DemandStatus_DEMAND_STATUS_CANCELLED
	case "CARRIED_OVER":
		return ppcv1.DemandStatus_DEMAND_STATUS_CARRIED_OVER
	case "DEFERRED":
		return ppcv1.DemandStatus_DEMAND_STATUS_DEFERRED
	case strSplit:
		return ppcv1.DemandStatus_DEMAND_STATUS_SPLIT
	case strPendingProductLink:
		return ppcv1.DemandStatus_DEMAND_STATUS_PENDING_PRODUCT_LINK
	default:
		return ppcv1.DemandStatus_DEMAND_STATUS_UNSPECIFIED
	}
}

func carryActionToString(t ppcv1.CarryAction) string {
	switch t {
	case ppcv1.CarryAction_CARRY_ACTION_CARRY_AS_IS:
		return "CARRY_AS_IS"
	case ppcv1.CarryAction_CARRY_ACTION_SPLIT:
		return strSplit
	case ppcv1.CarryAction_CARRY_ACTION_DEFER:
		return "DEFER"
	case ppcv1.CarryAction_CARRY_ACTION_PARTIAL_CARRY:
		return "PARTIAL_CARRY"
	case ppcv1.CarryAction_CARRY_ACTION_CANCEL:
		return "CANCEL"
	case ppcv1.CarryAction_CARRY_ACTION_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToCarryAction(s string) ppcv1.CarryAction {
	switch s {
	case "CARRY_AS_IS":
		return ppcv1.CarryAction_CARRY_ACTION_CARRY_AS_IS
	case strSplit:
		return ppcv1.CarryAction_CARRY_ACTION_SPLIT
	case "DEFER":
		return ppcv1.CarryAction_CARRY_ACTION_DEFER
	case "PARTIAL_CARRY":
		return ppcv1.CarryAction_CARRY_ACTION_PARTIAL_CARRY
	case "CANCEL":
		return ppcv1.CarryAction_CARRY_ACTION_CANCEL
	default:
		return ppcv1.CarryAction_CARRY_ACTION_UNSPECIFIED
	}
}

func gradeReqToString(t ppcv1.GradeReq) string {
	switch t {
	case ppcv1.GradeReq_GRADE_REQ_AX_ONLY:
		return "AX_ONLY"
	case ppcv1.GradeReq_GRADE_REQ_AX_AM_CLAUSE:
		return "AX_AM_CLAUSE"
	case ppcv1.GradeReq_GRADE_REQ_NONE:
		return "NONE"
	case ppcv1.GradeReq_GRADE_REQ_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToGradeReq(s string) ppcv1.GradeReq {
	switch s {
	case "AX_ONLY":
		return ppcv1.GradeReq_GRADE_REQ_AX_ONLY
	case "AX_AM_CLAUSE":
		return ppcv1.GradeReq_GRADE_REQ_AX_AM_CLAUSE
	case "NONE":
		return ppcv1.GradeReq_GRADE_REQ_NONE
	default:
		return ppcv1.GradeReq_GRADE_REQ_UNSPECIFIED
	}
}

// ── Plan item / WO shared enum mappers ───────────────────────────────────────

func planItemTypeToString(t ppcv1.PlanItemType) string {
	switch t {
	case ppcv1.PlanItemType_PLAN_ITEM_TYPE_FG_DELIVERY:
		return "FG_DELIVERY"
	case ppcv1.PlanItemType_PLAN_ITEM_TYPE_INTERMEDIATE:
		return "INTERMEDIATE"
	case ppcv1.PlanItemType_PLAN_ITEM_TYPE_MTS:
		return strMTS
	case ppcv1.PlanItemType_PLAN_ITEM_TYPE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToPlanItemType(s string) ppcv1.PlanItemType {
	switch s {
	case "FG_DELIVERY":
		return ppcv1.PlanItemType_PLAN_ITEM_TYPE_FG_DELIVERY
	case "INTERMEDIATE":
		return ppcv1.PlanItemType_PLAN_ITEM_TYPE_INTERMEDIATE
	case strMTS:
		return ppcv1.PlanItemType_PLAN_ITEM_TYPE_MTS
	default:
		return ppcv1.PlanItemType_PLAN_ITEM_TYPE_UNSPECIFIED
	}
}

func planItemStatusToString(t ppcv1.PlanItemStatus) string {
	switch t {
	case ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_DRAFT:
		return strDraft
	case ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_ACTIVE:
		return strConfirmed
	case ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_IN_PRODUCTION:
		return "IN_PROGRESS"
	case ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_COMPLETED:
		return strCompleted
	case ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_CANCELLED:
		return strClosed
	case ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToPlanItemStatus(s string) ppcv1.PlanItemStatus {
	switch s {
	case strDraft:
		return ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_DRAFT
	case strConfirmed:
		return ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_ACTIVE
	case "IN_PROGRESS":
		return ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_IN_PRODUCTION
	case strCompleted:
		return ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_COMPLETED
	case strClosed:
		return ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_CANCELLED
	default:
		return ppcv1.PlanItemStatus_PLAN_ITEM_STATUS_UNSPECIFIED
	}
}

func rmSourceToString(t ppcv1.RMSource) string {
	switch t {
	case ppcv1.RMSource_RM_SOURCE_STORE:
		return "STORE"
	case ppcv1.RMSource_RM_SOURCE_CAPTIVE:
		return "CAPTIVE"
	case ppcv1.RMSource_RM_SOURCE_MIXED:
		return "MIXED"
	case ppcv1.RMSource_RM_SOURCE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToRMSource(s string) ppcv1.RMSource {
	switch s {
	case "STORE":
		return ppcv1.RMSource_RM_SOURCE_STORE
	case "CAPTIVE":
		return ppcv1.RMSource_RM_SOURCE_CAPTIVE
	case "MIXED":
		return ppcv1.RMSource_RM_SOURCE_MIXED
	default:
		return ppcv1.RMSource_RM_SOURCE_UNSPECIFIED
	}
}

func woStatusToString(t ppcv1.WOStatus) string {
	switch t {
	case ppcv1.WOStatus_WO_STATUS_DRAFT:
		return strDraft
	case ppcv1.WOStatus_WO_STATUS_SUBMITTED:
		return "SUBMITTED"
	case ppcv1.WOStatus_WO_STATUS_APPROVED:
		return "APPROVED"
	case ppcv1.WOStatus_WO_STATUS_SCHEDULED:
		return "SCHEDULED"
	case ppcv1.WOStatus_WO_STATUS_CHANGEOVER:
		return "CHANGEOVER"
	case ppcv1.WOStatus_WO_STATUS_RUNNING:
		return "RUNNING"
	case ppcv1.WOStatus_WO_STATUS_COMPLETED:
		return strCompleted
	case ppcv1.WOStatus_WO_STATUS_CLOSED:
		return strClosed
	case ppcv1.WOStatus_WO_STATUS_REJECTED:
		return "REJECTED"
	case ppcv1.WOStatus_WO_STATUS_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToWOStatus(s string) ppcv1.WOStatus {
	switch s {
	case strDraft:
		return ppcv1.WOStatus_WO_STATUS_DRAFT
	case "SUBMITTED":
		return ppcv1.WOStatus_WO_STATUS_SUBMITTED
	case "APPROVED":
		return ppcv1.WOStatus_WO_STATUS_APPROVED
	case "SCHEDULED":
		return ppcv1.WOStatus_WO_STATUS_SCHEDULED
	case "CHANGEOVER":
		return ppcv1.WOStatus_WO_STATUS_CHANGEOVER
	case "RUNNING":
		return ppcv1.WOStatus_WO_STATUS_RUNNING
	case strCompleted:
		return ppcv1.WOStatus_WO_STATUS_COMPLETED
	case strClosed:
		return ppcv1.WOStatus_WO_STATUS_CLOSED
	case "REJECTED":
		return ppcv1.WOStatus_WO_STATUS_REJECTED
	default:
		return ppcv1.WOStatus_WO_STATUS_UNSPECIFIED
	}
}
