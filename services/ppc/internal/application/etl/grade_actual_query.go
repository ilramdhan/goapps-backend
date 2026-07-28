package etl

import (
	"context"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// GradeActualReader is the read port for packing grade-actuals. Implemented by
// postgres.ETLRepository.
type GradeActualReader interface {
	ListGradeActuals(ctx context.Context, filter postgres.GradeActualFilter) ([]postgres.GradeActualRow, int64, error)
}

// GradeActualView is the application projection of one wo_grade_actual row,
// decoupling the delivery layer from the persistence row type.
type GradeActualView struct {
	ID              int64
	WOID            int64
	LotNo           string
	Grade           string
	Dept            string
	TotalQtyKg      float64
	BobbinCount     int32
	LastPackingDate *time.Time
	SyncedAt        time.Time
}

// GradeActualQuery selects and paginates grade-actual rows for listing.
type GradeActualQuery struct {
	Page      int32
	PageSize  int32
	WOID      *int64
	Grade     string
	Dept      string
	SortBy    string
	SortOrder string
}

// GradeActualResult is a paginated grade-actual list.
type GradeActualResult struct {
	Items       []GradeActualView
	CurrentPage int32
	PageSize    int32
	TotalItems  int64
	TotalPages  int32
}

// GradeActualService serves the grade-actual read use case (packing done view).
type GradeActualService struct {
	reader GradeActualReader
}

// NewGradeActualService builds the grade-actual query service.
func NewGradeActualService(reader GradeActualReader) *GradeActualService {
	return &GradeActualService{reader: reader}
}

// List returns grade-actual rows matching the query, with pagination metadata.
func (s *GradeActualService) List(ctx context.Context, q GradeActualQuery) (GradeActualResult, error) {
	page, pageSize := q.Page, q.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	rows, total, err := s.reader.ListGradeActuals(ctx, postgres.GradeActualFilter{
		Page:      page,
		PageSize:  pageSize,
		WOID:      q.WOID,
		Grade:     q.Grade,
		Dept:      q.Dept,
		SortBy:    q.SortBy,
		SortOrder: q.SortOrder,
	})
	if err != nil {
		return GradeActualResult{}, err
	}

	items := make([]GradeActualView, len(rows))
	for i := range rows {
		items[i] = GradeActualView{
			ID:              rows[i].ID,
			WOID:            rows[i].WOID,
			LotNo:           rows[i].LotNo,
			Grade:           rows[i].Grade,
			Dept:            rows[i].Dept,
			TotalQtyKg:      rows[i].TotalQtyKg,
			BobbinCount:     rows[i].BobbinCount,
			LastPackingDate: rows[i].LastPackingDate,
			SyncedAt:        rows[i].SyncedAt,
		}
	}
	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return GradeActualResult{
		Items:       items,
		CurrentPage: page,
		PageSize:    pageSize,
		TotalItems:  total,
		TotalPages:  totalPages,
	}, nil
}
