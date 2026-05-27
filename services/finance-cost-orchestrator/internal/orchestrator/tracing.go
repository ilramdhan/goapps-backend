package orchestrator

// tracerName is the instrumentation scope used for orchestrator spans.
const tracerName = "finance-cost-orchestrator"

// spanCostCalcJob is the root span name for a single calc job. The matching
// worker chunk span (cost_calc.chunk) is a child via RMQ-propagated context.
const spanCostCalcJob = "cost_calc.job"
