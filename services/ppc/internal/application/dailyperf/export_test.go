package dailyperf

import (
	"context"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
)

// stubExportReader serves fixture snapshots and records the filter it received.
type stubExportReader struct {
	snaps     []*dailyperf.EfficiencySnapshot
	gotFilter dailyperf.SnapshotFilter
}

func (r *stubExportReader) ListSnapshots(_ context.Context, f dailyperf.SnapshotFilter) ([]*dailyperf.EfficiencySnapshot, int64, error) {
	r.gotFilter = f
	return r.snaps, int64(len(r.snaps)), nil
}

func TestExportDailyPerformanceNonEmpty(t *testing.T) {
	shift := "1"
	machineID := int64(7)
	reader := &stubExportReader{snaps: []*dailyperf.EfficiencySnapshot{
		{
			Area: "TXT", Scope: "MACHINE_SHIFT", MachineID: &machineID,
			Date: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), Shift: &shift,
			QtyTheoretical100: 1200, QtyActual: 1080, EffProductionPct: 90,
		},
	}}
	exp := NewExporter(reader)

	content, err := exp.ExportDailyPerformance(context.Background(), time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), "TXT")
	if err != nil {
		t.Fatalf("ExportDailyPerformance error: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty workbook bytes")
	}
	// xlsx is a zip archive; verify the PK magic header.
	if len(content) < 2 || content[0] != 'P' || content[1] != 'K' {
		t.Errorf("expected xlsx (zip) header PK, got %v", content[:min(2, len(content))])
	}
	if reader.gotFilter.Area != "TXT" {
		t.Errorf("filter area = %q, want TXT", reader.gotFilter.Area)
	}
}

func TestExportDailyPerformanceEmptyStillHasHeader(t *testing.T) {
	reader := &stubExportReader{snaps: nil}
	exp := NewExporter(reader)

	content, err := exp.ExportDailyPerformance(context.Background(), time.Now(), "")
	if err != nil {
		t.Fatalf("ExportDailyPerformance error: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty workbook even with no snapshots (header row)")
	}
}
