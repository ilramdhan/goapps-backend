package costcalc

// JobScope selects which products are included in a calc job.
type JobScope string

// JobScope constants.
const (
	ScopeAll           JobScope = "ALL"
	ScopeFiltered      JobScope = "FILTERED"
	ScopeSingleProduct JobScope = "SINGLE_PRODUCT"
	ScopeSingleRoute   JobScope = "SINGLE_ROUTE"
)
