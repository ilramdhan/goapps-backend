package dailyperf_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dpapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/dailyperf"
	dpdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeShiftLogRepo struct {
	upserted []*dpdomain.MachineShiftLog
	nextID   int64
}

func (f *fakeShiftLogRepo) Upsert(_ context.Context, log *dpdomain.MachineShiftLog) error {
	f.nextID++
	log.SetID(f.nextID)
	f.upserted = append(f.upserted, log)
	return nil
}

func (f *fakeShiftLogRepo) GetByKey(_ context.Context, _ int64, _ time.Time, _ string) (*dpdomain.MachineShiftLog, error) {
	return nil, dpdomain.ErrShiftLogNotFound
}

func (f *fakeShiftLogRepo) GetByID(_ context.Context, _ int64) (*dpdomain.MachineShiftLog, error) {
	return nil, dpdomain.ErrShiftLogNotFound
}

func (f *fakeShiftLogRepo) List(_ context.Context, _ dpdomain.ShiftLogFilter) ([]*dpdomain.MachineShiftLog, int64, error) {
	return f.upserted, int64(len(f.upserted)), nil
}

type fakeDowntimeRepo struct{ events []*dpdomain.DowntimeEvent }

func (f *fakeDowntimeRepo) ReplaceForShiftLog(_ context.Context, _ int64, events []*dpdomain.DowntimeEvent) error {
	f.events = events
	return nil
}

type fakeWasteRepo struct{ rows []*dpdomain.WasteActual }

func (f *fakeWasteRepo) ReplaceForShiftLog(_ context.Context, _ int64, rows []*dpdomain.WasteActual) error {
	f.rows = rows
	return nil
}

type fakeSnapshotRepo struct {
	upserted []*dpdomain.EfficiencySnapshot
}

func (f *fakeSnapshotRepo) Upsert(_ context.Context, snap *dpdomain.EfficiencySnapshot) error {
	f.upserted = append(f.upserted, snap)
	return nil
}

func (f *fakeSnapshotRepo) DeleteScope(_ context.Context, _ string, _ time.Time, _ *int64) error {
	return nil
}

type fakeProdReader struct{ rows []dpdomain.ProductionActual }

func (f *fakeProdReader) ProductionActuals(_ context.Context, _ string, _ time.Time, _ *int64, _ *string) ([]dpdomain.ProductionActual, error) {
	return f.rows, nil
}

type fakeShiftLogReader struct{ logs []*dpdomain.MachineShiftLog }

func (f *fakeShiftLogReader) ShiftLogsForArea(_ context.Context, _ string, _ time.Time, _ *int64, _ *string) ([]*dpdomain.MachineShiftLog, error) {
	return f.logs, nil
}

type staticWellKnown struct{ p dpdomain.WellKnownParams }

func (s staticWellKnown) WellKnown(_ context.Context, _, _ int64) (dpdomain.WellKnownParams, error) {
	return s.p, nil
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestSubmitShiftEntry_DerivesRunningMinutesFromDowntime(t *testing.T) {
	shiftRepo := &fakeShiftLogRepo{}
	downRepo := &fakeDowntimeRepo{}
	wasteRepo := &fakeWasteRepo{}
	svc := dpapp.NewService(dpapp.Deps{
		ShiftLogs: shiftRepo,
		Downtime:  downRepo,
		Waste:     wasteRepo,
		WellKnown: staticWellKnown{p: dpdomain.WellKnownParams{Positions: 48, Speed: 3000, Denier: 150}},
	})

	dur := int32(90)
	wo := int64(5)
	log, err := svc.SubmitShiftEntry(context.Background(), dpapp.SubmitShiftEntryCommand{
		MachineID:      10,
		Date:           time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC),
		Shift:          "1",
		PositionsTotal: 48,
		Status:         dpdomain.StatusDraft,
		Downtime:       []dpapp.DowntimeInput{{ReasonID: 1, WoID: &wo, DurationMin: &dur}},
		InputBy:        7,
	})
	require.NoError(t, err)
	// 480 - 90 = 390 running minutes.
	assert.Equal(t, int32(390), log.RunningMinutes())
	require.Len(t, downRepo.events, 1)
	// lost_kg auto-computed: 48 * 90 * 3000 * 150 / 9_000_000 = 216.
	require.NotNil(t, downRepo.events[0].LostKg)
	assert.InDelta(t, 216.0, *downRepo.events[0].LostKg, 1e-9)
}

func TestSubmitShiftEntry_RunningMinutesFlooredAtZero(t *testing.T) {
	svc := dpapp.NewService(dpapp.Deps{ShiftLogs: &fakeShiftLogRepo{}})
	dur := int32(600) // exceeds full shift
	log, err := svc.SubmitShiftEntry(context.Background(), dpapp.SubmitShiftEntryCommand{
		MachineID: 1, Date: time.Now(), Shift: "2", Status: dpdomain.StatusDraft,
		Downtime: []dpapp.DowntimeInput{{ReasonID: 1, DurationMin: &dur}},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(0), log.RunningMinutes())
}

func TestRecalc_WritesShiftDayAndAreaSnapshots(t *testing.T) {
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	snapRepo := &fakeSnapshotRepo{}
	svc := dpapp.NewService(dpapp.Deps{
		Snapshots: snapRepo,
		ProductionReader: &fakeProdReader{rows: []dpdomain.ProductionActual{
			{WoID: 1, MachineID: 10, Date: date, Shift: "1", QtyActual: 1000, Breaks: 6, ProdCategory: "NORMAL", Segment: "DTY"},
			{WoID: 2, MachineID: 10, Date: date, Shift: "2", QtyActual: 500, Breaks: 3, ProdCategory: "TRIAL", Segment: "DTY"},
		}},
		ShiftLogReader: &fakeShiftLogReader{logs: []*dpdomain.MachineShiftLog{
			dpdomain.ReconstructMachineShiftLog(dpdomain.ReconstructShiftLogParams{ID: 1, MachineID: 10, Date: date, Shift: "1", PositionsTotal: 48, PositionsRunning: 48, RunningMinutes: 480}),
			dpdomain.ReconstructMachineShiftLog(dpdomain.ReconstructShiftLogParams{ID: 2, MachineID: 10, Date: date, Shift: "2", PositionsTotal: 48, PositionsRunning: 48, RunningMinutes: 480}),
		}},
		WellKnown: staticWellKnown{p: dpdomain.WellKnownParams{Positions: 48, Speed: 3000, Denier: 150}},
	})

	written, err := svc.Recalc(context.Background(), "TXT", date, nil, nil)
	require.NoError(t, err)
	// 2 shifts × 2 variants (incl/excl) = 4 MACHINE_SHIFT;
	// 1 machine × 2 variants = 2 MACHINE_DAY; 2 variants = 2 AREA_DAY. Total 8.
	assert.Equal(t, 8, written)
	assert.Len(t, snapRepo.upserted, 8)

	// The Excluding AREA_DAY must aggregate only NORMAL production (shift 1 = 1000),
	// while Including AREA_DAY aggregates all (1500) — proving re-aggregation.
	var inclArea, exclArea *dpdomain.EfficiencySnapshot
	for _, s := range snapRepo.upserted {
		if s.Scope == dpdomain.ScopeAreaDay {
			if s.IsExcluding {
				exclArea = s
			} else {
				inclArea = s
			}
		}
	}
	require.NotNil(t, inclArea)
	require.NotNil(t, exclArea)
	assert.InDelta(t, 1500.0, inclArea.QtyActual, 1e-9)
	assert.InDelta(t, 1000.0, exclArea.QtyActual, 1e-9)
	// Day theoretical_100 re-aggregates both shifts: 2 × 1152 = 2304.
	assert.InDelta(t, 2304.0, inclArea.QtyTheoretical100, 1e-9)
	// Production eff for Including = 1500 / 2304 × 100 = 65.104...%.
	assert.InDelta(t, 65.10416667, inclArea.EffProductionPct, 1e-6)
}

func TestRecalc_NilDepsNoop(t *testing.T) {
	svc := dpapp.NewService(dpapp.Deps{})
	written, err := svc.Recalc(context.Background(), "TXT", time.Now(), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, written)
}
