package worker

import (
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/application/etl"
)

// etlSourceTxtProduction is the metric source label for the TXT production ETL.
const etlSourceTxtProduction = "txt_production"

// NewTxtProductionETLWorker builds the incremental TXT/TWT production ETL worker.
// A non-positive interval defaults to 15 minutes.
func NewTxtProductionETLWorker(usecase *etl.TxtProductionETL, interval time.Duration) *ProductionETLWorker {
	return NewProductionETLWorker(usecase, interval, etlSourceTxtProduction)
}
