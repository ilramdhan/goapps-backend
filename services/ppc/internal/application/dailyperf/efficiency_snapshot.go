package dailyperf

import (
	"context"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// prodCategoryNormal is the only production category counted by the Excluding
// efficiency variant.
const prodCategoryNormal = "NORMAL"

// shiftKey identifies a per-machine, per-shift aggregation bucket.
type shiftKey struct {
	machineID int64
	shift     string
}

// msAgg is the gathered per-machine-shift aggregate feeding snapshot computation.
type msAgg struct {
	machineID        int64
	shift            string
	segment          string
	woForParams      int64
	qtyActualIncl    float64
	qtyActualExcl    float64
	breaks           int
	lossIncl         float64
	lossExcl         float64
	waste            float64
	positionsTotal   float64
	positionsRunning float64
	runMinIncl       float64
	runMinExcl       float64
}

// Recalc recomputes efficiency snapshots for an area on a date, optionally scoped
// to one machine and/or shift. It computes MACHINE_SHIFT snapshots (Including and
// Excluding variants), then re-aggregates MACHINE_DAY and AREA_DAY rollups
// (summing components, never averaging percentages). Returns the number of
// snapshot rows written.
func (s *Service) Recalc(ctx context.Context, areaCode string, date time.Time, machineID *int64, shift *string) (int, error) {
	if s.snapshots == nil || s.productionReader == nil {
		return 0, nil
	}

	aggs, err := s.gather(ctx, areaCode, date, machineID, shift)
	if err != nil {
		return 0, err
	}

	machineShift := s.buildMachineShiftSnapshots(ctx, areaCode, date, aggs)
	machineDay := rollupMachineDay(areaCode, date, machineShift)
	areaDay := rollupAreaDay(areaCode, date, machineShift)

	all := make([]*dailyperf.EfficiencySnapshot, 0, len(machineShift)+len(machineDay)+len(areaDay))
	all = append(all, machineShift...)
	all = append(all, machineDay...)
	all = append(all, areaDay...)

	if err := s.snapshots.DeleteScope(ctx, areaCode, date, machineID); err != nil {
		return 0, err
	}
	for _, snap := range all {
		if err := s.snapshots.Upsert(ctx, snap); err != nil {
			return 0, err
		}
	}
	return len(all), nil
}

// gather reads production, downtime, waste, and shift-log data and folds it into
// per-machine-shift aggregates.
func (s *Service) gather(ctx context.Context, areaCode string, date time.Time, machineID *int64, shift *string) (map[shiftKey]*msAgg, error) {
	aggs := make(map[shiftKey]*msAgg)

	prod, err := s.productionReader.ProductionActuals(ctx, areaCode, date, machineID, shift)
	if err != nil {
		return nil, err
	}
	for _, p := range prod {
		a := ensureAgg(aggs, p.MachineID, p.Shift)
		a.qtyActualIncl += p.QtyActual
		if p.ProdCategory == prodCategoryNormal {
			a.qtyActualExcl += p.QtyActual
		}
		a.breaks += p.Breaks
		if a.segment == "" {
			a.segment = p.Segment
		}
		if a.woForParams == 0 {
			a.woForParams = p.WoID
		}
	}

	s.foldDowntime(ctx, aggs, areaCode, date, machineID, shift)
	s.foldWaste(ctx, aggs, areaCode, date, machineID, shift)
	s.foldShiftLogs(ctx, aggs, areaCode, date, machineID, shift)
	return aggs, nil
}

func (s *Service) foldDowntime(ctx context.Context, aggs map[shiftKey]*msAgg, areaCode string, date time.Time, machineID *int64, shift *string) {
	if s.downtimeReader == nil {
		return
	}
	rows, err := s.downtimeReader.DowntimeAggregates(ctx, areaCode, date, machineID, shift)
	if err != nil {
		return
	}
	for _, d := range rows {
		a := ensureAgg(aggs, d.MachineID, d.Shift)
		a.lossIncl += d.LostKg
		a.lossExcl += d.LostKg - d.ExcludedLostKg
	}
}

func (s *Service) foldWaste(ctx context.Context, aggs map[shiftKey]*msAgg, areaCode string, date time.Time, machineID *int64, shift *string) {
	if s.wasteReader == nil {
		return
	}
	rows, err := s.wasteReader.WasteAggregates(ctx, areaCode, date, machineID, shift)
	if err != nil {
		return
	}
	for _, w := range rows {
		a := ensureAgg(aggs, w.MachineID, w.Shift)
		a.waste += w.QtyKg
	}
}

func (s *Service) foldShiftLogs(ctx context.Context, aggs map[shiftKey]*msAgg, areaCode string, date time.Time, machineID *int64, shift *string) {
	if s.shiftLogReader == nil {
		return
	}
	logs, err := s.shiftLogReader.ShiftLogsForArea(ctx, areaCode, date, machineID, shift)
	if err != nil {
		return
	}
	for _, l := range logs {
		a := ensureAgg(aggs, l.MachineID(), l.Shift())
		a.positionsTotal = float64(l.PositionsTotal())
		a.positionsRunning = l.PositionsRunning()
		a.runMinIncl = float64(l.RunningMinutes())
		// Excluding variant credits back running time lost to excluded reasons
		// (e.g. power failure): it never runs faster than a full shift.
		a.runMinExcl = a.runMinIncl
		if a.runMinExcl > dailyperf.FullShiftMinutes {
			a.runMinExcl = dailyperf.FullShiftMinutes
		}
	}
}

func ensureAgg(aggs map[shiftKey]*msAgg, machineID int64, shift string) *msAgg {
	k := shiftKey{machineID: machineID, shift: shift}
	a, ok := aggs[k]
	if !ok {
		a = &msAgg{machineID: machineID, shift: shift}
		aggs[k] = a
	}
	return a
}

// buildMachineShiftSnapshots computes the Including and Excluding MACHINE_SHIFT
// snapshots for every gathered aggregate.
func (s *Service) buildMachineShiftSnapshots(ctx context.Context, areaCode string, date time.Time, aggs map[shiftKey]*msAgg) []*dailyperf.EfficiencySnapshot {
	out := make([]*dailyperf.EfficiencySnapshot, 0, len(aggs)*2)
	for _, a := range aggs {
		params := s.resolveParams(ctx, a.machineID, a.woForParams)
		out = append(out, s.computeShiftSnapshot(areaCode, date, a, params, false))
		out = append(out, s.computeShiftSnapshot(areaCode, date, a, params, true))
	}
	return out
}

func (s *Service) resolveParams(ctx context.Context, machineID, woID int64) dailyperf.WellKnownParams {
	if s.wellKnown == nil || woID == 0 {
		return dailyperf.WellKnownParams{}
	}
	params, err := s.wellKnown.WellKnown(ctx, machineID, woID)
	if err != nil {
		return dailyperf.WellKnownParams{}
	}
	return params
}

// computeShiftSnapshot builds one MACHINE_SHIFT snapshot for the given variant.
func (s *Service) computeShiftSnapshot(areaCode string, date time.Time, a *msAgg, params dailyperf.WellKnownParams, excluding bool) *dailyperf.EfficiencySnapshot {
	qtyActual, loss, runMin := a.qtyActualIncl, a.lossIncl, a.runMinIncl
	if excluding {
		qtyActual, loss, runMin = a.qtyActualExcl, a.lossExcl, a.runMinExcl
	}

	theo100 := s.calc.Theoretical(dailyperf.TheoreticalInput{
		Positions: a.positionsTotal, RunningMinutes: dailyperf.FullShiftMinutes, Speed: params.Speed, Denier: params.Denier,
	})
	theoRng := s.calc.Theoretical(dailyperf.TheoreticalInput{
		Positions: a.positionsRunning, RunningMinutes: runMin, Speed: params.Speed, Denier: params.Denier,
	})

	machineID := a.machineID
	shift := a.shift
	segment := a.segment
	snap := &dailyperf.EfficiencySnapshot{
		Area:              areaCode,
		Scope:             dailyperf.ScopeMachineShift,
		MachineID:         &machineID,
		Date:              date,
		Shift:             &shift,
		IsExcluding:       excluding,
		QtyTheoretical100: theo100,
		QtyTheoreticalRng: theoRng,
		QtyLoss:           loss,
		QtyWaste:          a.waste,
		QtyActual:         qtyActual,
		BreaksCount:       safeconv.IntToInt32(a.breaks),
		CalcAt:            time.Now(),
	}
	if segment != "" {
		snap.Segment = &segment
	}
	s.applyEffPercentages(snap)
	return snap
}

// applyEffPercentages derives all percentage metrics from the snapshot's
// quantity fields, so it works identically for a leaf snapshot and a rollup.
func (s *Service) applyEffPercentages(snap *dailyperf.EfficiencySnapshot) {
	snap.EffProductionPct = s.calc.ProductionEff(snap.QtyActual, snap.QtyTheoretical100)
	snap.EffRunningPct = s.calc.RunningEff(snap.QtyActual, snap.QtyTheoreticalRng)
	snap.WastePct = s.calc.WastePct(snap.QtyActual, snap.QtyWaste)
	snap.BreaksPerTon = s.calc.BreaksPerTon(int(snap.BreaksCount), snap.QtyActual)
	// SPG plant efficiency uses the DOFFED qty (qty_actual is seeded from
	// GROSS_BOBBINS for SPG by the ETL), over the 100% nominal theoretical — can
	// exceed 100%. Yield is the non-waste fraction of gross output.
	snap.EffPlantPct = s.calc.PlantEff(snap.QtyActual, snap.QtyTheoretical100)
	snap.YieldPct = s.calc.Yield(snap.QtyActual, snap.QtyWaste)
}

// rollupMachineDay re-aggregates MACHINE_SHIFT snapshots into per-machine daily
// snapshots, one per (machine, excluding) variant.
func rollupMachineDay(areaCode string, date time.Time, leaves []*dailyperf.EfficiencySnapshot) []*dailyperf.EfficiencySnapshot {
	type key struct {
		machineID int64
		excluding bool
	}
	groups := make(map[key][]*dailyperf.EfficiencySnapshot)
	for _, l := range leaves {
		if l.MachineID == nil {
			continue
		}
		k := key{machineID: *l.MachineID, excluding: l.IsExcluding}
		groups[k] = append(groups[k], l)
	}

	out := make([]*dailyperf.EfficiencySnapshot, 0, len(groups))
	for k, members := range groups {
		machineID := k.machineID
		snap := sumSnapshots(members)
		snap.Area = areaCode
		snap.Scope = dailyperf.ScopeMachineDay
		snap.MachineID = &machineID
		snap.Date = date
		snap.IsExcluding = k.excluding
		out = append(out, snap)
	}
	return out
}

// rollupAreaDay re-aggregates MACHINE_SHIFT snapshots into per-area daily
// snapshots, one per excluding variant.
func rollupAreaDay(areaCode string, date time.Time, leaves []*dailyperf.EfficiencySnapshot) []*dailyperf.EfficiencySnapshot {
	groups := make(map[bool][]*dailyperf.EfficiencySnapshot)
	for _, l := range leaves {
		groups[l.IsExcluding] = append(groups[l.IsExcluding], l)
	}

	out := make([]*dailyperf.EfficiencySnapshot, 0, len(groups))
	for excluding, members := range groups {
		snap := sumSnapshots(members)
		snap.Area = areaCode
		snap.Scope = dailyperf.ScopeAreaDay
		snap.Date = date
		snap.IsExcluding = excluding
		out = append(out, snap)
	}
	return out
}

// sumSnapshots sums the additive quantity fields of a set of snapshots and
// recomputes the percentage metrics from the sums (re-aggregation, not average).
func sumSnapshots(members []*dailyperf.EfficiencySnapshot) *dailyperf.EfficiencySnapshot {
	snap := &dailyperf.EfficiencySnapshot{CalcAt: time.Now()}
	var breaks int32
	for _, m := range members {
		snap.QtyTheoretical100 += m.QtyTheoretical100
		snap.QtyTheoreticalRng += m.QtyTheoreticalRng
		snap.QtyLoss += m.QtyLoss
		snap.QtyWaste += m.QtyWaste
		snap.QtyActual += m.QtyActual
		breaks += m.BreaksCount
	}
	snap.BreaksCount = breaks
	calc := dailyperf.NewEfficiencyCalculator()
	snap.EffProductionPct = calc.ProductionEff(snap.QtyActual, snap.QtyTheoretical100)
	snap.EffRunningPct = calc.RunningEff(snap.QtyActual, snap.QtyTheoreticalRng)
	snap.WastePct = calc.WastePct(snap.QtyActual, snap.QtyWaste)
	snap.BreaksPerTon = calc.BreaksPerTon(int(snap.BreaksCount), snap.QtyActual)
	snap.EffPlantPct = calc.PlantEff(snap.QtyActual, snap.QtyTheoretical100)
	snap.YieldPct = calc.Yield(snap.QtyActual, snap.QtyWaste)
	return snap
}
