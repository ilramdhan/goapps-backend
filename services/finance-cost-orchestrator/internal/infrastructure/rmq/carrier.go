package rmq

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

// amqpHeaderCarrier adapts an amqp.Table to the OpenTelemetry
// propagation.TextMapCarrier interface so trace context can be injected into
// (and extracted from) AMQP message headers. Values are stored as strings
// because the W3C TraceContext propagator only ever reads/writes string values.
type amqpHeaderCarrier amqp.Table

// Get returns the string value for key, or "" if absent / not a string.
func (c amqpHeaderCarrier) Get(key string) string {
	v, ok := c[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// Set stores value under key.
func (c amqpHeaderCarrier) Set(key, value string) {
	c[key] = value
}

// Keys returns all header keys carried by the table.
func (c amqpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
