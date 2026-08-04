package orchestrator

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedJobAndChunk inserts a minimal cal_job + cal_job_chunk pair and returns
// their ids. The chunk is seeded in the state the coordinator actually observes
// on the happy path: already terminal, because both the worker (MarkCompleted)
// and the sweeper (MarkChunkFailed) set the terminal status BEFORE publishing
// the completion event.
func seedJobAndChunk(t *testing.T, db *sql.DB, chunkStatus string) (jobID, chunkID int64) {
	t.Helper()
	ctx := context.Background()

	err := db.QueryRowContext(ctx, `
		WITH new_code AS (SELECT generate_cal_job_code() AS code)
		INSERT INTO cal_job (
			cj_job_code, cj_period, cj_calculation_type, cj_scope,
			cj_product_filter, cj_status, cj_priority, cj_triggered_by, cj_created_by
		)
		SELECT (SELECT code FROM new_code), '209901', 'ACTUAL', 'FILTERED',
		       NULL::JSONB, 'PROCESSING', 5, 'test', 'test'
		RETURNING cj_job_id
	`).Scan(&jobID)
	require.NoError(t, err)

	err = db.QueryRowContext(ctx, `
		INSERT INTO cal_job_chunk (
			cjc_job_id, cjc_chunk_number, cjc_wave_no,
			cjc_product_ids, cjc_product_count, cjc_status
		) VALUES ($1, 1, 0, '[1,2,3]'::jsonb, 3, $2)
		RETURNING cjc_chunk_id
	`, jobID, chunkStatus).Scan(&chunkID)
	require.NoError(t, err)

	t.Cleanup(func() {
		//nolint:errcheck // best-effort cleanup in a test
		db.ExecContext(context.Background(), `DELETE FROM cal_job WHERE cj_job_id = $1`, jobID)
	})
	return jobID, chunkID
}

// TestClaimProgressCount_FirstCallWinsSecondLoses is the core P4d guarantee:
// exactly one caller may fold a chunk's counts into the job totals.
func TestClaimProgressCount_FirstCallWinsSecondLoses(t *testing.T) {
	db := openTestDB(t)
	repo := NewChunkRepo(db)
	ctx := context.Background()

	_, chunkID := seedJobAndChunk(t, db, statusSuccess)

	first, err := repo.ClaimProgressCount(ctx, chunkID)
	require.NoError(t, err)
	assert.True(t, first, "the first event for a chunk must win the claim")

	second, err := repo.ClaimProgressCount(ctx, chunkID)
	require.NoError(t, err)
	assert.False(t, second, "a duplicate event must not be allowed to count again")

	third, err := repo.ClaimProgressCount(ctx, chunkID)
	require.NoError(t, err)
	assert.False(t, third, "the claim stays taken however many duplicates arrive")
}

// TestClaimProgressCount_TerminalChunkStillClaimable is the regression guard for
// a bug this test was written to catch: an earlier attempt at P4d keyed
// idempotency off chunk status, which silently skipped EVERY chunk. The worker
// marks a chunk SUCCESS before it publishes, so on the happy path the chunk is
// already terminal when the coordinator first sees it — a status-based guard
// would refuse the one and only legitimate count and leave every job at zero.
func TestClaimProgressCount_TerminalChunkStillClaimable(t *testing.T) {
	db := openTestDB(t)
	repo := NewChunkRepo(db)
	ctx := context.Background()

	for _, status := range []string{statusSuccess, statusPartial, statusFailed} {
		t.Run(status, func(t *testing.T) {
			_, chunkID := seedJobAndChunk(t, db, status)

			claimed, err := repo.ClaimProgressCount(ctx, chunkID)
			require.NoError(t, err)
			assert.True(t, claimed,
				"a chunk already %s must still be countable once: the worker sets the terminal status before publishing", status)
		})
	}
}

// TestIncrementProgress_OnlyAppliedOnceViaClaim ties the claim to the counters
// it protects, reproducing the production symptom (job 39: 48 processed chunks
// against 43 total, 4366 successes against 4166 products) end to end.
func TestIncrementProgress_OnlyAppliedOnceViaClaim(t *testing.T) {
	db := openTestDB(t)
	chunkRepo := NewChunkRepo(db)
	jobRepo := NewJobRepo(db)
	ctx := context.Background()

	jobID, chunkID := seedJobAndChunk(t, db, statusSuccess)

	// Three deliveries of the same completion event.
	for range 3 {
		counted, err := chunkRepo.ClaimProgressCount(ctx, chunkID)
		require.NoError(t, err)
		if counted {
			require.NoError(t, jobRepo.IncrementProgress(ctx, jobID, 100, 0, 0))
		}
	}

	processed, _, succ, fail, blocked, err := jobRepo.GetProgress(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 1, processed, "processed_chunks must count the chunk once, not once per delivery")
	assert.Equal(t, 100, succ, "success_count must not be multiplied by the redelivery count")
	assert.Equal(t, 0, fail)
	assert.Equal(t, 0, blocked)
}

// TestFindStuckChunks_CoversProcessingNotJustDispatched locks the P4b widening.
// A worker that dies after MarkProcessing leaves the chunk PROCESSING forever;
// before the fix the sweeper only looked at DISPATCHED, so the wave never
// completed and the job hung with no timeout — the reported "stuck di tengah".
func TestFindStuckChunks_CoversProcessingNotJustDispatched(t *testing.T) {
	db := openTestDB(t)
	repo := NewChunkRepo(db)
	ctx := context.Background()

	_, dispatchedID := seedJobAndChunk(t, db, statusDispatched)
	_, processingID := seedJobAndChunk(t, db, statusProcessing)
	_, freshID := seedJobAndChunk(t, db, statusProcessing)

	// Age the first two past the threshold; leave the third recent.
	_, err := db.ExecContext(ctx,
		`UPDATE cal_job_chunk SET cjc_dispatched_at = now() - interval '1 hour'
		  WHERE cjc_chunk_id = $1`, dispatchedID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`UPDATE cal_job_chunk SET cjc_started_at = now() - interval '1 hour'
		  WHERE cjc_chunk_id = $1`, processingID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`UPDATE cal_job_chunk SET cjc_started_at = now() WHERE cjc_chunk_id = $1`, freshID)
	require.NoError(t, err)

	stuck, err := repo.FindStuckChunks(ctx, 10*60*1000*1000*1000) // 10 minutes
	require.NoError(t, err)

	found := map[int64]bool{}
	for _, s := range stuck {
		found[s.ChunkID] = true
	}
	assert.True(t, found[dispatchedID], "an aged DISPATCHED chunk must be swept")
	assert.True(t, found[processingID], "an aged PROCESSING chunk must be swept too — this is the P4b fix")
	assert.False(t, found[freshID], "a chunk inside the threshold must be left alone")
}

// TestMarkFailed_PersistsErrorSummary locks P4a: a job that dies in planning
// must say why in the row, not only in the pod log. Production job 32/33/34
// showed FAILED with an empty cj_error_summary, which is what sent the
// investigation to the logs in the first place.
func TestMarkFailed_PersistsErrorSummary(t *testing.T) {
	db := openTestDB(t)
	jobRepo := NewJobRepo(db)
	ctx := context.Background()

	jobID, _ := seedJobAndChunk(t, db, statusQueued)

	require.NoError(t, jobRepo.MarkFailed(ctx, jobID, "mark chunk 1437 dispatched: pq: unnamed prepared statement does not exist"))

	var status string
	var summary sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT cj_status, cj_error_summary FROM cal_job WHERE cj_job_id = $1`, jobID,
	).Scan(&status, &summary)
	require.NoError(t, err)

	assert.Equal(t, statusFailed, status)
	require.True(t, summary.Valid, "cj_error_summary must be populated, not NULL")
	assert.Contains(t, summary.String, "unnamed prepared statement")
}
