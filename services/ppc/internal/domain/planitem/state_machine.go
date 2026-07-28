package planitem

// allowedTransitions encodes the plan-item lifecycle (PRD §4):
// DRAFT → CONFIRMED → IN_PROGRESS → COMPLETED → CLOSED. A plan item may be
// closed from any active state (cancellation).
var allowedTransitions = map[string]map[string]struct{}{
	StatusDraft: {
		StatusConfirmed: {},
		StatusClosed:    {},
	},
	StatusConfirmed: {
		StatusInProgress: {},
		StatusClosed:     {},
	},
	StatusInProgress: {
		StatusCompleted: {},
		StatusClosed:    {},
	},
	StatusCompleted: {
		StatusClosed: {},
	},
	StatusClosed: {},
}

// canTransition reports whether a plan item may move from -> to.
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
