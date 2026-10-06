package postgres

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// erp_batch_push_store.go adds the erpintegration.PushStore methods to the
// transaction-scoped erpBatchStore (plan-06 P5-T3). IsPeriodLocked and
// LoadAxComponents are shared with the DeriveStore; every read runs inside
// the G11-locked push transaction and is PG-only.

var _ erpintegration.PushStore = (*erpBatchStore)(nil)

// ListStdRows returns every std row of the batch ordered by key.
func (s *erpBatchStore) ListStdRows(ctx context.Context) ([]erpintegration.StdRow, error) {
	return listStdRows(ctx, s.tx, s.batchID)
}
