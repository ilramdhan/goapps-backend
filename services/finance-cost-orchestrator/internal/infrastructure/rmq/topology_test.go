package rmq

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeclareTopology_Idempotent(t *testing.T) {
	if os.Getenv("RMQ_INTEGRATION") != "true" {
		t.Skip("RMQ_INTEGRATION not set")
	}
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}
	conn, err := Connect(url)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	require.NoError(t, DeclareTopology(conn.Channel()))
	require.NoError(t, DeclareTopology(conn.Channel())) // second call must not error
}
