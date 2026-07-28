package dailyperf

import (
	"context"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
)

// SubmitAreaShiftLogCommand carries the inputs for an area shift log. A nil Shift
// records a daily (not per-shift) entry.
type SubmitAreaShiftLogCommand struct {
	Area    string
	Date    time.Time
	Shift   *string
	OtHours *float64
	Notes   string
	InputBy int64
}

// SubmitAreaShiftLog upserts an area shift log on its (area, date, shift) key.
func (s *Service) SubmitAreaShiftLog(ctx context.Context, cmd SubmitAreaShiftLogCommand) (*dailyperf.AreaShiftLog, error) {
	log, err := dailyperf.NewAreaShiftLog(dailyperf.NewAreaShiftLogParams{
		Area:    cmd.Area,
		Date:    cmd.Date,
		Shift:   cmd.Shift,
		OtHours: cmd.OtHours,
		Notes:   cmd.Notes,
		InputBy: cmd.InputBy,
	})
	if err != nil {
		return nil, err
	}
	if err := s.areaShiftLogs.Upsert(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}
