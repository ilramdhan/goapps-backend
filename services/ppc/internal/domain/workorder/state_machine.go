package workorder

// allowedTransitions encodes the WO v1.2 lifecycle (PRD §5):
// DRAFT → SUBMITTED → PC_APPROVED → APPROVED → SCHEDULED → CHANGEOVER → RUNNING →
// COMPLETED → CLOSED, with SUBMITTED/PC_APPROVED → REJECTED → DRAFT (rework) and
// CANCELLED (manual + reason) reachable from any non-terminal running-or-earlier
// state.
var allowedTransitions = map[string]map[string]struct{}{
	StatusDraft: {
		StatusSubmitted: {},
		StatusCancelled: {},
	},
	StatusSubmitted: {
		StatusPCApproved: {},
		StatusRejected:   {},
		StatusCancelled:  {},
	},
	StatusPCApproved: {
		StatusApproved:  {},
		StatusRejected:  {},
		StatusCancelled: {},
	},
	StatusRejected: {
		StatusDraft:     {},
		StatusSubmitted: {},
		StatusCancelled: {},
	},
	StatusApproved: {
		StatusScheduled: {},
		StatusCancelled: {},
	},
	StatusScheduled: {
		StatusChangeover: {},
		StatusRunning:    {},
		StatusCancelled:  {},
	},
	StatusChangeover: {
		StatusRunning:   {},
		StatusCancelled: {},
	},
	StatusRunning: {
		StatusCompleted: {},
		StatusCancelled: {},
	},
	StatusCompleted: {
		StatusClosed: {},
	},
	StatusClosed:    {},
	StatusCancelled: {},
}

// canTransition reports whether a WO may move from -> to.
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
