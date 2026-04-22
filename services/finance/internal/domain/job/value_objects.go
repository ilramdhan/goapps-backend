package job

import (
	"fmt"
	"strings"
)

// Status represents the current state of a job execution.
type Status string

// Status constants.
const (
	StatusQueued     Status = "QUEUED"
	StatusProcessing Status = "PROCESSING"
	StatusSuccess    Status = "SUCCESS"
	StatusFailed     Status = "FAILED"
	StatusCancelled  Status = "CANCELLED" //nolint:misspell // matches proto enum and DB CHECK constraint
)

// IsTerminal returns true if the status is a final state.
func (s Status) IsTerminal() bool {
	return s == StatusSuccess || s == StatusFailed || s == StatusCancelled
}

// IsActive returns true if the job is still running or waiting.
func (s Status) IsActive() bool {
	return s == StatusQueued || s == StatusProcessing
}

// String returns the string representation.
func (s Status) String() string {
	return string(s)
}

// LogStatus represents the status of a job execution log step.
type LogStatus string

// LogStatus constants.
const (
	LogStarted LogStatus = "STARTED"
	LogSuccess LogStatus = "SUCCESS"
	LogFailed  LogStatus = "FAILED"
	LogSkipped LogStatus = "SKIPPED"
)

// String returns the string representation.
func (s LogStatus) String() string {
	return string(s)
}

// Type represents the kind of job.
type Type string

// Type constants.
const (
	TypeOracleSync        Type = "oracle_sync"
	TypeRMCostCalculation Type = "rm_cost_calculation"
	TypeCalculation       Type = "calculation"
	TypeExport            Type = "export"
)

// String returns the string representation.
func (t Type) String() string {
	return string(t)
}

// Code represents a unique, human-readable job identifier.
type Code struct{ value string }

// NewCode creates a Code from a raw string.
func NewCode(s string) (Code, error) {
	if s == "" {
		return Code{}, fmt.Errorf("job code cannot be empty")
	}
	if len(s) > 30 {
		return Code{}, fmt.Errorf("job code must be at most 30 characters")
	}
	return Code{value: s}, nil
}

// GenerateCode creates a job code in the format TYPE-PERIOD-SEQ (e.g., SYNC-202601-001).
func GenerateCode(jobType Type, period string, seq int) Code {
	prefix := strings.ToUpper(string(jobType))
	if len(prefix) > 10 {
		prefix = prefix[:10]
	}
	if period == "" {
		period = "NOPERIOD"
	}
	return Code{value: fmt.Sprintf("%s-%s-%03d", prefix, period, seq)}
}

// String returns the code value.
func (c Code) String() string {
	return c.value
}
