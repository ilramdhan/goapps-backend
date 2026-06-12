package costimportjob

import "context"

// Repository persists CostImportJob aggregates.
type Repository interface {
	Create(ctx context.Context, job *CostImportJob) error
	GetByID(ctx context.Context, id int64) (*CostImportJob, error)
	Update(ctx context.Context, job *CostImportJob) error
	List(ctx context.Context, entity, status string, page, pageSize int) ([]*CostImportJob, int64, error)
}
