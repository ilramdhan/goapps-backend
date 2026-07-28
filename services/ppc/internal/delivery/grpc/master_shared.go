// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"strconv"
	"strings"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
)

const systemActor = "system"

// successResponse builds a successful BaseResponse envelope.
func successResponse(message string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{
		IsSuccess:  true,
		StatusCode: "200",
		Message:    message,
	}
}

// errorResponse builds a failed BaseResponse envelope with the given code.
func errorResponse(statusCode, message string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{
		IsSuccess:  false,
		StatusCode: statusCode,
		Message:    message,
	}
}

// domainErrorToBaseResponse maps a domain error to a BaseResponse by inspecting
// the error message, mirroring the finance reference implementation.
func domainErrorToBaseResponse(err error) *commonv1.BaseResponse {
	if err == nil {
		return successResponse("")
	}

	errMsg := err.Error()

	switch {
	case strings.Contains(errMsg, "not found"):
		return errorResponse("404", errMsg)
	case strings.Contains(errMsg, "already exists"),
		strings.Contains(errMsg, "cannot be deleted"),
		strings.Contains(errMsg, "cannot delete"),
		strings.Contains(errMsg, "in use"):
		return errorResponse("409", errMsg)
	case strings.Contains(errMsg, "invalid"),
		strings.Contains(errMsg, "cannot generate"),
		strings.Contains(errMsg, "required"),
		strings.Contains(errMsg, "cannot be empty"),
		strings.Contains(errMsg, "must be"):
		return errorResponse("400", errMsg)
	default:
		return errorResponse("500", errMsg)
	}
}

// getUserFromContext extracts the acting username from the request context.
func getUserFromContext(ctx context.Context) string {
	if username, ok := ctx.Value(AuthUsernameKey).(string); ok && username != "" {
		return username
	}
	if userID, ok := ctx.Value(AuthUserIDKey).(string); ok && userID != "" {
		return userID
	}
	return systemActor
}

// =============================================================================
// Enum mapping helpers (proto <-> domain string)
// =============================================================================

// areaCodeToString maps a proto AreaCode to its domain string representation.
func areaCodeToString(a ppcv1.AreaCode) string {
	switch a {
	case ppcv1.AreaCode_AREA_CODE_TXT:
		return "TXT"
	case ppcv1.AreaCode_AREA_CODE_SPG:
		return "SPG"
	case ppcv1.AreaCode_AREA_CODE_TWT:
		return "TWT"
	case ppcv1.AreaCode_AREA_CODE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

// stringToAreaCode maps a domain area string to its proto AreaCode.
func stringToAreaCode(s string) ppcv1.AreaCode {
	switch s {
	case "TXT":
		return ppcv1.AreaCode_AREA_CODE_TXT
	case "SPG":
		return ppcv1.AreaCode_AREA_CODE_SPG
	case "TWT":
		return ppcv1.AreaCode_AREA_CODE_TWT
	default:
		return ppcv1.AreaCode_AREA_CODE_UNSPECIFIED
	}
}

// thresholdLevelToString maps a proto ThresholdLevel to its domain string.
func thresholdLevelToString(l ppcv1.ThresholdLevel) string {
	switch l {
	case ppcv1.ThresholdLevel_THRESHOLD_LEVEL_SYSTEM:
		return "SYSTEM"
	case ppcv1.ThresholdLevel_THRESHOLD_LEVEL_MACHINE_GROUP:
		return "MACHINE_GROUP"
	case ppcv1.ThresholdLevel_THRESHOLD_LEVEL_PRODUCT_TYPE:
		return "PRODUCT_TYPE"
	case ppcv1.ThresholdLevel_THRESHOLD_LEVEL_PRODUCT:
		return "PRODUCT"
	case ppcv1.ThresholdLevel_THRESHOLD_LEVEL_WO:
		return "WO"
	case ppcv1.ThresholdLevel_THRESHOLD_LEVEL_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

// stringToThresholdLevel maps a domain level string to its proto ThresholdLevel.
func stringToThresholdLevel(s string) ppcv1.ThresholdLevel {
	switch s {
	case "SYSTEM":
		return ppcv1.ThresholdLevel_THRESHOLD_LEVEL_SYSTEM
	case "MACHINE_GROUP":
		return ppcv1.ThresholdLevel_THRESHOLD_LEVEL_MACHINE_GROUP
	case "PRODUCT_TYPE":
		return ppcv1.ThresholdLevel_THRESHOLD_LEVEL_PRODUCT_TYPE
	case "PRODUCT":
		return ppcv1.ThresholdLevel_THRESHOLD_LEVEL_PRODUCT
	case "WO":
		return ppcv1.ThresholdLevel_THRESHOLD_LEVEL_WO
	default:
		return ppcv1.ThresholdLevel_THRESHOLD_LEVEL_UNSPECIFIED
	}
}

// thresholdUnitToString maps a proto ThresholdUnit to its domain string.
func thresholdUnitToString(u ppcv1.ThresholdUnit) string {
	switch u {
	case ppcv1.ThresholdUnit_THRESHOLD_UNIT_PCT:
		return "PCT"
	case ppcv1.ThresholdUnit_THRESHOLD_UNIT_DOFF:
		return "DOFF"
	case ppcv1.ThresholdUnit_THRESHOLD_UNIT_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

// stringToThresholdUnit maps a domain unit string to its proto ThresholdUnit.
func stringToThresholdUnit(s string) ppcv1.ThresholdUnit {
	switch s {
	case "PCT":
		return ppcv1.ThresholdUnit_THRESHOLD_UNIT_PCT
	case "DOFF":
		return ppcv1.ThresholdUnit_THRESHOLD_UNIT_DOFF
	default:
		return ppcv1.ThresholdUnit_THRESHOLD_UNIT_UNSPECIFIED
	}
}

// activeFilterToBool maps a proto ActiveFilter to an optional bool filter.
func activeFilterToBool(f ppcv1.ActiveFilter) *bool {
	switch f {
	case ppcv1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		v := true
		return &v
	case ppcv1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		v := false
		return &v
	case ppcv1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		return nil
	default:
		return nil
	}
}

// =============================================================================
// Decimal-as-string helpers (proto strings <-> float64)
// =============================================================================

// parseDecimal parses a decimal-as-string into a float64. Empty string is 0.
func parseDecimal(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// parseOptionalDecimal parses a decimal-as-string into an optional float64.
// Empty string yields nil (unset).
func parseOptionalDecimal(s string) (*float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil //nolint:nilnil // empty decimal legitimately maps to no value
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// formatDecimal formats a float64 as a decimal string with minimal digits.
func formatDecimal(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// formatOptionalDecimal formats an optional float64; nil yields an empty string.
func formatOptionalDecimal(v *float64) string {
	if v == nil {
		return ""
	}
	return formatDecimal(*v)
}

// optionalStringField maps a proto string to an optional string pointer. An
// empty string yields nil (unset).
func optionalStringField(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// optionalBoolField maps a proto bool plus its presence flag to an optional
// bool pointer. When present is false the field is treated as unset (nil).
func optionalBoolField(value, present bool) *bool {
	if !present {
		return nil
	}
	return &value
}

// decimalField parses a required decimal-as-string field. On failure it returns
// a populated 400 BaseResponse (second return); callers return it directly. On
// success the BaseResponse is nil.
func decimalField(field, value string) (float64, *commonv1.BaseResponse) {
	v, err := parseDecimal(value)
	if err != nil {
		return 0, errorResponse("400", "invalid "+field)
	}
	return v, nil
}

// optionalDecimalField parses an optional decimal-as-string field. On failure it
// returns a populated 400 BaseResponse (second return); callers return it
// directly. On success the BaseResponse is nil (value may be nil when empty).
func optionalDecimalField(field, value string) (*float64, *commonv1.BaseResponse) {
	v, err := parseOptionalDecimal(value)
	if err != nil {
		return nil, errorResponse("400", "invalid "+field)
	}
	return v, nil
}
