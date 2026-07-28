package worker

import (
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/application/etl"
)

// etlSourceSpgProduction is the metric source label for the SPG production ETL.
const etlSourceSpgProduction = "spg_production"

// NewSpgProductionETLWorker builds the incremental SPG production ETL worker. A
// non-positive interval defaults to 15 minutes. SPG DOFF_OPTION 1=Full is the
// inverse of the TXT TRN_STS convention; that logic lives in the usecase.
func NewSpgProductionETLWorker(usecase *etl.SpgProductionETL, interval time.Duration) *ProductionETLWorker {
	return NewProductionETLWorker(usecase, interval, etlSourceSpgProduction)
}
