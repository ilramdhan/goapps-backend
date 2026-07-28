package dailyperf

import (
	"context"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// CreateNoteCommand carries the inputs for creating a shift-log note.
type CreateNoteCommand struct {
	MachineID int64
	Date      time.Time
	Shift     string
	NoteType  string
	Note      string
	InputBy   int64
}

// CreateNote validates and persists a new shift-log note.
func (s *Service) CreateNote(ctx context.Context, cmd CreateNoteCommand) (*dailyperf.ShiftLogNote, error) {
	note, err := dailyperf.NewShiftLogNote(dailyperf.NewShiftLogNoteParams{
		MachineID: cmd.MachineID,
		Date:      cmd.Date,
		Shift:     cmd.Shift,
		NoteType:  cmd.NoteType,
		Note:      cmd.Note,
		InputBy:   cmd.InputBy,
	})
	if err != nil {
		return nil, err
	}
	if err := s.notes.Create(ctx, note); err != nil {
		return nil, err
	}
	note.SetMachineNo(s.resolveMachineNo(ctx, cmd.MachineID))
	return note, nil
}

// UpdateNoteCommand carries the inputs for updating a shift-log note.
type UpdateNoteCommand struct {
	ID       int64
	NoteType *string
	Note     *string
}

// UpdateNote mutates an existing shift-log note.
func (s *Service) UpdateNote(ctx context.Context, cmd UpdateNoteCommand) (*dailyperf.ShiftLogNote, error) {
	note, err := s.notes.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := note.Update(cmd.NoteType, cmd.Note); err != nil {
		return nil, err
	}
	if err := s.notes.Update(ctx, note); err != nil {
		return nil, err
	}
	note.SetMachineNo(s.resolveMachineNo(ctx, note.MachineID()))
	return note, nil
}

// DeleteNote removes a shift-log note by id.
func (s *Service) DeleteNote(ctx context.Context, id int64) error {
	return s.notes.Delete(ctx, id)
}

// GetNote retrieves a shift-log note by id.
func (s *Service) GetNote(ctx context.Context, id int64) (*dailyperf.ShiftLogNote, error) {
	return s.notes.GetByID(ctx, id)
}

// ListNotesQuery carries inputs for listing shift-log notes.
type ListNotesQuery struct {
	Page      int
	PageSize  int
	MachineID *int64
	Date      *time.Time
	Shift     string
	NoteType  string
	SortBy    string
	SortOrder string
}

// ListNotesResult holds a page of shift-log notes plus pagination metadata.
type ListNotesResult struct {
	Items       []*dailyperf.ShiftLogNote
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListNotes retrieves a filtered, paginated page of shift-log notes.
func (s *Service) ListNotes(ctx context.Context, query ListNotesQuery) (*ListNotesResult, error) {
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	items, total, err := s.notes.List(ctx, dailyperf.ShiftLogNoteFilter{
		MachineID: query.MachineID,
		Date:      query.Date,
		Shift:     query.Shift,
		NoteType:  query.NoteType,
		Page:      page,
		PageSize:  pageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	})
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &ListNotesResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(page),
		PageSize:    safeconv.IntToInt32(pageSize),
	}, nil
}
