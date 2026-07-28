package worker

import (
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/application/etl"
)

// etlSourceGradeActual is the metric source label for the packing grade-actual ETL.
const etlSourceGradeActual = "grade_actual"

// NewGradeActualETLWorker builds the incremental packing grade-actual ETL worker.
// A non-positive interval defaults to 15 minutes.
func NewGradeActualETLWorker(usecase *etl.GradeActualETL, interval time.Duration) *ProductionETLWorker {
	return NewProductionETLWorker(usecase, interval, etlSourceGradeActual)
}
