// Package rmq holds RabbitMQ wiring for the worker. The worker does NOT
// declare topology -- the orchestrator owns that. These constants must stay in
// lockstep with goapps-backend/services/finance-cost-orchestrator/internal/infrastructure/rmq.
package rmq

// Exchange + queue + routing-key identifiers shared with the orchestrator.
const (
	ExchangeCost        = "finance.cost"
	QueueChunk          = "finance.cost.chunk"
	QueueChunkDone      = "finance.cost.chunk.completed"
	RoutingKeyChunk     = "finance.cost.chunk"
	RoutingKeyChunkDone = "finance.cost.chunk.completed"
)
