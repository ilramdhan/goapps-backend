// Package changeover provides application usecases for PPC changeover events:
// preview detection, create, get, list, start, and complete (actual capture).
package changeover

import (
	"context"
	"time"

	domain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/changeover"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// Service orchestrates changeover usecases over the domain repository, the WO
// spec source (for auto-detection), and the component-default config.
type Service struct {
	repo    domain.Repository
	specs   domain.WOSpecSource
	config  map[string]domain.ComponentDefault
	nowFunc func() time.Time
}

// NewService builds the changeover service. A nil config falls back to the PRD
// default component table.
func NewService(repo domain.Repository, specs domain.WOSpecSource, config map[string]domain.ComponentDefault) *Service {
	if config == nil {
		config = domain.DefaultComponentConfig
	}
	return &Service{repo: repo, specs: specs, config: config, nowFunc: time.Now}
}

// DetectResult is the preview of a changeover detection: the active components
// with the derived total duration, waste, and group classification.
type DetectResult struct {
	Components        []domain.Component
	DurationEstimated int32
	WasteEstimated    float64
	Group             string
}

// Detect previews the changeover components for a from->to WO transition,
// resolving each WO's spec from persistence. It does not persist anything.
func (s *Service) Detect(ctx context.Context, fromWOID, toWOID int64, deepClean bool) (DetectResult, error) {
	fromSpec, err := s.specForWO(ctx, fromWOID)
	if err != nil {
		return DetectResult{}, err
	}
	toSpec, err := s.specForWO(ctx, toWOID)
	if err != nil {
		return DetectResult{}, err
	}
	comps := domain.Detect(domain.DetectInput{From: fromSpec, To: toSpec, DeepCleanFlag: deepClean}, s.config)
	dur, waste := totals(comps)
	return DetectResult{
		Components:        comps,
		DurationEstimated: dur,
		WasteEstimated:    waste,
		Group:             domain.ClassifyGroup(dur),
	}, nil
}

// CreateCommand carries the inputs to create a changeover event. When Components
// is empty they are auto-detected from the from/to WO specs.
type CreateCommand struct {
	FromWOID   int64
	ToWOID     int64
	MachineID  int64
	DeepClean  bool
	Notes      string
	Components []domain.Component
}

// Create builds and persists a PLANNED changeover event, auto-detecting the
// component breakdown when none is supplied and resolving the machine from the
// incoming WO when not given.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.Event, error) {
	comps := cmd.Components
	if len(comps) == 0 {
		res, err := s.Detect(ctx, cmd.FromWOID, cmd.ToWOID, cmd.DeepClean)
		if err != nil {
			return nil, err
		}
		comps = res.Components
	}

	machineID := cmd.MachineID
	if machineID == 0 && s.specs != nil {
		if id, ok, err := s.specs.MachineForWO(ctx, cmd.ToWOID); err != nil {
			return nil, err
		} else if ok {
			machineID = id
		}
	}

	event, err := domain.NewEvent(cmd.FromWOID, cmd.ToWOID, machineID, comps, cmd.Notes)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, event); err != nil {
		return nil, err
	}
	return event, nil
}

// Get returns a changeover event with its components.
func (s *Service) Get(ctx context.Context, id int64) (*domain.Event, error) {
	return s.repo.GetByID(ctx, id)
}

// ListResult is a paginated changeover list.
type ListResult struct {
	Items       []*domain.Event
	CurrentPage int32
	PageSize    int32
	TotalItems  int64
	TotalPages  int32
}

// List returns changeover events matching the filter, with pagination metadata.
func (s *Service) List(ctx context.Context, filter domain.Filter) (ListResult, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return ListResult{}, err
	}
	totalPages := int32(0)
	if filter.PageSize > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))
	}
	return ListResult{
		Items:       items,
		CurrentPage: filter.Page,
		PageSize:    filter.PageSize,
		TotalItems:  total,
		TotalPages:  totalPages,
	}, nil
}

// Start marks a PLANNED changeover as in-progress.
func (s *Service) Start(ctx context.Context, id int64) (*domain.Event, error) {
	event, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := event.Start(s.nowFunc()); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateActual(ctx, event); err != nil {
		return nil, err
	}
	return event, nil
}

// UpdateActualCommand carries the actual duration/waste capture for a changeover.
type UpdateActualCommand struct {
	EventID        int64
	DurationActual int32
	WasteActual    float64
	Notes          string
}

// UpdateActual records the actual duration/waste and marks the changeover done.
func (s *Service) UpdateActual(ctx context.Context, cmd UpdateActualCommand) (*domain.Event, error) {
	event, err := s.repo.GetByID(ctx, cmd.EventID)
	if err != nil {
		return nil, err
	}
	if err := event.Complete(cmd.DurationActual, cmd.WasteActual, s.nowFunc()); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateActual(ctx, event); err != nil {
		return nil, err
	}
	return event, nil
}

// specForWO resolves a WO's spec, tolerating a nil spec source (returns an empty
// spec that yields BASE-only detection). A not-found WO likewise yields an empty
// spec rather than an error, so detection degrades to BASE only.
func (s *Service) specForWO(ctx context.Context, woID int64) (domain.Spec, error) {
	if s.specs == nil {
		return domain.Spec{}, nil
	}
	spec, _, err := s.specs.SpecForWO(ctx, woID)
	return spec, err
}

// totals sums component duration and waste for a detection preview.
func totals(comps []domain.Component) (durationMin int32, wasteKg float64) {
	for i := range comps {
		durationMin += comps[i].DurationMin()
		wasteKg += comps[i].WasteKg()
	}
	return durationMin, wasteKg
}
