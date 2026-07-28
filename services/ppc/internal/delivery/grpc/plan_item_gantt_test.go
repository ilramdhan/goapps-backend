package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

// A 5-day plan window must render as a 5-day bar: the end date is the deadline
// and the start is four days earlier, both endpoints inclusive. Before the
// hybrid timeline the handler faked start = deadline - 1, so every bar was one
// day wide regardless of quantity.
func TestPlanItemToGanttBar_DerivedDurationFiveSpansFourDays(t *testing.T) {
	deadline := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	start := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	days := int32(5)

	item := planitemdomain.Reconstruct(planitemdomain.ReconstructParams{
		ID:               1,
		CpmProductSysID:  100,
		Type:             planitemdomain.TypeFGDelivery,
		QtyTarget:        500,
		Deadline:         deadline,
		RMSource:         planitemdomain.RMSourceStore,
		Status:           planitemdomain.StatusConfirmed,
		MachineGroupID:   7,
		Month:            "2026-08",
		PlannedStartDate: &start,
		PlannedDuration:  &days,
		DurationSource:   planitemdomain.DurationSourceDerived,
	})

	bar := planItemToGanttBar(&planitemdomain.GanttRow{Item: item}, nil)

	require.Equal(t, "2026-08-11", bar.GetStartDate())
	require.Equal(t, "2026-08-15", bar.GetEndDate())

	s, err := time.Parse("2006-01-02", bar.GetStartDate())
	require.NoError(t, err)
	e, err := time.Parse("2006-01-02", bar.GetEndDate())
	require.NoError(t, err)
	assert.Equal(t, 4*24*time.Hour, e.Sub(s))
}

// With no stored start date the bar collapses onto the deadline rather than
// inventing a window.
func TestPlanItemToGanttBar_NoStartDateCollapsesOntoDeadline(t *testing.T) {
	deadline := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)

	item := planitemdomain.Reconstruct(planitemdomain.ReconstructParams{
		ID:              2,
		CpmProductSysID: 100,
		Type:            planitemdomain.TypeFGDelivery,
		QtyTarget:       500,
		Deadline:        deadline,
		RMSource:        planitemdomain.RMSourceStore,
		Status:          planitemdomain.StatusConfirmed,
		MachineGroupID:  7,
		Month:           "2026-08",
		DurationSource:  planitemdomain.DurationSourceDerived,
	})

	bar := planItemToGanttBar(&planitemdomain.GanttRow{Item: item}, nil)

	assert.Equal(t, "2026-08-15", bar.GetStartDate())
	assert.Equal(t, "2026-08-15", bar.GetEndDate())
}
