package worker

const (
	// tracerName is the instrumentation scope used for worker spans.
	tracerName = "finance-cost-worker"
	// spanCostCalcChunk is the per-chunk span name. It is a child of the
	// orchestrator's cost_calc.job span via RMQ-propagated trace context.
	spanCostCalcChunk = "cost_calc.chunk"
)
