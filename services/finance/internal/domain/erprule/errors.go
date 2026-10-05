package erprule

import "errors"

// Sentinel errors for the ERP rule master (design Part 1 §4.1, §5.2; plan-03
// P2-T1). Delivery (P6) maps the "invalid" family to gRPC InvalidArgument,
// the not-found family to NotFound, ErrDuplicateRule to AlreadyExists and
// ErrRuleInUse / ErrRuleInactive to FailedPrecondition.
var (
	// ErrRuleMissing (V-08) is returned by RuleSet lookups when no active
	// rule exists for (fg type, prod type, grade group).
	ErrRuleMissing = errors.New("erprule: no active valloss rule for the key")

	// ErrRuleAmbiguous is returned by RuleSet lookups when more than one
	// active rule exists for the same key. The unique index uk_cevr_key
	// prevents it in PostgreSQL; the domain still refuses to guess.
	ErrRuleAmbiguous = errors.New("erprule: more than one active valloss rule for the key")

	// ErrSellPriceMissing (V-08, NO_SELL_PRICE) is returned when a
	// selling-price basis has no active cst_erp_sell_price row.
	ErrSellPriceMissing = errors.New("erprule: no active sell price for the basis")

	// ErrDuplicateRule is returned when an active rule already exists for the
	// key (uk_cevr_key), or when a RuleSet input repeats a sell price or a
	// grade code.
	ErrDuplicateRule = errors.New("erprule: an active rule already exists for the key")

	// ErrRuleInUse is returned when deleting a rule while a non-terminal ERP
	// batch (DERIVED..VALUATED) still uses the current rule hash.
	ErrRuleInUse = errors.New("erprule: rule set is in use by a non-terminal ERP batch")

	// ErrRuleNotFound is returned by repositories for an unknown rule id.
	ErrRuleNotFound = errors.New("erprule: valloss rule not found")

	// ErrRuleInactive is returned when changing a soft-deleted rule.
	ErrRuleInactive = errors.New("erprule: valloss rule is inactive")

	// ErrSellPriceNotFound is returned by repositories for an unknown basis.
	ErrSellPriceNotFound = errors.New("erprule: sell price not found")

	// ErrGradeNotFound is returned by repositories for an unknown ERP grade.
	ErrGradeNotFound = errors.New("erprule: ERP grade not found")

	// ErrInvalidPercent is returned for a value loss outside [0, 999.999999]
	// or with more than 6 decimal places (chk_cevr_val_loss, NUMERIC(20,6),
	// proto pattern ^[0-9]{1,3}(\.[0-9]{1,6})?$).
	ErrInvalidPercent = errors.New("erprule: value loss must be between 0 and 999.999999 with at most 6 decimal places")

	// ErrInvalidPrice is returned for a sell price that is not > 0, is too
	// large for NUMERIC(20,6), or has more than 6 decimal places.
	ErrInvalidPrice = errors.New("erprule: sell price must be > 0 with at most 6 decimal places")

	// ErrInvalidBasis is returned for a basis outside COST/SPPTY/SPITY/SPBSD,
	// or for COST where a selling-price basis is required.
	ErrInvalidBasis = errors.New("erprule: invalid basis")

	// ErrInvalidProdType is returned for a prod type outside POY/PTY/ITY.
	ErrInvalidProdType = errors.New("erprule: invalid prod type")

	// ErrInvalidGradeGroup is returned for an unknown grade group.
	ErrInvalidGradeGroup = errors.New("erprule: invalid grade group")

	// ErrAxRule is returned for a valloss rule on grade group AX
	// (chk_cevr_grade_group): AX never goes through a rule.
	ErrAxRule = errors.New("erprule: grade group AX never goes through a valloss rule")

	// ErrInvalidFgType is returned for an empty or too-long FG type.
	ErrInvalidFgType = errors.New("erprule: invalid FG type")

	// ErrInvalidGradeCode is returned for an empty or too-long ERP grade code.
	ErrInvalidGradeCode = errors.New("erprule: invalid ERP grade code")

	// ErrUserRequired is returned when the acting user is empty or too long.
	ErrUserRequired = errors.New("erprule: user is required (max 64 characters)")
)
