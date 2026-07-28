package demand

import (
	"context"
	"time"
)

// Repository defines persistence operations for demands and their SO-staging inbox.
type Repository interface {
	// Create persists a new demand and assigns its ID.
	Create(ctx context.Context, entity *Demand) error
	// GetByID retrieves a demand by its ID.
	GetByID(ctx context.Context, id int64) (*Demand, error)
	// List retrieves demands with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Demand, int64, error)
	// Update persists changes to an existing demand.
	Update(ctx context.Context, entity *Demand) error
	// Delete removes a demand by its ID.
	Delete(ctx context.Context, id int64) error

	// CountPlanItems returns how many plan items reference the demand.
	CountPlanItems(ctx context.Context, demandID int64) (int64, error)
	// ListCarryCandidates returns demands eligible for carry-forward in a month.
	ListCarryCandidates(ctx context.Context, sourceMonth string) ([]*Demand, error)

	// GetStagingByIDs retrieves SO-staging rows by their ids.
	GetStagingByIDs(ctx context.Context, ids []int64) ([]*SalesOrderStaging, error)
	// ListStaging retrieves SO-staging rows with filtering and pagination.
	ListStaging(ctx context.Context, filter StagingListFilter) ([]*SalesOrderStaging, int64, error)
	// ListStagingIDs retrieves the ids of every SO-staging row matching a
	// filter, up to limit, plus the untruncated match count. It exists so a
	// "select all matching" never has to page through full rows just to learn
	// their ids.
	ListStagingIDs(ctx context.Context, filter StagingIDsFilter, limit int) ([]int64, int64, error)
	// LookupStagingItemCodes returns the Orion item code of each given staging
	// row id, keyed by sos id. Rows with no item code are omitted. It exists so
	// an unlinked demand can be named by the Orion row it came from instead of
	// rendering as a placeholder.
	LookupStagingItemCodes(ctx context.Context, sosIDs []int64) (map[int64]string, error)
	// MarkStagingPulled sets sos_pulled_to_demand_id for a staging row.
	MarkStagingPulled(ctx context.Context, sosID, demandID int64) error
	// SetStagingProduct records a planner's manual product pick on a staging row
	// that has not been pulled yet, marking it MANUAL so automatic resolution
	// never overwrites it. Returns the updated row.
	SetStagingProduct(ctx context.Context, sosID, cpmProductSysID int64) (*SalesOrderStaging, error)

	// ListUnresolvedStagingPairs returns the distinct normalized (item code,
	// shade code) pairs of staging rows that are still UNRESOLVED and not yet
	// pulled into a demand.
	ListUnresolvedStagingPairs(ctx context.Context) ([]StagingPair, error)
	// ApplyStagingResolutions writes resolution outcomes back onto every staging
	// row sharing each normalized pair, leaving MANUAL rows untouched, and
	// returns the number of rows updated.
	ApplyStagingResolutions(ctx context.Context, resolutions []ProductResolution) (int64, error)
}

// Match statuses recorded on sales_order_staging.sos_match_status. They mirror
// the chk_sos_match_status CHECK constraint from migration 000030.
const (
	// MatchStatusUnresolved means resolution has not been attempted yet.
	MatchStatusUnresolved = "UNRESOLVED"
	// MatchStatusAuto means exactly one finance product matched.
	MatchStatusAuto = "AUTO"
	// MatchStatusAmbiguous means two or more finance products matched.
	MatchStatusAmbiguous = "AMBIGUOUS"
	// MatchStatusNotFound means no finance product matched.
	MatchStatusNotFound = "NOT_FOUND"
	// MatchStatusManual means a planner picked the product by hand. Automatic
	// resolution never overwrites it.
	MatchStatusManual = "MANUAL"
)

// StagingPair is a normalized (ERP item code, shade code) resolution key. Both
// components are trimmed and upper-cased; ShadeCode may be empty.
type StagingPair struct {
	ItemCode  string
	ShadeCode string
}

// ProductResolution is the outcome of resolving one StagingPair against the
// finance cost product master. CpmProductSysID is set only when MatchCount is 1.
type ProductResolution struct {
	Pair            StagingPair
	MatchCount      int32
	CpmProductSysID *int64
}

// MatchStatus derives the persisted status from the match count: a unique match
// is AUTO, several are AMBIGUOUS, none is NOT_FOUND.
func (r ProductResolution) MatchStatus() string {
	switch {
	case r.MatchCount == 1 && r.CpmProductSysID != nil:
		return MatchStatusAuto
	case r.MatchCount >= 2:
		return MatchStatusAmbiguous
	default:
		return MatchStatusNotFound
	}
}

// ProductResolver resolves ERP item/shade code pairs to finance cost product
// master sys ids. ppc_db and finance_db are separate databases, so this is a
// gRPC call, not a join. Implemented by the finance client adapter.
type ProductResolver interface {
	ResolveByErpCode(ctx context.Context, pairs []StagingPair) ([]ProductResolution, error)
}

// ListFilter contains filtering and pagination for listing demands.
type ListFilter struct {
	Search          string
	Type            string
	Status          string
	Month           string
	CpmProductSysID *int64
	// WithoutPlan hides demands that already have at least one plan item. It is
	// opt-in from the planning context only: the demand list proper must keep
	// showing planned demands.
	WithoutPlan bool
	Page        int
	PageSize    int
	SortBy      string
	SortOrder   string
}

// Validate normalizes pagination and sort defaults.
func (f *ListFilter) Validate() {
	normalizePaging(&f.Page, &f.PageSize)
	if f.SortBy == "" {
		f.SortBy = "created_at"
	}
	if f.SortOrder == "" {
		f.SortOrder = "desc"
	}
}

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int { return (f.Page - 1) * f.PageSize }

// StagingListFilter contains filtering and pagination for SO-staging rows.
type StagingListFilter struct {
	Search       string
	CustomerCode string
	ItemCode     string
	UnpulledOnly bool
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

// MaxSelectAllStagingIDs bounds a "select all matching" response. It is
// deliberately equal to the PullFromOrion batch limit (max_items on
// PullFromOrionRequest.sos_ids): selecting more rows than one pull can carry
// would just move the lie from the count to the action.
const MaxSelectAllStagingIDs = 200

// StagingIDsFilter is the row-selection half of StagingListFilter — everything
// that decides *which* staging rows match, with nothing about paging or sort.
// ListStaging and ListStagingIDs share it so the count a planner sees and the
// ids they select can never resolve to different sets.
type StagingIDsFilter struct {
	Search       string
	CustomerCode string
	ItemCode     string
	UnpulledOnly bool
}

// Predicate returns the row-selection half of the filter.
func (f *StagingListFilter) Predicate() StagingIDsFilter {
	return StagingIDsFilter{
		Search:       f.Search,
		CustomerCode: f.CustomerCode,
		ItemCode:     f.ItemCode,
		UnpulledOnly: f.UnpulledOnly,
	}
}

// Validate normalizes pagination and sort defaults.
func (f *StagingListFilter) Validate() {
	normalizePaging(&f.Page, &f.PageSize)
	if f.SortBy == "" {
		f.SortBy = "deadline"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL offset for pagination.
func (f *StagingListFilter) Offset() int { return (f.Page - 1) * f.PageSize }

func normalizePaging(page, pageSize *int) {
	if *page < 1 {
		*page = 1
	}
	if *pageSize < 1 {
		*pageSize = 10
	}
	if *pageSize > 100 {
		*pageSize = 100
	}
}

// SalesOrderStaging is the ETL inbox projection consumed by PullFromOrion. It is
// a read model (full DDL in migration 000010), not an aggregate.
type SalesOrderStaging struct {
	SosID            int64
	ContractNo       string
	ContractDate     *time.Time
	ContractSysID    *int64
	CustomerCode     string
	CustomerName     string
	ItemCode         string
	ItemDesc         string
	GradeCode        string
	ShadeCode        string
	ShadeName        string
	QtyOrdered       float64
	QtyDelivered     float64
	QtyRemaining     float64
	Deadline         *time.Time
	ShipDate         string
	MergeNo          string
	Term             string
	Rate             float64
	Currency         string
	BlockedStatus    string
	OutstandingAr    float64
	PalletType       string
	EndUse           string
	MixFlag          string
	Annotation       string
	Remarks          string
	EtlSyncedAt      *time.Time
	PulledToDemandID *int64

	// CpmProductSysID is the finance product resolved from (ItemCode, ShadeCode),
	// or nil while the row is UNRESOLVED / AMBIGUOUS / NOT_FOUND.
	CpmProductSysID *int64
	// MatchStatus is one of the MatchStatus* constants.
	MatchStatus string
	// MatchCount is how many finance products matched at resolution time.
	MatchCount int32
	// MatchedAt is when the resolution was last written, nil while unresolved.
	MatchedAt *time.Time
}
