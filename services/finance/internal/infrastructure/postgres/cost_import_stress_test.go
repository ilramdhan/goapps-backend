package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx stdlib driver: required for COPY (staging repo).
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costimportetl"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// TestCostImportStreamingMemory is the v2 ETL memory stress test. It streams a
// large number of product-parameter rows (default 2.4M — the real params-only
// workload) through the staging COPY path and asserts the worker's resident heap
// stays bounded, proving the COPY streams (bounded channel buffer) rather than
// buffering the whole dataset — the structural fix for the old OOM.
//
// Gated by STRESS_TEST=true (and a reachable finance DB) so it never runs in the
// normal unit/integration sweep. Tune the row count with STRESS_ROWS.
//
//	STRESS_TEST=true INTEGRATION_TEST=true go test -run TestCostImportStreamingMemory \
//	  -timeout 20m ./internal/infrastructure/postgres/
func TestCostImportStreamingMemory(t *testing.T) {
	if os.Getenv("STRESS_TEST") != "true" {
		t.Skip("Skipping stress test. Set STRESS_TEST=true to run.")
	}

	rows := int64(2_400_000)
	if v := os.Getenv("STRESS_ROWS"); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		require.NoError(t, err)
		rows = parsed
	}

	ctx := context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"),
		getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"),
		getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"),
	)
	raw, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	require.NoError(t, waitForDB(raw, 10*time.Second))
	db := postgres.NewDBFromSQL(raw)
	defer func() { _ = db.Close() }()
	repo := postgres.NewCostImportStagingRepository(db)

	jobID := time.Now().UnixNano()
	defer func() { _ = repo.CleanupStaging(ctx, jobID) }()

	// Sample peak heap-in-use on a background ticker while the COPY runs.
	var peakHeapInUse uint64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		var ms runtime.MemStats
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				runtime.ReadMemStats(&ms)
				if ms.HeapInuse > atomic.LoadUint64(&peakHeapInUse) {
					atomic.StoreUint64(&peakHeapInUse, ms.HeapInuse)
				}
			}
		}
	}()

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	// Producer emits `rows` synthetic param rows from a single reused slice. The
	// repository COPIES each row before queueing it, so reuse here is safe and is
	// exactly the zero-accumulation pattern the streaming parser uses in prod.
	start := time.Now()
	produce := func(emit costimportetl.RowEmitter) error {
		buf := make([]string, 6)
		for i := int64(0); i < rows; i++ {
			// legacy_oracle_sys_id, param_code, data_type, value_numeric, value_text, value_flag
			buf[0] = "STRESS_MISSING" // intentionally unknown product → no real upsert
			buf[1] = "STRESS_PARAM"
			buf[2] = "NUMERIC"
			buf[3] = strconv.FormatInt(i%1000, 10)
			buf[4] = ""
			buf[5] = ""
			if err := emit(buf); err != nil {
				return err
			}
		}
		return nil
	}

	copied, err := repo.CopyStagingProductParameter(ctx, jobID, produce)
	require.NoError(t, err)
	elapsed := time.Since(start)

	close(stop)
	<-done

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	peak := atomic.LoadUint64(&peakHeapInUse)
	const mib = 1024 * 1024
	t.Logf("stress: copied=%d rows in %s (%.0f rows/s) | heapInuse before=%dMiB peak=%dMiB after=%dMiB",
		copied, elapsed.Round(time.Millisecond), float64(copied)/elapsed.Seconds(),
		before.HeapInuse/mib, peak/mib, after.HeapInuse/mib)

	require.Equal(t, rows, copied, "every produced row must be copied into staging")

	// The streaming COPY must NOT scale memory with row count. 2.4M six-field rows
	// would be hundreds of MB if buffered; the bounded channel keeps peak well under
	// the worker's 1Gi limit. Assert a generous-but-meaningful ceiling.
	const ceilingMiB = 512
	require.Less(t, peak/mib, uint64(ceilingMiB),
		"peak heap-in-use must stay under %d MiB for %d rows (streaming, not buffered)", ceilingMiB, rows)
}
