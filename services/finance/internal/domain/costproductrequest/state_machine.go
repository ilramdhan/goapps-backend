package costproductrequest

// Status values per PRD §6.2.
const (
	StatusDraft              = "DRAFT"
	StatusSubmitted          = "SUBMITTED"
	StatusUnderReview        = "UNDER_REVIEW"
	StatusRoutingDefined     = "ROUTING_DEFINED"
	StatusParameterPending   = "PARAMETER_PENDING"
	StatusParameterComplete  = "PARAMETER_COMPLETE"
	StatusCostingDone        = "COSTING_DONE"
	StatusQuoted             = "QUOTED"
	StatusQuoteReady         = "QUOTE_READY"
	StatusClosed             = "CLOSED"
	StatusRejected           = "REJECTED"
)

// ClosedSubstatus values.
const (
	ClosedWon       = "won"
	ClosedLost      = "lost"
	ClosedCancelled = "cancelled"
	ClosedOnHold    = "on_hold"
)

// Classification + feasibility constants.
const (
	ClassExisting = "existing"
	ClassNew      = "new"

	FeasibilityFeasible    = "FEASIBLE"
	FeasibilityNotFeasible = "NOT_FEASIBLE"

	UrgencyLow    = "low"
	UrgencyMedium = "medium"
	UrgencyHigh   = "high"
)

// allowedTransitions encodes the FR-2 state machine. For each from-state, the set of
// to-states reachable via *any* transition method. The actual method enforces the
// specific predicate (e.g., DecideFeasibility allows only Under_Review → Routing_Defined or Rejected).
var allowedTransitions = map[string]map[string]struct{}{
	StatusDraft: {
		StatusSubmitted: {},                  // Submit
		StatusClosed:    {},                  // Cancel
	},
	StatusSubmitted: {
		StatusUnderReview: {},
		StatusRejected:    {},
		StatusClosed:      {},
	},
	StatusUnderReview: {
		StatusRoutingDefined: {},
		StatusRejected:       {},
		StatusQuoteReady:     {},
		StatusClosed:         {},
	},
	StatusRoutingDefined: {
		StatusParameterPending: {},
		StatusClosed:           {},
	},
	StatusParameterPending: {
		StatusParameterComplete: {},
		StatusClosed:            {},
	},
	StatusParameterComplete: {
		StatusCostingDone: {},
		StatusClosed:      {},
	},
	StatusCostingDone: {
		StatusQuoted: {},
		StatusClosed: {},
	},
	StatusQuoteReady: {
		StatusQuoted: {},
		StatusClosed: {},
	},
	StatusQuoted: {
		StatusClosed: {},
	},
	StatusRejected: {
		StatusSubmitted: {}, // Revise
		StatusClosed:    {}, // Cancel
	},
	// StatusClosed is terminal — no outbound edges.
}

func canTransition(from, to string) bool {
	if from == to {
		return false
	}
	tos, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = tos[to]
	return ok
}
