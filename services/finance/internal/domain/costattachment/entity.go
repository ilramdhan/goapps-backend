// Package costattachment is the cost_attachment domain (PRD Phase A §7.1.10).
// Exactly one of (RequestID, CommentID) is set per the XOR DB check.
package costattachment

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Sentinel errors.
var (
	// ErrNotFound is returned when an attachment is missing.
	ErrNotFound = errors.New("cost attachment not found")
	// ErrOwnerXor when neither or both owner FKs are set.
	ErrOwnerXor = errors.New("attachment must hang off exactly one of (request_id, comment_id)")
	// ErrInvalidFile when filename/mime/size fail validation.
	ErrInvalidFile = errors.New("invalid file metadata")
)

// MaxSizeBytes — 25 MB per PRD FR-5.
const MaxSizeBytes int64 = 25 * 1024 * 1024

// Attachment is a value-object — no lifecycle behavior beyond create/delete.
type Attachment struct {
	AttachmentID int64
	RequestID    *int64
	CommentID    *int64
	Filename     string
	MimeType     string
	SizeBytes    int64
	StorageKey   string
	UploadedBy   string
	UploadedAt   time.Time
}

// NewInput is the upload-time input.
type NewInput struct {
	RequestID  int64 // 0 if not set
	CommentID  int64 // 0 if not set
	Filename   string
	MimeType   string
	SizeBytes  int64
	StorageKey string
	UploadedBy string
}

// New constructs and validates an Attachment ready for persistence.
func New(in NewInput) (*Attachment, error) {
	if strings.TrimSpace(in.Filename) == "" || strings.TrimSpace(in.MimeType) == "" || in.SizeBytes <= 0 {
		return nil, ErrInvalidFile
	}
	if in.SizeBytes > MaxSizeBytes {
		return nil, ErrInvalidFile
	}
	hasReq := in.RequestID > 0
	hasCmt := in.CommentID > 0
	if hasReq == hasCmt {
		return nil, ErrOwnerXor
	}
	a := &Attachment{
		Filename:   strings.TrimSpace(in.Filename),
		MimeType:   strings.TrimSpace(in.MimeType),
		SizeBytes:  in.SizeBytes,
		StorageKey: strings.TrimSpace(in.StorageKey),
		UploadedBy: in.UploadedBy,
		UploadedAt: time.Now().UTC(),
	}
	if hasReq {
		v := in.RequestID
		a.RequestID = &v
	}
	if hasCmt {
		v := in.CommentID
		a.CommentID = &v
	}
	return a, nil
}

// Repository persists attachments.
type Repository interface {
	Create(ctx context.Context, a *Attachment) error
	GetByID(ctx context.Context, id int64) (*Attachment, error)
	ListByRequest(ctx context.Context, requestID int64) ([]*Attachment, error)
	ListByComment(ctx context.Context, commentID int64) ([]*Attachment, error)
	Delete(ctx context.Context, id int64) error
}
