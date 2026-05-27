package rmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Exchange + queue names. Public so other packages (e.g. orchestrator) can
// bind their publish/consume calls to the same identifiers.
const (
	ExchangeCost    = "finance.cost"
	ExchangeCostDLX = "finance.cost.dlx"

	QueueChunk        = "finance.cost.chunk"
	QueueChunkDone    = "finance.cost.chunk.completed"
	QueueJobTriggered = "finance.cost.job_triggered"
	QueueDLQ          = "finance.cost.dlq"

	// Routing keys (same as queue names for direct exchanges -- keeps wiring simple).
	RoutingKeyChunk        = "finance.cost.chunk"
	RoutingKeyChunkDone    = "finance.cost.chunk.completed"
	RoutingKeyJobTriggered = "finance.cost.job_triggered"
	RoutingKeyDLQ          = "finance.cost.dlq"

	chunkTTLMillis     = 60 * 60 * 1000 // 1h
	chunkDoneTTLMillis = 30 * 60 * 1000 // 30min
	maxQueueLength     = 100_000
)

// DeclareTopology is idempotent: declares the two exchanges + three queues +
// their bindings + DLX routing on the given channel. Safe to call on every
// startup.
func DeclareTopology(ch *amqp.Channel) error {
	if err := declareExchanges(ch); err != nil {
		return err
	}
	if err := declareDLQ(ch); err != nil {
		return err
	}
	if err := declareChunkQueue(ch); err != nil {
		return err
	}
	if err := declareChunkDoneQueue(ch); err != nil {
		return err
	}
	return declareJobTriggeredQueue(ch)
}

func declareJobTriggeredQueue(ch *amqp.Channel) error {
	if _, err := ch.QueueDeclare(QueueJobTriggered, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %s: %w", QueueJobTriggered, err)
	}
	if err := ch.QueueBind(QueueJobTriggered, RoutingKeyJobTriggered, ExchangeCost, false, nil); err != nil {
		return fmt.Errorf("bind queue %s: %w", QueueJobTriggered, err)
	}
	return nil
}

func declareExchanges(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(ExchangeCost, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %s: %w", ExchangeCost, err)
	}
	if err := ch.ExchangeDeclare(ExchangeCostDLX, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %s: %w", ExchangeCostDLX, err)
	}
	return nil
}

func declareDLQ(ch *amqp.Channel) error {
	if _, err := ch.QueueDeclare(QueueDLQ, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %s: %w", QueueDLQ, err)
	}
	if err := ch.QueueBind(QueueDLQ, RoutingKeyDLQ, ExchangeCostDLX, false, nil); err != nil {
		return fmt.Errorf("bind queue %s: %w", QueueDLQ, err)
	}
	return nil
}

func declareChunkQueue(ch *amqp.Channel) error {
	args := amqp.Table{
		"x-message-ttl":             int32(chunkTTLMillis),
		"x-max-length":              int32(maxQueueLength),
		"x-dead-letter-exchange":    ExchangeCostDLX,
		"x-dead-letter-routing-key": RoutingKeyDLQ,
	}
	if _, err := ch.QueueDeclare(QueueChunk, true, false, false, false, args); err != nil {
		return fmt.Errorf("declare queue %s: %w", QueueChunk, err)
	}
	if err := ch.QueueBind(QueueChunk, RoutingKeyChunk, ExchangeCost, false, nil); err != nil {
		return fmt.Errorf("bind queue %s: %w", QueueChunk, err)
	}
	return nil
}

func declareChunkDoneQueue(ch *amqp.Channel) error {
	args := amqp.Table{
		"x-message-ttl": int32(chunkDoneTTLMillis),
		"x-max-length":  int32(maxQueueLength),
	}
	if _, err := ch.QueueDeclare(QueueChunkDone, true, false, false, false, args); err != nil {
		return fmt.Errorf("declare queue %s: %w", QueueChunkDone, err)
	}
	if err := ch.QueueBind(QueueChunkDone, RoutingKeyChunkDone, ExchangeCost, false, nil); err != nil {
		return fmt.Errorf("bind queue %s: %w", QueueChunkDone, err)
	}
	return nil
}
