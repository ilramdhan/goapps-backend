// Package costrequestcomment holds the comment thread use cases.
package costrequestcomment

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	cprapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	cprdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costrequestcomment"
)

// CreateCommand input.
type CreateCommand struct {
	RequestID        int64
	ParentCommentID  int64
	AuthorUserID     string
	AuthorName       string // display name for notifications
	BodyRichtext     string
	BodyPlaintext    string
	MentionedUserIDs []string
}

// CreateHandler creates a comment.
type CreateHandler struct {
	repo        domain.Repository
	cprRepo     cprdomain.Repository // optional: fetch CPR for notification
	cprNotifier cprapp.CPRNotifier   // optional: best-effort CPR_COMMENT_ADDED event
}

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(r domain.Repository) *CreateHandler { return &CreateHandler{repo: r} }

// WithCPRNotifier attaches a notifier so that CPR_COMMENT_ADDED events are emitted
// after a comment is saved. Both arguments must be non-nil.
func (h *CreateHandler) WithCPRNotifier(repo cprdomain.Repository, notifier cprapp.CPRNotifier) *CreateHandler {
	h.cprRepo = repo
	h.cprNotifier = notifier
	return h
}

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
	h.emitCommentAdded(ctx, cmd.RequestID, cmd.AuthorName, cmd.MentionedUserIDs)
	return c, nil
}

// emitCommentAdded fires CPR_COMMENT_ADDED (and CPR_MENTIONED per mention) best-effort.
func (h *CreateHandler) emitCommentAdded(ctx context.Context, requestID int64, authorName string, mentionedUserIDs []string) {
	if h.cprNotifier == nil || h.cprRepo == nil {
		return
	}
	req, err := h.cprRepo.GetByID(ctx, requestID)
	if err != nil {
		log.Warn().Err(err).Int64("request_id", requestID).
			Msg("CreateCommentHandler: fetch CPR for notification failed (non-fatal)")
		return
	}
	rules := []cprapp.CPRNotifRule{
		{RuleType: "BY_PERMISSION", Value: "finance.product.request.review"},
	}
	if _, err := uuid.Parse(req.RequesterUserID()); err == nil {
		rules = append([]cprapp.CPRNotifRule{{RuleType: "BY_USER_ID", Value: req.RequesterUserID()}}, rules...)
	}
	event := cprapp.CPREvent{
		EventType:       "CPR_COMMENT_ADDED",
		RequestID:       requestID,
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		ActorName:       authorName,
		Rules:           rules,
	}
	if notifyErr := h.cprNotifier.NotifyEvent(ctx, event); notifyErr != nil {
		log.Warn().Err(notifyErr).Int64("request_id", requestID).
			Msg("CreateCommentHandler: emit CPR_COMMENT_ADDED failed (non-fatal)")
	}
	// Emit a separate CPR_MENTIONED notification per mentioned user.
	for _, uid := range dedupStrings(mentionedUserIDs) {
		mentionEvent := cprapp.CPREvent{
			EventType:       "CPR_MENTIONED",
			RequestID:       requestID,
			RequestNo:       req.RequestNo(),
			RequesterUserID: req.RequesterUserID(),
			ActorName:       authorName,
			Rules:           []cprapp.CPRNotifRule{{RuleType: "BY_USER_ID", Value: uid}},
		}
		if notifyErr := h.cprNotifier.NotifyEvent(ctx, mentionEvent); notifyErr != nil {
			log.Warn().Err(notifyErr).Str("user_id", uid).
				Msg("CreateCommentHandler: emit CPR_MENTIONED failed (non-fatal)")
		}
	}
}

func dedupStrings(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
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
func NewListByRequestHandler(r domain.Repository) *ListByRequestHandler {
	return &ListByRequestHandler{repo: r}
}

// Handle returns the comments.
func (h *ListByRequestHandler) Handle(ctx context.Context, q ListByRequestQuery) ([]*domain.Comment, error) {
	return h.repo.ListByRequest(ctx, q.RequestID, q.IncludeHidden)
}

// ListEditHistoryQuery input.
type ListEditHistoryQuery struct{ CommentID int64 }

// ListEditHistoryHandler returns CCEH_ rows for one comment.
type ListEditHistoryHandler struct{ repo domain.Repository }

// NewListEditHistoryHandler constructs.
func NewListEditHistoryHandler(r domain.Repository) *ListEditHistoryHandler {
	return &ListEditHistoryHandler{repo: r}
}

// Handle returns the history.
func (h *ListEditHistoryHandler) Handle(ctx context.Context, q ListEditHistoryQuery) ([]domain.EditHistoryEntry, error) {
	return h.repo.ListEditHistory(ctx, q.CommentID)
}
