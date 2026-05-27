// Package costattachment uploads + manages cost_attachment files.
package costattachment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costattachment"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// ErrStorageRequired is returned when no MinIO service is configured.
var ErrStorageRequired = errors.New("MinIO storage service is required for attachments")

// UploadCommand input.
type UploadCommand struct {
	RequestID   int64
	CommentID   int64
	Filename    string
	MimeType    string
	FileContent []byte
	UploadedBy  string
}

// UploadHandler uploads the file to MinIO and persists the metadata.
type UploadHandler struct {
	repo    domain.Repository
	storage storage.Service
}

// NewUploadHandler constructs an UploadHandler.
func NewUploadHandler(repo domain.Repository, svc storage.Service) *UploadHandler {
	return &UploadHandler{repo: repo, storage: svc}
}

// Handle uploads to object storage then persists the metadata row.
// Storage key format: cost-attachments/{request|comment}-{id}/{uuid}-{filename}.
func (h *UploadHandler) Handle(ctx context.Context, cmd UploadCommand) (*domain.Attachment, error) {
	if h.storage == nil {
		return nil, ErrStorageRequired
	}
	owner := "request"
	ownerID := cmd.RequestID
	if cmd.CommentID > 0 {
		owner = "comment"
		ownerID = cmd.CommentID
	}
	key := fmt.Sprintf("cost-attachments/%s-%d/%s-%s",
		owner, ownerID,
		uuid.NewString(),
		sanitizeFilename(cmd.Filename),
	)
	size := int64(len(cmd.FileContent))
	if err := h.storage.PutObject(ctx, key, bytes.NewReader(cmd.FileContent), size, cmd.MimeType); err != nil {
		return nil, fmt.Errorf("upload to storage: %w", err)
	}
	a, err := domain.New(domain.NewInput{
		RequestID:  cmd.RequestID,
		CommentID:  cmd.CommentID,
		Filename:   cmd.Filename,
		MimeType:   cmd.MimeType,
		SizeBytes:  size,
		StorageKey: key,
		UploadedBy: cmd.UploadedBy,
	})
	if err != nil {
		// Best-effort cleanup if the metadata is invalid.
		if e := h.storage.RemoveObject(ctx, key); e != nil {
			_ = e
		}
		return nil, err
	}
	if err := h.repo.Create(ctx, a); err != nil {
		if e := h.storage.RemoveObject(ctx, key); e != nil {
			_ = e
		}
		return nil, err
	}
	return a, nil
}

// ListByRequestHandler returns request-level attachments.
type ListByRequestHandler struct{ repo domain.Repository }

// NewListByRequestHandler constructs.
func NewListByRequestHandler(r domain.Repository) *ListByRequestHandler {
	return &ListByRequestHandler{repo: r}
}

// Handle executes the list.
func (h *ListByRequestHandler) Handle(ctx context.Context, requestID int64) ([]*domain.Attachment, error) {
	return h.repo.ListByRequest(ctx, requestID)
}

// ListByCommentHandler returns comment-level attachments.
type ListByCommentHandler struct{ repo domain.Repository }

// NewListByCommentHandler constructs.
func NewListByCommentHandler(r domain.Repository) *ListByCommentHandler {
	return &ListByCommentHandler{repo: r}
}

// Handle executes the list.
func (h *ListByCommentHandler) Handle(ctx context.Context, commentID int64) ([]*domain.Attachment, error) {
	return h.repo.ListByComment(ctx, commentID)
}

// DownloadURLHandler returns a presigned download URL.
type DownloadURLHandler struct {
	repo    domain.Repository
	storage storage.Service
}

// NewDownloadURLHandler constructs.
func NewDownloadURLHandler(repo domain.Repository, svc storage.Service) *DownloadURLHandler {
	return &DownloadURLHandler{repo: repo, storage: svc}
}

// DownloadURLResult bundles the signed URL + validity.
type DownloadURLResult struct {
	URL          string
	ValidSeconds int32
}

// Handle issues a 5-minute presigned URL. Disposition picks `inline` for MIME
// types that browsers render natively (image/*, application/pdf, text/plain)
// so the URL opens a preview tab instead of force-downloading. Everything else
// (Office docs, archives) falls back to attachment download because browsers
// can't preview those natively.
func (h *DownloadURLHandler) Handle(ctx context.Context, attachmentID int64) (DownloadURLResult, error) {
	if h.storage == nil {
		return DownloadURLResult{}, ErrStorageRequired
	}
	a, err := h.repo.GetByID(ctx, attachmentID)
	if err != nil {
		return DownloadURLResult{}, err
	}
	const validity = 5 * time.Minute
	disposition := previewDisposition(a.MimeType)
	url, err := h.storage.PresignedGetURLWithDisposition(ctx, a.StorageKey, validity, a.Filename, disposition)
	if err != nil {
		return DownloadURLResult{}, fmt.Errorf("presigned url: %w", err)
	}
	return DownloadURLResult{URL: url, ValidSeconds: int32(validity / time.Second)}, nil
}

// previewDisposition returns "inline" for MIME types the browser previews
// natively, otherwise "attachment" so the file downloads.
func previewDisposition(mime string) string {
	switch mime {
	case "application/pdf", "text/plain", "text/csv":
		return "inline"
	default:
		if len(mime) >= 6 && mime[:6] == "image/" {
			return "inline"
		}
		return "attachment"
	}
}

// DeleteHandler removes the metadata + storage object.
type DeleteHandler struct {
	repo    domain.Repository
	storage storage.Service
}

// NewDeleteHandler constructs.
func NewDeleteHandler(repo domain.Repository, svc storage.Service) *DeleteHandler {
	return &DeleteHandler{repo: repo, storage: svc}
}

// Handle deletes the storage object first (best-effort), then the row.
func (h *DeleteHandler) Handle(ctx context.Context, attachmentID int64) error {
	a, err := h.repo.GetByID(ctx, attachmentID)
	if err != nil {
		return err
	}
	if h.storage != nil {
		if rmErr := h.storage.RemoveObject(ctx, a.StorageKey); rmErr != nil {
			return fmt.Errorf("remove storage object: %w", rmErr)
		}
	}
	return h.repo.Delete(ctx, attachmentID)
}

// =============================================================================
// helpers
// =============================================================================

// sanitizeFilename strips path separators and control chars from the user-supplied filename
// before it becomes part of the storage key. Caller still gets the original Filename back
// via the metadata row.
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		out = "file"
	}
	return out
}

// Ensure unused-import safety in case io changes.
var _ = io.Reader(nil)
