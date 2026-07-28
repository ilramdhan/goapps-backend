package dailyperf

import (
	"context"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/shared/excel"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
)

// exportSheetName is the worksheet name for the daily-performance export.
const exportSheetName = "Daily Performance"

// exportPageSize caps the number of snapshot rows pulled per export. A day's
// snapshots across all areas/machines/shifts stays well under this.
const exportPageSize = 5000

// ExportReader is the read surface the daily-performance export needs.
// Implemented by postgres.DailyPerfRepository.
type ExportReader interface {
	ListSnapshots(ctx context.Context, filter dailyperf.SnapshotFilter) ([]*dailyperf.EfficiencySnapshot, int64, error)
}

// Exporter builds the daily-performance Excel workbook from efficiency snapshots.
type Exporter struct {
	reader ExportReader
}

// NewExporter builds the daily-performance exporter.
func NewExporter(reader ExportReader) *Exporter {
	return &Exporter{reader: reader}
}

// exportColumns defines the daily-performance export sheet layout.
var exportColumns = []excel.Column{
	{Header: "Area", Width: 8},
	{Header: "Scope", Width: 16},
	{Header: "Date", Width: 12},
	{Header: "Shift", Width: 8},
	{Header: "Machine ID", Width: 12},
	{Header: "Segment", Width: 10},
	{Header: "Basis", Width: 12},
	{Header: "Qty Theo 100%", Width: 15},
	{Header: "Qty Actual", Width: 14},
	{Header: "Qty Loss", Width: 12},
	{Header: "Qty Waste", Width: 12},
	{Header: "Prod Eff %", Width: 12},
	{Header: "Running Eff %", Width: 14},
	{Header: "Plant Eff %", Width: 12},
	{Header: "Yield %", Width: 10},
	{Header: "Waste %", Width: 10},
	{Header: "Breaks", Width: 9},
}

// ExportDailyPerformance builds an .xlsx workbook of the efficiency snapshots for
// a date, optionally scoped to one area (empty area = all areas). The workbook
// always contains a header row, so it is never empty even when no snapshots exist.
func (e *Exporter) ExportDailyPerformance(ctx context.Context, date time.Time, area string) ([]byte, error) {
	filter := dailyperf.SnapshotFilter{
		Page:      1,
		PageSize:  exportPageSize,
		Area:      area,
		DateFrom:  &date,
		DateTo:    &date,
		SortBy:    "area",
		SortOrder: "asc",
	}
	snaps, _, err := e.reader.ListSnapshots(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("read snapshots for export: %w", err)
	}

	rows := make([]excel.ExportRow, 0, len(snaps))
	for _, s := range snaps {
		rows = append(rows, snapshotToExportRow(s))
	}
	return excel.Export(exportSheetName, exportColumns, rows)
}

// snapshotToExportRow maps one efficiency snapshot to an Excel row.
func snapshotToExportRow(s *dailyperf.EfficiencySnapshot) excel.ExportRow {
	basis := "INCLUDING"
	if s.IsExcluding {
		basis = "EXCLUDING"
	}
	return excel.ExportRow{
		s.Area,
		s.Scope,
		s.Date.Format("2006-01-02"),
		derefString(s.Shift),
		derefInt64(s.MachineID),
		derefString(s.Segment),
		basis,
		s.QtyTheoretical100,
		s.QtyActual,
		s.QtyLoss,
		s.QtyWaste,
		s.EffProductionPct,
		s.EffRunningPct,
		s.EffPlantPct,
		s.YieldPct,
		s.WastePct,
		s.BreaksCount,
	}
}

// derefString returns the pointed-to string or "".
func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// derefInt64 returns the pointed-to int64 or 0.
func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
