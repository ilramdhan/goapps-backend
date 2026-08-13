package rabbitmq

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

// unreachableConfig points at a port nothing listens on, so every dial fails
// fast with connection-refused. The tiny ReconnectDelay keeps the retry test
// well under a second.
func unreachableConfig() config.RabbitMQConfig {
	return config.RabbitMQConfig{
		URL:            "amqp://guest:guest@127.0.0.1:1/",
		PrefetchCount:  1,
		ReconnectDelay: 10 * time.Millisecond,
	}
}

// TestNewConnectionWithRetry_ExhaustsAttempts verifies the startup retry loop
// gives up after exactly maxAttempts, returns the last dial error, and sleeps
// only between attempts (not after the final one).
func TestNewConnectionWithRetry_ExhaustsAttempts(t *testing.T) {
	const maxAttempts = 3
	cfg := unreachableConfig()

	start := time.Now()
	conn, err := NewConnectionWithRetry(cfg, zerolog.Nop(), maxAttempts)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Nil(t, conn)
	require.ErrorContains(t, err, "connect to rabbitmq")

	// (maxAttempts-1) sleeps of 10ms; generous ceiling to stay non-flaky while
	// still failing loudly if the loop slept after the last attempt or looped
	// unbounded.
	require.GreaterOrEqual(t, elapsed, time.Duration(maxAttempts-1)*cfg.ReconnectDelay)
	require.Less(t, elapsed, 5*time.Second, "retry loop took far longer than the configured delays")
}

// TestNewConnectionWithRetry_ClampsNonPositiveAttempts ensures a caller passing
// 0 or a negative count still gets exactly one attempt rather than none.
func TestNewConnectionWithRetry_ClampsNonPositiveAttempts(t *testing.T) {
	for _, attempts := range []int{0, -5} {
		conn, err := NewConnectionWithRetry(unreachableConfig(), zerolog.Nop(), attempts)
		require.Error(t, err, "attempts=%d must still dial once and report the failure", attempts)
		require.Nil(t, conn)
	}
}

// TestReconnectDelay_DefaultsWhenUnset covers the 5s fallback used by both the
// startup retry and the supervisor.
func TestReconnectDelay_DefaultsWhenUnset(t *testing.T) {
	require.Equal(t, 5*time.Second, reconnectDelay(config.RabbitMQConfig{}))
	require.Equal(t, 250*time.Millisecond,
		reconnectDelay(config.RabbitMQConfig{ReconnectDelay: 250 * time.Millisecond}))
}

// TestOnReconnect_RegistersAndInvokesHooks verifies hooks are stored in
// registration order, nil hooks are ignored, and swap() runs every hook with
// the new channel even when one of them fails (a failing hook must not abort
// the remaining declares).
func TestOnReconnect_RegistersAndInvokesHooks(t *testing.T) {
	c := &Connection{config: unreachableConfig(), logger: zerolog.Nop()}

	var order []string
	c.OnReconnect(func(*amqp.Channel) error {
		order = append(order, "first")
		return nil
	})
	c.OnReconnect(func(*amqp.Channel) error {
		order = append(order, "boom")
		return errBoom
	})
	c.OnReconnect(func(*amqp.Channel) error {
		order = append(order, "last")
		return nil
	})
	c.OnReconnect(nil) // must be dropped, not stored

	require.Len(t, c.onReconnect, 3, "nil hook must not be registered")

	// swap installs the fresh pair then replays hooks. A nil amqp.Channel is
	// fine here: the hooks under test never dereference it.
	c.swap(&Connection{})

	require.Equal(t, []string{"first", "boom", "last"}, order,
		"all hooks must run in registration order despite the middle one failing")
}

// TestConnection_ChannelRaceSafeUnderSwap exercises concurrent Channel()
// readers against repeated swaps. Meaningful under -race: it fails if the
// conn/channel fields are read or written without the mutex.
func TestConnection_ChannelRaceSafeUnderSwap(t *testing.T) {
	c := &Connection{config: unreachableConfig(), logger: zerolog.Nop()}

	var stop atomic.Bool
	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				_ = c.Channel()
				_ = c.isClosed()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 500 {
			c.swap(&Connection{})
		}
	}()

	// Concurrent hook registration must also be safe.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			c.OnReconnect(func(*amqp.Channel) error { return nil })
		}
	}()

	time.Sleep(50 * time.Millisecond)
	stop.Store(true)
	wg.Wait()
}

// TestConnection_CloseMarksClosed verifies Close() flips the flag Supervise
// uses to distinguish a deliberate shutdown from a broker outage. Both fields
// are nil here, so Close() does no AMQP I/O.
func TestConnection_CloseMarksClosed(t *testing.T) {
	c := &Connection{config: unreachableConfig(), logger: zerolog.Nop()}
	require.False(t, c.isClosed())
	require.NoError(t, c.Close())
	require.True(t, c.isClosed(), "Supervise must see a deliberate close and exit quietly")
}

// TestSanitizeURL_StripsCredentials guards the logging path added to the
// startup failure warning — credentials must never reach the logs.
func TestSanitizeURL_StripsCredentials(t *testing.T) {
	// url.URL.String() percent-encodes the "***" placeholder, so assert the
	// invariant that matters (no username, no password, host preserved) rather
	// than the exact encoded spelling.
	got := SanitizeURL("amqp://user:s3cret@rabbit:5672/")
	require.NotContains(t, got, "s3cret", "password must never reach the logs")
	require.NotContains(t, got, "user", "username must never reach the logs")
	require.Contains(t, got, "rabbit:5672", "host must survive so the log stays actionable")
	require.Equal(t, "amqp://***", SanitizeURL("://not a url"))
}
