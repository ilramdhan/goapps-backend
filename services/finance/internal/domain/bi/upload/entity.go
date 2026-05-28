// Package upload contains the BI Excel-upload aggregate (session header + staging rows)
// and the repository contract for staging-to-fact commit.
package upload

import (
	"time"

	"github.com/google/uuid"
)

// Session status values (mirror bi_excel_upload.status CHECK constraint).
const (
	StatusPending    = "PENDING"
	StatusValidated  = "VALIDATED"
	StatusCommitting = "COMMITTING"
	StatusCommitted  = "COMMITTED"
	StatusFailed     = "FAILED"
	StatusCancelled  = "CANCELLED"
)

// Per-row validation status values (mirror bi_excel_staging.validation_status CHECK constraint).
const (
	ValidationValid         = "VALID"
	ValidationInvalid       = "INVALID"
	ValidationWillOverwrite = "WILL_OVERWRITE"
)

// Upload is the aggregate root for one Excel upload session (bi_excel_upload row).
type Upload struct {
	uploadID      uuid.UUID
	sourceID      uuid.UUID
	targetType    string
	fileName      string
	fileSize      int
	status        string
	totalRows     int
	validRows     int
	invalidRows   int
	overwriteRows int
	committedRows int
	uploadedBy    uuid.UUID
	uploadedAt    time.Time
	committedAt   time.Time
	cancelledAt   time.Time
}

// NewUpload constructs a fresh upload session in the given status.
func NewUpload(sourceID uuid.UUID, targetType, fileName string, fileSize int, status string, uploadedBy uuid.UUID) *Upload {
	return &Upload{
		uploadID:   uuid.New(),
		sourceID:   sourceID,
		targetType: targetType,
		fileName:   fileName,
		fileSize:   fileSize,
		status:     status,
		uploadedBy: uploadedBy,
		uploadedAt: time.Now().UTC(),
	}
}

// Hydrate rebuilds an Upload from persisted fields (used by the repository).
func Hydrate(
	uploadID, sourceID uuid.UUID,
	targetType, fileName string,
	fileSize int,
	status string,
	totalRows, validRows, invalidRows, overwriteRows, committedRows int,
	uploadedBy uuid.UUID,
	uploadedAt, committedAt, cancelledAt time.Time,
) *Upload {
	return &Upload{
		uploadID:      uploadID,
		sourceID:      sourceID,
		targetType:    targetType,
		fileName:      fileName,
		fileSize:      fileSize,
		status:        status,
		totalRows:     totalRows,
		validRows:     validRows,
		invalidRows:   invalidRows,
		overwriteRows: overwriteRows,
		committedRows: committedRows,
		uploadedBy:    uploadedBy,
		uploadedAt:    uploadedAt,
		committedAt:   committedAt,
		cancelledAt:   cancelledAt,
	}
}

// SetCounts records the parse-time row tallies.
func (u *Upload) SetCounts(total, valid, invalid, overwrite int) {
	u.totalRows = total
	u.validRows = valid
	u.invalidRows = invalid
	u.overwriteRows = overwrite
}

// SetOverwriteRows updates the overwrite tally after fact-metric reconciliation.
func (u *Upload) SetOverwriteRows(overwrite int) { u.overwriteRows = overwrite }

// MarkCommitting transitions the session into the committing state.
func (u *Upload) MarkCommitting() { u.status = StatusCommitting }

// MarkCommitted records a successful commit with the number of rows written.
func (u *Upload) MarkCommitted(committed int) {
	u.status = StatusCommitted
	u.committedRows = committed
	u.committedAt = time.Now().UTC()
}

// MarkFailed transitions the session into the failed state.
func (u *Upload) MarkFailed() { u.status = StatusFailed }

// MarkCancelled records a cancellation.
func (u *Upload) MarkCancelled() {
	u.status = StatusCancelled
	u.cancelledAt = time.Now().UTC()
}

// ID returns the upload session identifier.
func (u *Upload) ID() uuid.UUID { return u.uploadID }

// SourceID returns the data-source identifier.
func (u *Upload) SourceID() uuid.UUID { return u.sourceID }

// TargetType returns the fact-metric type the file targets.
func (u *Upload) TargetType() string { return u.targetType }

// FileName returns the uploaded file name.
func (u *Upload) FileName() string { return u.fileName }

// FileSize returns the uploaded file size in bytes.
func (u *Upload) FileSize() int { return u.fileSize }

// Status returns the session status.
func (u *Upload) Status() string { return u.status }

// TotalRows returns the total data-row count.
func (u *Upload) TotalRows() int { return u.totalRows }

// ValidRows returns the count of rows that passed validation.
func (u *Upload) ValidRows() int { return u.validRows }

// InvalidRows returns the count of rows that failed validation.
func (u *Upload) InvalidRows() int { return u.invalidRows }

// OverwriteRows returns the count of rows whose business key already exists.
func (u *Upload) OverwriteRows() int { return u.overwriteRows }

// CommittedRows returns the count of rows written to the fact table.
func (u *Upload) CommittedRows() int { return u.committedRows }

// UploadedBy returns the uploader's user identifier.
func (u *Upload) UploadedBy() uuid.UUID { return u.uploadedBy }

// UploadedAt returns the upload timestamp.
func (u *Upload) UploadedAt() time.Time { return u.uploadedAt }

// CommittedAt returns the commit timestamp (zero when not committed).
func (u *Upload) CommittedAt() time.Time { return u.committedAt }

// CancelledAt returns the cancellation timestamp (zero when not cancelled).
func (u *Upload) CancelledAt() time.Time { return u.cancelledAt }

// StagingRow is one validated data row staged for commit (bi_excel_staging row).
type StagingRow struct {
	RowNumber        int
	Type             string
	Group1           string
	Group2           string
	Group3           string
	Group1Order      int
	Group2Order      int
	Group3Order      int
	PeriodGrain      string
	PeriodDate       time.Time
	PeriodLabel      string
	Value            float64
	DisplayValue     float64
	UOM              string
	Scenario         string
	MetricName       string // optional — defaults to 'VALUE' when absent
	MetricCategory   string // optional — defaults to 'VALUE' when absent
	AggMethod        string // optional — defaults to 'SUM' when absent
	ValidationStatus string
	ValidationMsg    string
}
