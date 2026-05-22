package costcalc

// Shared validation error messages reused across handlers. Extracted to satisfy
// the goconst linter (threshold 3 repeated literals per package).
const (
	errMsgJobIDPositive     = "job_id must be > 0"
	errMsgProductIDPositive = "product_sys_id must be > 0"
	errMsgCostIDPositive    = "cost_id must be > 0"
	errMsgPeriodFormat      = "period must be YYYYMM"
	errMsgActorRequired     = "actor required"
)
