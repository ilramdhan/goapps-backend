// Package costrequestcomment is the cost_request_comment aggregate
// (PRD Phase A §7.1.7) with embedded edit history and mention value objects.
package costrequestcomment

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// Sentinel errors.
var (
	// ErrNotFound when a comment is missing.
	ErrNotFound = errors.New("cost request comment not found")
	// ErrInvalidBody when the body fails validation.
	ErrInvalidBody = errors.New("invalid comment body")
	// ErrHiddenReasonRequired when hiding without a reason.
	ErrHiddenReasonRequired = errors.New("hidden reason required")
	// ErrNotAuthor when a non-author tries to edit.
	ErrNotAuthor = errors.New("only the comment author may edit")
)

// @-mention pattern: "@" followed by 1..64 alphanumerics, dot, underscore, hyphen, or @.
// Matches common username + email-prefix forms. Final match is captured w/o the leading @.
var mentionPattern = regexp.MustCompile(`@([A-Za-z0-9._@-]{1,64})`)

// EditHistoryEntry is one row of the append-only edit log (CCEH_).
type EditHistoryEntry struct {
	EditID        int64
	CommentID     int64
	BodyRichtext  string // raw JSON text
	BodyPlaintext string
	EditedBy      string
	EditedAt      time.Time
}

// Comment is the aggregate root.
type Comment struct {
	commentID        int64
	requestID        int64
	parentCommentID  *int64
	authorUserID     string
	bodyRichtext     string // raw JSON text
	bodyPlaintext    string
	isEdited         bool
	isHidden         bool
	hiddenReason     string
	createdAt        time.Time
	updatedAt        time.Time
	mentionedUserIDs []string
}

// NewInput is the create-time input.
type NewInput struct {
	RequestID        int64
	ParentCommentID  int64 // 0 = top-level
	AuthorUserID     string
	BodyRichtext     string
	BodyPlaintext    string
	MentionedUserIDs []string // optional; backend re-extracts as fallback
}

// New constructs a comment.
func New(in NewInput) (*Comment, error) {
	plain := strings.TrimSpace(in.BodyPlaintext)
	if plain == "" || strings.TrimSpace(in.BodyRichtext) == "" {
		return nil, ErrInvalidBody
	}
	now := time.Now().UTC()
	c := &Comment{
		requestID:     in.RequestID,
		authorUserID:  in.AuthorUserID,
		bodyRichtext:  in.BodyRichtext,
		bodyPlaintext: plain,
		createdAt:     now,
		updatedAt:     now,
	}
	if in.ParentCommentID > 0 {
		v := in.ParentCommentID
		c.parentCommentID = &v
	}
	c.mentionedUserIDs = mergeUnique(ExtractMentions(plain), in.MentionedUserIDs)
	return c, nil
}

// ReconstructInput rebuilds an aggregate from persistence.
type ReconstructInput struct {
	CommentID        int64
	RequestID        int64
	ParentCommentID  *int64
	AuthorUserID     string
	BodyRichtext     string
	BodyPlaintext    string
	IsEdited         bool
	IsHidden         bool
	HiddenReason     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	MentionedUserIDs []string
}

// Reconstruct rebuilds from a row.
func Reconstruct(in ReconstructInput) *Comment {
	return &Comment{
		commentID:        in.CommentID,
		requestID:        in.RequestID,
		parentCommentID:  in.ParentCommentID,
		authorUserID:     in.AuthorUserID,
		bodyRichtext:     in.BodyRichtext,
		bodyPlaintext:    in.BodyPlaintext,
		isEdited:         in.IsEdited,
		isHidden:         in.IsHidden,
		hiddenReason:     in.HiddenReason,
		createdAt:        in.CreatedAt,
		updatedAt:        in.UpdatedAt,
		mentionedUserIDs: in.MentionedUserIDs,
	}
}

// SetID is called by the repo after insert.
func (c *Comment) SetID(id int64) { c.commentID = id }

// =============================================================================
// Behavior
// =============================================================================

// EditSnapshot holds a comment's prior values so the caller can write a CCEH_
// edit-history row inside the same transaction.
type EditSnapshot struct {
	PriorBodyRichtext  string
	PriorBodyPlaintext string
}

// Edit replaces the body. Only the author may edit (caller passes editor user_id).
func (c *Comment) Edit(editorUserID, bodyRichtext, bodyPlaintext string, mentionsHint []string) (EditSnapshot, error) {
	if c.authorUserID != editorUserID {
		return EditSnapshot{}, ErrNotAuthor
	}
	plain := strings.TrimSpace(bodyPlaintext)
	if plain == "" || strings.TrimSpace(bodyRichtext) == "" {
		return EditSnapshot{}, ErrInvalidBody
	}
	prior := EditSnapshot{
		PriorBodyRichtext:  c.bodyRichtext,
		PriorBodyPlaintext: c.bodyPlaintext,
	}
	c.bodyRichtext = bodyRichtext
	c.bodyPlaintext = plain
	c.isEdited = true
	c.updatedAt = time.Now().UTC()
	c.mentionedUserIDs = mergeUnique(ExtractMentions(plain), mentionsHint)
	return prior, nil
}

// Hide flips is_hidden=true with a reason.
func (c *Comment) Hide(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrHiddenReasonRequired
	}
	c.isHidden = true
	c.hiddenReason = strings.TrimSpace(reason)
	c.updatedAt = time.Now().UTC()
	return nil
}

// Unhide flips is_hidden=false.
func (c *Comment) Unhide() {
	c.isHidden = false
	c.hiddenReason = ""
	c.updatedAt = time.Now().UTC()
}

// =============================================================================
// Mention extraction
// =============================================================================

// ExtractMentions parses @user-id tokens from the plaintext body.
// Returns a slice in first-seen order with duplicates removed.
func ExtractMentions(plaintext string) []string {
	matches := mentionPattern.FindAllStringSubmatch(plaintext, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		uid := m[1]
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		out = append(out, uid)
	}
	return out
}

func mergeUnique(a, b []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, v := range list {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// =============================================================================
// Accessors
// =============================================================================

// CommentID returns the comment id.
func (c *Comment) CommentID() int64 { return c.commentID }

// RequestID returns the request id.
func (c *Comment) RequestID() int64 { return c.requestID }

// ParentCommentID returns the parent comment id.
func (c *Comment) ParentCommentID() *int64 { return c.parentCommentID }

// AuthorUserID returns the author user id.
func (c *Comment) AuthorUserID() string { return c.authorUserID }

// BodyRichtext returns the body richtext.
func (c *Comment) BodyRichtext() string { return c.bodyRichtext }

// BodyPlaintext returns the body plaintext.
func (c *Comment) BodyPlaintext() string { return c.bodyPlaintext }

// IsEdited returns the is edited.
func (c *Comment) IsEdited() bool { return c.isEdited }

// IsHidden returns the is hidden.
func (c *Comment) IsHidden() bool { return c.isHidden }

// HiddenReason returns the hidden reason.
func (c *Comment) HiddenReason() string { return c.hiddenReason }

// CreatedAt returns the created at.
func (c *Comment) CreatedAt() time.Time { return c.createdAt }

// UpdatedAt returns the updated at.
func (c *Comment) UpdatedAt() time.Time { return c.updatedAt }

// MentionedUserIDs returns the mentioned user i ds.
func (c *Comment) MentionedUserIDs() []string {
	return append([]string(nil), c.mentionedUserIDs...)
}
