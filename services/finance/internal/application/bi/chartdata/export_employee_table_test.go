package chartdata

import (
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// ShapeEmployeeTableForTest exposes shapeEmployeeTable to the external test package.
func ShapeEmployeeTableForTest(rows []factmetric.AggRow) ([]Series, []string) {
	return shapeEmployeeTable(rows, chart.NumberFormat("raw"), 2)
}

// ShapeEmployeeOTForTest exposes shapeEmployeeOT to the external test package.
func ShapeEmployeeOTForTest(rows []factmetric.AggRow, f ViewerFilters) ([]Series, []string, map[string]any) {
	return shapeEmployeeOT(rows, f, chart.NumberFormat("raw"), 2)
}

// ShapeEmployeeOTDaysForTest exposes shapeEmployeeOTDays to the external test package.
func ShapeEmployeeOTDaysForTest(rows []factmetric.AggRow) ([]Series, []string) {
	return shapeEmployeeOTDays(rows, chart.NumberFormat("raw"), 2)
}

// EmployeeOTMaxRows exposes the row cap to the external test package.
const EmployeeOTMaxRows = employeeOTMaxRows
