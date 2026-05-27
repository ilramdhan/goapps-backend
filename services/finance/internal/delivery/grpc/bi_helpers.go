package grpc

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	dsdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/datasource"
	factmetricdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
	groupdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/group"
	jobdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/job"
)

// iso8601 formats a time as ISO 8601, or empty for zero times.
func iso8601(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// biPaginationResponse builds a PaginationResponse from page/pageSize/total.
func biPaginationResponse(page, pageSize int, total int64) *commonv1.PaginationResponse {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	totalPages := int32(0)
	if total > 0 {
		totalPages = int32((total + int64(pageSize) - 1) / int64(pageSize)) //nolint:gosec
	}
	return &commonv1.PaginationResponse{
		CurrentPage: int32(page),     //nolint:gosec
		PageSize:    int32(pageSize), //nolint:gosec
		TotalItems:  total,
		TotalPages:  totalPages,
	}
}

// biDomainErrorToBase maps BI domain errors to BaseResponse with appropriate status code.
func biDomainErrorToBase(err error) *commonv1.BaseResponse {
	if err == nil {
		return successResponse("")
	}
	switch {
	case errors.Is(err, dashboarddomain.ErrNotFound),
		errors.Is(err, groupdomain.ErrNotFound),
		errors.Is(err, dsdomain.ErrNotFound),
		errors.Is(err, jobdomain.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, dashboarddomain.ErrAlreadyExists),
		errors.Is(err, groupdomain.ErrAlreadyExists):
		return ConflictResponse(err.Error())
	case errors.Is(err, dashboarddomain.ErrForbidden):
		return ErrorResponse("403", err.Error())
	case errors.Is(err, groupdomain.ErrInUse):
		return ConflictResponse(err.Error())
	case errors.Is(err, dashboarddomain.ErrInvalidCode),
		errors.Is(err, dashboarddomain.ErrInvalidChartType),
		errors.Is(err, dashboarddomain.ErrInvalidGrain),
		errors.Is(err, dashboarddomain.ErrInvalidCompareMode),
		errors.Is(err, dashboarddomain.ErrInvalidPeriod),
		errors.Is(err, dashboarddomain.ErrInvalidDrillLevel),
		errors.Is(err, dashboarddomain.ErrInvalidCacheTTL),
		errors.Is(err, dashboarddomain.ErrInvalidRefreshInterval),
		errors.Is(err, dashboarddomain.ErrInvalidChartConfig),
		errors.Is(err, dashboarddomain.ErrInvalidKpiConfig),
		errors.Is(err, dashboarddomain.ErrInvalidTitle),
		errors.Is(err, groupdomain.ErrInvalidCode),
		errors.Is(err, groupdomain.ErrInvalidName),
		errors.Is(err, factmetricdomain.ErrInvalidPlan),
		errors.Is(err, factmetricdomain.ErrDrillTooDeep):
		return ErrorResponse("400", err.Error())
	}
	// Fallback to error-message inspection for wrapped errors not directly tagged.
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"):
		return NotFoundResponse(msg)
	case strings.Contains(msg, "already exists"):
		return ConflictResponse(msg)
	case strings.Contains(msg, "invalid"):
		return ErrorResponse("400", msg)
	}
	return InternalErrorResponse(msg)
}

// userUUIDFromContext parses the authenticated user's UUID from context.
// Returns uuid.Nil when no user is set (system/anonymous).
func userUUIDFromContext(userID string) uuid.UUID {
	if userID == "" || userID == "system" {
		return uuid.Nil
	}
	if id, err := uuid.Parse(userID); err == nil {
		return id
	}
	return uuid.Nil
}

// uuidFromString returns uuid.Nil for empty strings; otherwise the parsed UUID (or Nil on bad input).
func uuidFromString(s string) uuid.UUID {
	if s == "" {
		return uuid.Nil
	}
	if id, err := uuid.Parse(s); err == nil {
		return id
	}
	return uuid.Nil
}

// =========================================================================
// Enum maps
// =========================================================================

func periodeGrainToString(g financev1.PeriodeGrain) string {
	switch g {
	case financev1.PeriodeGrain_PERIODE_GRAIN_DAILY:
		return "DAILY"
	case financev1.PeriodeGrain_PERIODE_GRAIN_MONTHLY:
		return "MONTHLY"
	case financev1.PeriodeGrain_PERIODE_GRAIN_QUARTERLY:
		return "QUARTERLY"
	case financev1.PeriodeGrain_PERIODE_GRAIN_YEARLY:
		return "YEARLY"
	default:
		return ""
	}
}

func stringToPeriodeGrain(s string) financev1.PeriodeGrain {
	switch s {
	case "DAILY":
		return financev1.PeriodeGrain_PERIODE_GRAIN_DAILY
	case "MONTHLY":
		return financev1.PeriodeGrain_PERIODE_GRAIN_MONTHLY
	case "QUARTERLY":
		return financev1.PeriodeGrain_PERIODE_GRAIN_QUARTERLY
	case "YEARLY":
		return financev1.PeriodeGrain_PERIODE_GRAIN_YEARLY
	}
	return financev1.PeriodeGrain_PERIODE_GRAIN_UNSPECIFIED
}

func chartTypeToString(t financev1.ChartType) string {
	switch t {
	case financev1.ChartType_CHART_TYPE_BAR:
		return "bar"
	case financev1.ChartType_CHART_TYPE_HORIZONTAL_BAR:
		return "horizontal_bar"
	case financev1.ChartType_CHART_TYPE_STACKED_BAR:
		return "stacked_bar"
	case financev1.ChartType_CHART_TYPE_LINE:
		return "line"
	case financev1.ChartType_CHART_TYPE_AREA:
		return "area"
	case financev1.ChartType_CHART_TYPE_WATERFALL:
		return "waterfall"
	case financev1.ChartType_CHART_TYPE_DONUT:
		return "donut"
	case financev1.ChartType_CHART_TYPE_KPI_CARD:
		return "kpi_card"
	case financev1.ChartType_CHART_TYPE_TREEMAP:
		return "treemap"
	case financev1.ChartType_CHART_TYPE_HEATMAP:
		return "heatmap"
	case financev1.ChartType_CHART_TYPE_SCATTER:
		return "scatter"
	case financev1.ChartType_CHART_TYPE_MIXED:
		return "mixed"
	case financev1.ChartType_CHART_TYPE_DATA_TABLE:
		return "data_table"
	default:
		return ""
	}
}

func stringToChartType(s string) financev1.ChartType {
	switch s {
	case "bar":
		return financev1.ChartType_CHART_TYPE_BAR
	case "horizontal_bar":
		return financev1.ChartType_CHART_TYPE_HORIZONTAL_BAR
	case "stacked_bar":
		return financev1.ChartType_CHART_TYPE_STACKED_BAR
	case "line":
		return financev1.ChartType_CHART_TYPE_LINE
	case "area":
		return financev1.ChartType_CHART_TYPE_AREA
	case "waterfall":
		return financev1.ChartType_CHART_TYPE_WATERFALL
	case "donut":
		return financev1.ChartType_CHART_TYPE_DONUT
	case "kpi_card":
		return financev1.ChartType_CHART_TYPE_KPI_CARD
	case "treemap":
		return financev1.ChartType_CHART_TYPE_TREEMAP
	case "heatmap":
		return financev1.ChartType_CHART_TYPE_HEATMAP
	case "scatter":
		return financev1.ChartType_CHART_TYPE_SCATTER
	case "mixed":
		return financev1.ChartType_CHART_TYPE_MIXED
	case "data_table":
		return financev1.ChartType_CHART_TYPE_DATA_TABLE
	}
	return financev1.ChartType_CHART_TYPE_UNSPECIFIED
}

func compareModeToString(m financev1.CompareMode) string {
	switch m {
	case financev1.CompareMode_COMPARE_MODE_NONE:
		return "none"
	case financev1.CompareMode_COMPARE_MODE_MOM:
		return "MoM"
	case financev1.CompareMode_COMPARE_MODE_QOQ:
		return "QoQ"
	case financev1.CompareMode_COMPARE_MODE_YOY:
		return "YoY"
	case financev1.CompareMode_COMPARE_MODE_YTD:
		return "YTD"
	case financev1.CompareMode_COMPARE_MODE_R12:
		return "R12"
	default:
		return ""
	}
}

func stringToCompareMode(s string) financev1.CompareMode {
	switch s {
	case "none":
		return financev1.CompareMode_COMPARE_MODE_NONE
	case "MoM":
		return financev1.CompareMode_COMPARE_MODE_MOM
	case "QoQ":
		return financev1.CompareMode_COMPARE_MODE_QOQ
	case "YoY":
		return financev1.CompareMode_COMPARE_MODE_YOY
	case "YTD":
		return financev1.CompareMode_COMPARE_MODE_YTD
	case "R12":
		return financev1.CompareMode_COMPARE_MODE_R12
	}
	return financev1.CompareMode_COMPARE_MODE_UNSPECIFIED
}

func stringToDataSourceType(s string) financev1.DataSourceType {
	switch s {
	case "ORACLE":
		return financev1.DataSourceType_DATA_SOURCE_TYPE_ORACLE
	case "LARAVEL":
		return financev1.DataSourceType_DATA_SOURCE_TYPE_LARAVEL
	case "EXCEL":
		return financev1.DataSourceType_DATA_SOURCE_TYPE_EXCEL
	case "MANUAL":
		return financev1.DataSourceType_DATA_SOURCE_TYPE_MANUAL
	case "API":
		return financev1.DataSourceType_DATA_SOURCE_TYPE_API
	}
	return financev1.DataSourceType_DATA_SOURCE_TYPE_UNSPECIFIED
}

// =========================================================================
// Struct <-> map conversion (protobuf Struct ↔ Go map)
// =========================================================================

// mapToStruct converts a Go map to a *structpb.Struct, returning nil for empty/nil input.
func mapToStruct(m map[string]any) *structpb.Struct {
	if m == nil {
		return nil
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		return nil
	}
	return s
}

// structToMap converts a *structpb.Struct to a Go map. Returns nil for nil input.
func structToMap(s *structpb.Struct) map[string]any {
	if s == nil {
		return nil
	}
	return s.AsMap()
}

// structListToMaps converts a *structpb.Struct (interpreted as having a single "items" array
// of objects) — used as a transport mechanism for repeated JSON objects.
//
// Convention: kpi_config is sent as a Struct with key "items" mapping to a list of objects.
// Plain JSON arrays cannot be sent directly as Struct, so we wrap them. The frontend BFF
// adheres to this same convention.
func structListToMaps(s *structpb.Struct) []map[string]any {
	if s == nil {
		return nil
	}
	items, ok := s.Fields["items"]
	if !ok {
		return nil
	}
	list := items.GetListValue()
	if list == nil {
		return nil
	}
	out := make([]map[string]any, 0, len(list.Values))
	for _, v := range list.Values {
		if sv := v.GetStructValue(); sv != nil {
			out = append(out, sv.AsMap())
		}
	}
	return out
}

// mapsToStructList wraps a list of maps in a Struct under the "items" key.
func mapsToStructList(items []map[string]any) *structpb.Struct {
	if len(items) == 0 {
		return nil
	}
	values := make([]any, 0, len(items))
	for _, m := range items {
		values = append(values, m)
	}
	wrapped := map[string]any{"items": values}
	s, err := structpb.NewStruct(wrapped)
	if err != nil {
		return nil
	}
	return s
}

// =========================================================================
// Proto <-> domain converters
// =========================================================================

// dashboardToProto converts a domain Dashboard to its proto representation.
func dashboardToProto(d *dashboarddomain.Dashboard) *financev1.Dashboard {
	if d == nil {
		return nil
	}
	compareModes := d.CompareModes().Strings()
	pbCompare := make([]financev1.CompareMode, 0, len(compareModes))
	for _, s := range compareModes {
		pbCompare = append(pbCompare, stringToCompareMode(s))
	}

	audit := &commonv1.AuditInfo{}
	if !d.CreatedAt().IsZero() {
		audit.CreatedAt = iso8601(d.CreatedAt())
	}
	if d.CreatedBy() != uuid.Nil {
		audit.CreatedBy = d.CreatedBy().String()
	}
	if !d.UpdatedAt().IsZero() {
		audit.UpdatedAt = iso8601(d.UpdatedAt())
	}
	if d.UpdatedBy() != uuid.Nil {
		audit.UpdatedBy = d.UpdatedBy().String()
	}

	return &financev1.Dashboard{
		DashboardId:        d.ID().String(),
		DashboardCode:      d.Code().String(),
		DashboardTitle:     d.Title(),
		Description:        d.Description(),
		FilterType:         d.FilterType(),
		FilterGroup_1:      d.FilterGroup1(),
		PeriodeGrain:       stringToPeriodeGrain(d.PeriodGrain().String()),
		DefaultPeriod:      d.DefaultPeriod().String(),
		ChartType:          stringToChartType(d.ChartType().String()),
		ChartConfig:        mapToStruct(d.ChartConfig().MarshalToMap()),
		LayoutConfig:       mapToStruct(d.LayoutConfig()),
		CompareModes:       pbCompare,
		KpiConfig:          mapsToStructList(d.KpiConfig().MarshalToList()),
		DrillEnabled:       d.DrillEnabled(),
		MaxDrillLevel:      int32(d.MaxDrillLevel().Int()),       //nolint:gosec // bounded 1..3 by VO
		CacheTtlSec:        int32(d.CacheTTL().Seconds()),        //nolint:gosec // bounded 0..86400
		RefreshIntervalSec: int32(d.RefreshInterval().Seconds()), //nolint:gosec // bounded 0..3600
		DisplayOrder:       int32(d.DisplayOrder()),              //nolint:gosec
		GroupId:            d.GroupID().String(),
		IsActive:           d.IsActive(),
		AllowedRoleCodes:   d.AllowedRoleCodes(),
		Audit:              audit,
	}
}

// groupToProto converts a domain Group to its proto representation.
func groupToProto(g *groupdomain.Group) *financev1.DashboardGroup {
	if g == nil {
		return nil
	}
	audit := &commonv1.AuditInfo{}
	if !g.CreatedAt().IsZero() {
		audit.CreatedAt = iso8601(g.CreatedAt())
	}
	if g.CreatedBy() != uuid.Nil {
		audit.CreatedBy = g.CreatedBy().String()
	}
	if !g.UpdatedAt().IsZero() {
		audit.UpdatedAt = iso8601(g.UpdatedAt())
	}
	if g.UpdatedBy() != uuid.Nil {
		audit.UpdatedBy = g.UpdatedBy().String()
	}
	return &financev1.DashboardGroup{
		GroupId:      g.ID().String(),
		GroupCode:    g.Code(),
		GroupName:    g.Name(),
		Description:  g.Description(),
		Icon:         g.Icon(),
		DisplayOrder: int32(g.DisplayOrder()), //nolint:gosec
		IsActive:     g.IsActive(),
		Audit:        audit,
	}
}

// dataSourceToProto converts a domain DataSource to its proto representation.
func dataSourceToProto(ds *dsdomain.DataSource) *financev1.DataSource {
	if ds == nil {
		return nil
	}
	audit := &commonv1.AuditInfo{}
	if !ds.CreatedAt.IsZero() {
		audit.CreatedAt = iso8601(ds.CreatedAt)
	}
	if !ds.UpdatedAt.IsZero() {
		audit.UpdatedAt = iso8601(ds.UpdatedAt)
	}
	return &financev1.DataSource{
		SourceId:    ds.ID.String(),
		SourceCode:  ds.Code,
		SourceName:  ds.Name,
		SourceType:  stringToDataSourceType(ds.Type),
		Description: ds.Description,
		IsActive:    ds.IsActive,
		Audit:       audit,
	}
}

// biJobToProto converts a domain Job to its proto representation.
func biJobToProto(j *jobdomain.Job) *financev1.BiJob {
	if j == nil {
		return nil
	}
	audit := &commonv1.AuditInfo{}
	if !j.CreatedAt.IsZero() {
		audit.CreatedAt = iso8601(j.CreatedAt)
	}
	if !j.UpdatedAt.IsZero() {
		audit.UpdatedAt = iso8601(j.UpdatedAt)
	}
	out := &financev1.BiJob{
		JobId:           j.ID.String(),
		JobName:         j.Name,
		SourceId:        j.SourceID.String(),
		SourceCode:      j.SourceCode,
		TargetType:      j.TargetType,
		ScheduleCron:    j.ScheduleCron,
		OracleProcedure: j.OracleProcedure,
		Config:          mapToStruct(j.Config),
		IsActive:        j.IsActive,
		LastStatus:      j.LastStatus,
		LastDurationMs:  int32(j.LastDurationMs), //nolint:gosec
		Audit:           audit,
	}
	if !j.LastRunAt.IsZero() {
		out.LastRunAt = timestamppb.New(j.LastRunAt)
	}
	return out
}

// jobLogToProto converts a domain Log to its proto representation.
func jobLogToProto(l *jobdomain.Log) *financev1.BiJobLog {
	if l == nil {
		return nil
	}
	out := &financev1.BiJobLog{
		LogId:        l.LogID,
		JobId:        l.JobID.String(),
		JobName:      l.JobName,
		StartedAt:    timestamppb.New(l.StartedAt),
		Status:       l.Status,
		RowsAffected: int32(l.RowsAffected), //nolint:gosec
		ErrorMessage: l.ErrorMessage,
		TriggeredBy:  l.TriggeredBy,
		DurationMs:   int32(l.DurationMs), //nolint:gosec
	}
	if !l.EndedAt.IsZero() {
		out.EndedAt = timestamppb.New(l.EndedAt)
	}
	return out
}
