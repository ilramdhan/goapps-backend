package rabbitmq

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// fakeAcknowledger records Ack/Nack calls without touching a real AMQP
// channel, so Delivery can be driven directly in-process.
type fakeAcknowledger struct {
	mu     sync.Mutex
	acked  []uint64
	nacked []uint64
}

func (f *fakeAcknowledger) Ack(tag uint64, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acked = append(f.acked, tag)
	return nil
}

func (f *fakeAcknowledger) Nack(tag uint64, _ bool, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nacked = append(f.nacked, tag)
	return nil
}

func (f *fakeAcknowledger) Reject(tag uint64, _ bool) error {
	return f.Nack(tag, false, false)
}

func newTestDelivery(t *testing.T, ack *fakeAcknowledger, tag uint64, jobID string) amqp.Delivery {
	t.Helper()
	body, err := json.Marshal(JobMessage{JobID: jobID, JobType: "product_cost_sheet_export"})
	require.NoError(t, err)
	return amqp.Delivery{
		Acknowledger: ack,
		DeliveryTag:  tag,
		Body:         body,
	}
}

// TestConsumer_DispatchBoundsConcurrency drives dispatch() directly (bypassing
// the real AMQP Consume() call, which needs a live channel) with a handler
// that blocks until released, verifying that no more than `concurrency`
// handler invocations are ever in flight at once, and that every delivery is
// eventually ack'd. Run with -race to catch any data race in the pool's
// shared state (sem/wg/inFlight counter).
func TestConsumer_DispatchBoundsConcurrency(t *testing.T) {
	const concurrency = 4
	const totalJobs = 20

	var inFlight int32
	var maxInFlight int32
	release := make(chan struct{})

	handler := func(_ context.Context, _ JobMessage) error {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			prevMax := atomic.LoadInt32(&maxInFlight)
			if cur <= prevMax || atomic.CompareAndSwapInt32(&maxInFlight, prevMax, cur) {
				break
			}
		}
		<-release
		atomic.AddInt32(&inFlight, -1)
		return nil
	}

	c := NewConcurrentConsumer(nil, "test-queue", handler, zerolog.Nop(), concurrency)

	sem := make(chan struct{}, c.concurrency)
	var wg sync.WaitGroup
	ack := &fakeAcknowledger{}

	ctx := context.Background()
	// dispatch() blocks the caller when the pool is full (mirrors Start()'s
	// own delivery loop), so drive it from its own goroutine — otherwise the
	// test itself would deadlock waiting for a free slot before it ever
	// closes `release`.
	dispatchDone := make(chan struct{})
	go func() {
		defer close(dispatchDone)
		for i := 0; i < totalJobs; i++ {
			d := newTestDelivery(t, ack, uint64(i), "job-"+string(rune('A'+i%26)))
			c.dispatch(ctx, d, sem, &wg)
		}
	}()

	// Let the pool ramp up to its cap before releasing.
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&inFlight) == concurrency
	}, 2*time.Second, 5*time.Millisecond, "pool never reached full concurrency")

	close(release)
	<-dispatchDone // all totalJobs dispatched before Wait, so wg.Add races are impossible
	wg.Wait()

	require.LessOrEqual(t, int(atomic.LoadInt32(&maxInFlight)), concurrency,
		"more deliveries were processed concurrently than the configured bound")
	require.Equal(t, concurrency, int(atomic.LoadInt32(&maxInFlight)),
		"pool should have reached its configured concurrency at least once")

	ack.mu.Lock()
	defer ack.mu.Unlock()
	require.Len(t, ack.acked, totalJobs, "every delivery must be acked exactly once")
	require.Empty(t, ack.nacked)
}

// TestConsumer_DispatchSequentialWhenConcurrencyOne verifies that the default
// NewConsumer (concurrency 1) never runs two handler invocations in parallel,
// preserving the pre-existing strictly-sequential behavior for job types not
// opted into the pool.
func TestConsumer_DispatchSequentialWhenConcurrencyOne(t *testing.T) {
	var inFlight int32
	var sawOverlap int32

	handler := func(_ context.Context, _ JobMessage) error {
		if atomic.AddInt32(&inFlight, 1) > 1 {
			atomic.StoreInt32(&sawOverlap, 1)
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	}

	c := NewConsumer(nil, "test-queue-seq", handler, zerolog.Nop())
	require.Equal(t, 1, c.concurrency)

	sem := make(chan struct{}, c.concurrency)
	var wg sync.WaitGroup
	ack := &fakeAcknowledger{}
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		d := newTestDelivery(t, ack, uint64(i), "seq-job")
		c.dispatch(ctx, d, sem, &wg)
	}
	wg.Wait()

	require.Zero(t, atomic.LoadInt32(&sawOverlap), "concurrency-1 consumer must process deliveries strictly sequentially")

	ack.mu.Lock()
	defer ack.mu.Unlock()
	require.Len(t, ack.acked, 10)
}

// TestConsumer_HandlerErrorNacksDelivery preserves the existing
// nack-to-DLQ-on-error contract under the new dispatch path.
func TestConsumer_HandlerErrorNacksDelivery(t *testing.T) {
	handler := func(_ context.Context, _ JobMessage) error {
		return errBoom
	}
	c := NewConcurrentConsumer(nil, "test-queue-err", handler, zerolog.Nop(), 3)

	sem := make(chan struct{}, c.concurrency)
	var wg sync.WaitGroup
	ack := &fakeAcknowledger{}
	ctx := context.Background()

	d := newTestDelivery(t, ack, 42, "bad-job")
	c.dispatch(ctx, d, sem, &wg)
	wg.Wait()

	ack.mu.Lock()
	defer ack.mu.Unlock()
	require.Empty(t, ack.acked)
	require.Equal(t, []uint64{42}, ack.nacked)
}

var errBoom = errTest("boom")

type errTest string

func (e errTest) Error() string { return string(e) }
