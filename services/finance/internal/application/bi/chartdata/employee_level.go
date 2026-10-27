package chartdata

import (
	"fmt"
	"strings"

	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// PermWorkforceEmployee is the permission required to read employee-level (group_3 = NIK)
// facts of HR_* types (HD-07, HD-10).
const PermWorkforceEmployee = "finance.bi.workforceemployee.view"

// PermWorkforceView is the permission required to read any HR_* report (department level, HD-10).
const PermWorkforceView = "finance.bi.workforce.view"

// hrTypePrefix marks fact types from the HR dashboard whose group_3 holds a NIK.
const hrTypePrefix = "HR_"

// IsHRType reports whether a fact type belongs to the HR dashboard (case-insensitive).
func IsHRType(factType string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(factType)), hrTypePrefix)
}

// TouchesHRType reports whether a dashboard reads any HR_* fact type: its own filter_type or a
// KPI cross_ratio numerator/denominator type (HD-08). Gating only filter_type would let a
// non-HR dashboard pull HR numbers through a KPI.
func TouchesHRType(filterType string, kpis dashboarddomain.KpiConfig) bool {
	if IsHRType(filterType) {
		return true
	}
	for _, k := range kpis {
		if IsHRType(k.CrossRatioNumeratorType) || IsHRType(k.CrossRatioDenominatorType) {
			return true
		}
	}
	return false
}

func normalizedGroupBy(g string) string { return strings.ToLower(strings.TrimSpace(g)) }

// NormalizeGroupBy validates the computed-ratio group_by column against a whitelist, because
// the planner interpolates it into SQL. Empty defaults to group_2.
func NormalizeGroupBy(g string) (string, error) {
	switch n := normalizedGroupBy(g); n {
	case "":
		return colGroup2, nil
	case colGroup1, colGroup2, colGroup3:
		return n, nil
	}
	return "", fmt.Errorf("%w: group_by %q not allowed", factmetric.ErrInvalidPlan, g)
}

// RequestsEmployeeLevel reports whether the planned query would expose group_3 of an HR_*
// type: a drill into level 3, or a computed ratio grouped by group_3. Dashboards of other
// types are never restricted here.
func RequestsEmployeeLevel(d *dashboarddomain.Dashboard, f ViewerFilters) bool {
	if !IsHRType(d.FilterType()) {
		return false
	}
	if isEmployeeTable(d) {
		return true
	}
	if f.ComputedRatio != nil && normalizedGroupBy(f.ComputedRatio.GroupBy) == colGroup3 {
		return true
	}
	// Without a pinned group_1, drill depth 2 reaches group_3; with one, depth 1 already does.
	reach := 2
	if d.FilterGroup1() != "" {
		reach = 1
	}
	return len(f.DrillPath) >= reach
}
