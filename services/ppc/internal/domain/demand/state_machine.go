package demand

// allowedTransitions encodes the demand status lifecycle (PRD §3). For each
// from-state, the set of legal to-states. Terminal states (FULFILLED, CANCELLED,
// CARRIED_OVER, SPLIT) have no onward transitions.
var allowedTransitions = map[string]map[string]struct{}{
	// An unlinked demand is not plannable: linking is its only way forward. It
	// deliberately cannot be confirmed, carried forward, split or cancelled —
	// every one of those would commit production against an unknown product.
	StatusPendingProductLink: {
		StatusPendingConfirmation: {},
	},
	StatusPendingConfirmation: {
		StatusConfirmed: {},
		StatusCancelled: {},
	},
	StatusConfirmed: {
		StatusInProduction: {},
		StatusCancelled:    {},
		StatusCarriedOver:  {},
		StatusDeferred:     {},
		StatusSplit:        {},
	},
	StatusInProduction: {
		StatusPartial:     {},
		StatusFulfilled:   {},
		StatusCarriedOver: {},
		StatusDeferred:    {},
		StatusSplit:       {},
		StatusCancelled:   {},
	},
	StatusPartial: {
		StatusFulfilled:   {},
		StatusCarriedOver: {},
		StatusDeferred:    {},
		StatusSplit:       {},
		StatusCancelled:   {},
	},
	StatusFulfilled:   {},
	StatusCancelled:   {},
	StatusCarriedOver: {},
	StatusDeferred: {
		StatusCarriedOver: {},
		StatusCancelled:   {},
		StatusSplit:       {},
	},
	StatusSplit: {},
}

// canTransition reports whether a demand may move from -> to.
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
