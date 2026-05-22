// Package costrequestcomment holds the comment thread use cases.
package costrequestcomment

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costrequestcomment"
)

// CreateCommand input.
type CreateCommand struct {
	RequestID        int64
	ParentCommentID  int64
	AuthorUserID     string
	BodyRichtext     string
	BodyPlaintext    string
	MentionedUserIDs []string
}

// CreateHandler creates a comment.
type CreateHandler struct{ repo domain.Repository }

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(r domain.Repository) *CreateHandler { return &CreateHandler{repo: r} }

// Handle executes the create.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*domain.Comment, error) {
	c, err := domain.New(domain.NewInput{
		RequestID:        cmd.RequestID,
		ParentCommentID:  cmd.ParentCommentID,
		AuthorUserID:     cmd.AuthorUserID,
		BodyRichtext:     cmd.BodyRichtext,
		BodyPlaintext:    cmd.BodyPlaintext,
		MentionedUserIDs: cmd.MentionedUserIDs,
	})
	if err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// UpdateCommand input.
type UpdateCommand struct {
	CommentID        int64
	EditorUserID     string
	BodyRichtext     string
	BodyPlaintext    string
	MentionedUserIDs []string
}

// UpdateHandler edits an existing comment.
type UpdateHandler struct{ repo domain.Repository }

// NewUpdateHandler constructs an UpdateHandler.
func NewUpdateHandler(r domain.Repository) *UpdateHandler { return &UpdateHandler{repo: r} }

// Handle executes the edit + snapshot.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*domain.Comment, error) {
	c, err := h.repo.GetByID(ctx, cmd.CommentID)
	if err != nil {
		return nil, err
	}
	snap, err := c.Edit(cmd.EditorUserID, cmd.BodyRichtext, cmd.BodyPlaintext, cmd.MentionedUserIDs)
	if err != nil {
		return nil, err
	}
	if err := h.repo.Update(ctx, c, snap, cmd.EditorUserID); err != nil {
		return nil, err
	}
	return c, nil
}

// HideCommand input.
type HideCommand struct {
	CommentID    int64
	HiddenReason string
}

// HideHandler hides a comment.
type HideHandler struct{ repo domain.Repository }

// NewHideHandler constructs a HideHandler.
func NewHideHandler(r domain.Repository) *HideHandler { return &HideHandler{repo: r} }

// Handle executes the hide.
func (h *HideHandler) Handle(ctx context.Context, cmd HideCommand) (*domain.Comment, error) {
	c, err := h.repo.GetByID(ctx, cmd.CommentID)
	if err != nil {
		return nil, err
	}
	if err := c.Hide(cmd.HiddenReason); err != nil {
		return nil, err
	}
	if err := h.repo.UpdateHidden(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// UnhideCommand input.
type UnhideCommand struct{ CommentID int64 }

// UnhideHandler reverses a hide.
type UnhideHandler struct{ repo domain.Repository }

// NewUnhideHandler constructs an UnhideHandler.
func NewUnhideHandler(r domain.Repository) *UnhideHandler { return &UnhideHandler{repo: r} }

// Handle executes the unhide.
func (h *UnhideHandler) Handle(ctx context.Context, cmd UnhideCommand) (*domain.Comment, error) {
	c, err := h.repo.GetByID(ctx, cmd.CommentID)
	if err != nil {
		return nil, err
	}
	c.Unhide()
	if err := h.repo.UpdateHidden(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// DeleteCommand input.
type DeleteCommand struct{ CommentID int64 }

// DeleteHandler deletes a comment.
type DeleteHandler struct{ repo domain.Repository }

// NewDeleteHandler constructs a DeleteHandler.
func NewDeleteHandler(r domain.Repository) *DeleteHandler { return &DeleteHandler{repo: r} }

// Handle executes the delete.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.Delete(ctx, cmd.CommentID)
}

// ListByRequestQuery input.
type ListByRequestQuery struct {
	RequestID     int64
	IncludeHidden bool
}

// ListByRequestHandler returns the thread.
type ListByRequestHandler struct{ repo domain.Repository }

// NewListByRequestHandler constructs.
func NewListByRequestHandler(r domain.Repository) *ListByRequestHandler { return &ListByRequestHandler{repo: r} }

// Handle returns the comments.
func (h *ListByRequestHandler) Handle(ctx context.Context, q ListByRequestQuery) ([]*domain.Comment, error) {
	return h.repo.ListByRequest(ctx, q.RequestID, q.IncludeHidden)
}

// ListEditHistoryQuery input.
type ListEditHistoryQuery struct{ CommentID int64 }

// ListEditHistoryHandler returns CCEH_ rows for one comment.
type ListEditHistoryHandler struct{ repo domain.Repository }

// NewListEditHistoryHandler constructs.
func NewListEditHistoryHandler(r domain.Repository) *ListEditHistoryHandler { return &ListEditHistoryHandler{repo: r} }

// Handle returns the history.
func (h *ListEditHistoryHandler) Handle(ctx context.Context, q ListEditHistoryQuery) ([]domain.EditHistoryEntry, error) {
	return h.repo.ListEditHistory(ctx, q.CommentID)
}
