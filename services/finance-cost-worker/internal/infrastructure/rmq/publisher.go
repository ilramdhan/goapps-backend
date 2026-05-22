// Package rmq publisher placeholder. The worker will publish "chunk-done"
// events back to the orchestrator in S8c.7.
package rmq

// Publisher is a placeholder type. Methods will be added in S8c.7.
type Publisher struct {
	conn *Connection
}

// NewPublisher constructs a Publisher bound to the given connection.
func NewPublisher(conn *Connection) *Publisher {
	return &Publisher{conn: conn}
}
