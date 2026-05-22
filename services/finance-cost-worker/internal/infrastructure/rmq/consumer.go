// Package rmq consumer placeholder. The worker will consume chunk messages
// from the orchestrator-published queue in S8c.7.
package rmq

// Consumer is a placeholder type. Methods will be added in S8c.7.
type Consumer struct {
	conn *Connection
}

// NewConsumer constructs a Consumer bound to the given connection.
func NewConsumer(conn *Connection) *Consumer {
	return &Consumer{conn: conn}
}
