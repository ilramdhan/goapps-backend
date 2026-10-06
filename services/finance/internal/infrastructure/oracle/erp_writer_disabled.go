package oracle

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// DisabledWriter is the default OracleWriter: every method refuses with
// erpintegration.ErrWriterDisabled and never touches Oracle.
type DisabledWriter struct{}

var _ erpintegration.OracleWriter = DisabledWriter{}

// NewDisabledWriter returns the disabled writer.
func NewDisabledWriter() DisabledWriter { return DisabledWriter{} }

// InsertBatch implements erpintegration.OracleWriter.
func (DisabledWriter) InsertBatch(context.Context, erpintegration.PushBatch, []erpintegration.PushRow) (erpintegration.WriteResult, error) {
	return erpintegration.WriteResult{}, erpintegration.ErrWriterDisabled
}

// ValuateAdj implements erpintegration.OracleWriter.
func (DisabledWriter) ValuateAdj(context.Context, int64) (erpintegration.Summary, error) {
	return erpintegration.Summary{}, erpintegration.ErrWriterDisabled
}

// ApproveAdj implements erpintegration.OracleWriter.
func (DisabledWriter) ApproveAdj(context.Context, int64, string) (erpintegration.Summary, error) {
	return erpintegration.Summary{}, erpintegration.ErrWriterDisabled
}

// RestoreAdj implements erpintegration.OracleWriter.
func (DisabledWriter) RestoreAdj(context.Context, int64) (erpintegration.Summary, error) {
	return erpintegration.Summary{}, erpintegration.ErrWriterDisabled
}

// LockBatch implements erpintegration.OracleWriter.
func (DisabledWriter) LockBatch(context.Context, int64) (erpintegration.Summary, error) {
	return erpintegration.Summary{}, erpintegration.ErrWriterDisabled
}
