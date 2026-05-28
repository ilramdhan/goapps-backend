// Package job contains the BI ETL job registry and per-run log.
//
// Real Oracle workers live in spec 1D; this package defines the data types and
// repository contract so admin pages can list jobs and trigger manual runs in MVP.
package job

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors.
var (
	// ErrNotFound is returned when no job matches the lookup.
	ErrNotFound = errors.New("bi job not found")
	// ErrAlreadyExists is returned when a job with the same name already exists.
	ErrAlreadyExists = errors.New("bi job name already exists")
	// ErrInvalidCron is returned when a cron expression is malformed.
	ErrInvalidCron = errors.New("invalid cron expression")
)

// Job is a registered ETL job.
type Job struct {
	ID              uuid.UUID
	Name            string
	SourceID        uuid.UUID
	SourceCode      string // resolved on read for UI convenience
	TargetType      string // bi_fact_metric.type this job feeds
	ScheduleCron    string // 5-field cron or empty for manual-only
	OracleProcedure string
	Config          map[string]any
	IsActive        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time

	// Resolved on List for UI; not stored on Job row.
	LastStatus     string
	LastRunAt      time.Time
	LastDurationMs int
}

// Status enum.
const (
	StatusRunning = "RUNNING"
	StatusSuccess = "SUCCESS"
	StatusFailed  = "FAILED"
	// StatusCancelled is the persisted DB/proto value for a canceled job run.
	StatusCancelled = "CANCELLED" //nolint:misspell // CANCELLED is a persisted DB/proto enum value
)

// Log is one execution log entry for a Job.
type Log struct {
	LogID        int64
	JobID        uuid.UUID
	JobName      string // resolved on read
	StartedAt    time.Time
	EndedAt      time.Time
	Status       string
	RowsAffected int
	ErrorMessage string
	TriggeredBy  string // 'CRON' | 'MANUAL:<user_id>' | 'EVENT:<source>'
	DurationMs   int
}

// CreateJobParams holds the fields required to persist a new Job.
// SourceCode is resolved to source_id by the repository using a sub-select on bi_data_source.
type CreateJobParams struct {
	JobName         string
	SourceCode      string
	TargetType      string
	ScheduleCron    string
	OracleProcedure string
	Config          map[string]any
	IsActive        bool
	CreatedBy       uuid.UUID
}

// UpdateJobParams holds the mutable fields for UpdateJob (all optional).
type UpdateJobParams struct {
	ID              uuid.UUID
	ScheduleCron    *string
	OracleProcedure *string
	Config          map[string]any
	IsActive        *bool
	UpdatedBy       uuid.UUID
}

// Repository is the read + trigger + CRUD contract.
type Repository interface {
	List(ctx context.Context, includeInactive bool) ([]*Job, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Job, error)
	Create(ctx context.Context, p CreateJobParams) (*Job, error)
	Update(ctx context.Context, p UpdateJobParams) (*Job, error)
	Delete(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error

	ListLogs(ctx context.Context, jobID uuid.UUID, page, pageSize int) ([]*Log, int64, error)
	InsertLog(ctx context.Context, log *Log) error
	UpdateLog(ctx context.Context, log *Log) error
}
