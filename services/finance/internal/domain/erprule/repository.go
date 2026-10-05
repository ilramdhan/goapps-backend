package erprule

import "context"

// VallossRuleFilter narrows a valloss rule listing. Zero values mean "any".
type VallossRuleFilter struct {
	FgType          FgType
	ProdType        ProdType
	GradeGroup      GradeGroup
	Basis           Basis
	IncludeInactive bool
	Page            int
	PageSize        int
}

// GradeFilter narrows an ERP grade listing.
type GradeFilter struct {
	// UnassignedOnly returns grades with ceg_grade_group IS NULL (the V-08
	// worklist).
	UnassignedOnly bool
	Search         string
	Page           int
	PageSize       int
}

// VallossRuleRepository persists cst_erp_valloss_rule (P2-T3).
type VallossRuleRepository interface {
	// Create inserts an active rule and returns it with cevr_id set. A clash
	// with an active rule of the same key returns ErrDuplicateRule.
	Create(ctx context.Context, rule *VallossRule) (*VallossRule, error)
	// GetByID returns the rule, or ErrRuleNotFound.
	GetByID(ctx context.Context, id int64) (*VallossRule, error)
	// List returns one page of rules and the total count.
	List(ctx context.Context, filter VallossRuleFilter) ([]*VallossRule, int64, error)
	// Update persists basis, value loss, is_active and updated_at/by (soft
	// delete is an Update of a deactivated rule). ErrRuleNotFound when absent.
	Update(ctx context.Context, rule *VallossRule) error
}

// SellPriceRepository persists cst_erp_sell_price (P2-T3).
type SellPriceRepository interface {
	// Get returns the price for a basis, or ErrSellPriceNotFound.
	Get(ctx context.Context, basis Basis) (*SellPrice, error)
	// List returns every sell price ordered by basis.
	List(ctx context.Context, includeInactive bool) ([]*SellPrice, error)
	// Upsert inserts or updates the price row for its basis.
	Upsert(ctx context.Context, price *SellPrice) error
}

// GradeGroupRepository reads cost_erp_grade and writes ceg_grade_group only
// (P2-T3). No other column of the replica is ever written here.
type GradeGroupRepository interface {
	// GetGrade returns the grade, or ErrGradeNotFound.
	GetGrade(ctx context.Context, gradeCode string) (*Grade, error)
	// ListGrades returns one page of grades and the total count.
	ListGrades(ctx context.Context, filter GradeFilter) ([]*Grade, int64, error)
	// SetGradeGroup updates ceg_grade_group only (nil clears it), for a grade
	// changed by Grade.AssignGroup / ClearGroup. ErrGradeNotFound when absent.
	SetGradeGroup(ctx context.Context, grade *Grade) error
}

// RuleSetLoader reads every active rule, sell price and grade assignment in
// one REPEATABLE READ snapshot and builds a RuleSet (P2-T3).
type RuleSetLoader interface {
	LoadRuleSet(ctx context.Context) (*RuleSet, error)
}

// RuleHashUsage answers whether a non-terminal ERP batch (DERIVED..VALUATED)
// uses a rule hash. The application layer checks it before a delete and
// returns ErrRuleInUse (P2-T4); it is stubbed until the P3-T2 batch
// repository exists.
type RuleHashUsage interface {
	IsRuleHashInUse(ctx context.Context, ruleHash string) (bool, error)
}
