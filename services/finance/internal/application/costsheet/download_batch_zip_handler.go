package costsheet

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// ObjectFetcher fetches a stored object's raw bytes by key. Implemented by
// the storage package; injected here as an interface for tests.
type ObjectFetcher interface {
	GetObject(ctx context.Context, key string) (io.ReadCloser, int64, error)
}

// DownloadBatchZipQuery identifies the batch-tracking parent job whose
// completed child artifacts should be bundled into a single zip.
type DownloadBatchZipQuery struct {
	ParentJobID uuid.UUID
}

// DownloadBatchZipResult carries the assembled zip bytes + suggested filename.
type DownloadBatchZipResult struct {
	ZipData  []byte
	FileName string
}

// DownloadBatchZipHandler bundles every SUCCESS child artifact of a batch
// export job into a single in-memory zip archive, so the caller can offer
// one "download all" action instead of forcing a per-file download loop.
//
// Lifecycle: load parent → verify job type → verify it is actually a parent
// → list children → fetch each SUCCESS child's bytes from storage → write
// into a zip archive with a deduped, sys_id-free entry name → return the
// whole archive as bytes.
type DownloadBatchZipHandler struct {
	jobRepo job.Repository
	storage ObjectFetcher
}

// NewDownloadBatchZipHandler constructs the handler, reusing the same
// storage dependency style as the other costsheet export handlers.
func NewDownloadBatchZipHandler(jobRepo job.Repository, storage ObjectFetcher) *DownloadBatchZipHandler {
	return &DownloadBatchZipHandler{jobRepo: jobRepo, storage: storage}
}

// Handle builds and returns the zip archive of every completed child
// artifact belonging to the given batch parent job.
func (h *DownloadBatchZipHandler) Handle(ctx context.Context, q DownloadBatchZipQuery) (*DownloadBatchZipResult, error) {
	if h.storage == nil {
		return nil, fmt.Errorf("storage unavailable")
	}
	parent, err := h.jobRepo.GetByID(ctx, q.ParentJobID)
	if err != nil {
		return nil, fmt.Errorf("load parent job: %w", err)
	}
	if parent.JobType() != job.TypeProductCostSheetExport {
		return nil, fmt.Errorf("job %s is not a product_cost_sheet_export job", q.ParentJobID)
	}
	if !parent.IsParent() {
		return nil, job.ErrNotBatchParent
	}

	children, err := h.jobRepo.ListChildren(ctx, q.ParentJobID)
	if err != nil {
		return nil, fmt.Errorf("list child jobs: %w", err)
	}

	zipData, count, err := h.buildZip(ctx, children)
	if err != nil {
		return nil, err
	}

	return &DownloadBatchZipResult{
		ZipData:  zipData,
		FileName: fmt.Sprintf("cost-sheet-export-%s-%dfiles.zip", parent.Period(), count),
	}, nil
}

// buildZip streams every SUCCESS child's artifact bytes into an in-memory
// zip archive, skipping children that failed or whose artifact could not be
// fetched (logged, not fatal — matches ListBatchChildrenHandler's
// tolerate-per-child-failure behavior).
func (h *DownloadBatchZipHandler) buildZip(ctx context.Context, children []*job.Execution) ([]byte, int, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	taken := make(map[string]bool, len(children))
	written := 0
	for _, child := range children {
		if child.Status() != job.StatusSuccess {
			continue
		}
		if h.writeChildToZip(ctx, zw, child, taken) {
			written++
		}
	}

	if err := zw.Close(); err != nil {
		return nil, 0, fmt.Errorf("close zip writer: %w", err)
	}
	return buf.Bytes(), written, nil
}

// writeChildToZip fetches one child's artifact and writes it into the zip
// under a deduped, sys_id-free entry name. Returns false (without failing
// the batch) when the child's result_summary is invalid or the fetch fails.
func (h *DownloadBatchZipHandler) writeChildToZip(ctx context.Context, zw *zip.Writer, child *job.Execution, taken map[string]bool) bool {
	summary, err := parseResultSummary(child.ResultSummary())
	if err != nil {
		log.Warn().Err(err).Str("job_id", child.ID().String()).
			Msg("batch zip: skip child, invalid result_summary")
		return false
	}

	reader, _, err := h.storage.GetObject(ctx, summary.FilePath)
	if err != nil {
		log.Warn().Err(err).Str("job_id", child.ID().String()).
			Msg("batch zip: skip child, fetch failed")
		return false
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Str("job_id", child.ID().String()).Msg("batch zip: close object reader")
		}
	}()

	entryName := sanitizeZipEntryName(summary.FileName, taken)
	w, err := zw.Create(entryName)
	if err != nil {
		log.Warn().Err(err).Str("job_id", child.ID().String()).
			Msg("batch zip: skip child, create zip entry failed")
		return false
	}
	if _, err := io.Copy(w, reader); err != nil {
		log.Warn().Err(err).Str("job_id", child.ID().String()).
			Msg("batch zip: skip child, copy into zip failed")
		return false
	}
	return true
}

// defaultZipEntryName is used when a child's stored file name is empty.
const defaultZipEntryName = "export.xlsx"

// sanitizeZipEntryName turns an arbitrary stored file name into a safe,
// unique zip entry name: strips path separators (so no entry can escape its
// intended directory), falls back to a default when empty, and appends a
// numeric suffix when the name collides with one already written. Mirrors
// the dedup approach used by sanitizeSheetName in the worker's Excel export.
func sanitizeZipEntryName(name string, taken map[string]bool) string {
	cleaned := strings.NewReplacer("/", "_", "\\", "_").Replace(strings.TrimSpace(name))
	if cleaned == "" {
		cleaned = defaultZipEntryName
	}

	ext := ""
	base := cleaned
	if idx := strings.LastIndex(cleaned, "."); idx > 0 {
		base, ext = cleaned[:idx], cleaned[idx:]
	}

	candidate := cleaned
	for i := 2; taken[candidate]; i++ {
		candidate = base + " (" + strconv.Itoa(i) + ")" + ext
	}
	taken[candidate] = true
	return candidate
}
