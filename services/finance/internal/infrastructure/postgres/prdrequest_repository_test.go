// Package postgres_test provides integration tests for the prdrequest repository.
package postgres_test

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// openPrdRequestTestDB opens the finance test database using existing helpers.
// It reuses openTestDB (defined in product_repository_test.go) since both tables
// live in the same finance_db.
func openPrdRequestTestDB(t *testing.T) *postgres.DB {
	t.Helper()
	return openTestDB(t)
}

// =============================================================================
// Repository tests
// =============================================================================

// TestPrdRequestRepository_CreateThenGet verifies that a created request can be
// retrieved by ID with all fields preserved.
func TestPrdRequestRepository_CreateThenGet(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	repo := postgres.NewPrdRequestRepository(db)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	requesterID := uuid.New()
	deptID := uuid.New()
	req := seedPrdRequest(t, ctx, repo, gen, "Create Then Get", requesterID, deptID, "test-creator")
	defer cleanupPrdRequest(t, ctx, db, req.ID())

	got, err := repo.GetByID(ctx, req.ID())
	require.NoError(t, err)

	assert.Equal(t, req.ID(), got.ID())
	assert.Equal(t, req.TicketNo().String(), got.TicketNo().String())
	assert.Equal(t, "Create Then Get", got.Title().String())
	assert.Equal(t, prdrequest.StatusOpen.String(), got.Status().String())
	assert.Equal(t, requesterID, got.RequesterID())
	assert.Equal(t, deptID, got.RequesterDeptID())
	assert.Equal(t, "test-creator", got.CreatedBy())
	assert.Nil(t, got.UpdatedAt())
	assert.Nil(t, got.DeletedAt())
}

// TestPrdRequestRepository_GetByTicketNo verifies retrieval by ticket number string.
func TestPrdRequestRepository_GetByTicketNo(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	repo := postgres.NewPrdRequestRepository(db)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	req := seedPrdRequest(t, ctx, repo, gen, "Get By Ticket No", uuid.New(), uuid.New(), "tester")
	defer cleanupPrdRequest(t, ctx, db, req.ID())

	got, err := repo.GetByTicketNo(ctx, req.TicketNo().String())
	require.NoError(t, err)
	assert.Equal(t, req.ID(), got.ID())
	assert.Equal(t, req.TicketNo().String(), got.TicketNo().String())
}

// TestPrdRequestRepository_GetByID_NotFound verifies that querying a non-existent ID
// returns ErrNotFound.
func TestPrdRequestRepository_GetByID_NotFound(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	repo := postgres.NewPrdRequestRepository(db)
	ctx := context.Background()

	got, err := repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, prdrequest.ErrNotFound)
	assert.Nil(t, got)
}

// TestPrdRequestRepository_GetByID_Deleted_ReturnsNotFound verifies that a soft-deleted
// request is not returned by GetByID.
func TestPrdRequestRepository_GetByID_Deleted_ReturnsNotFound(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	repo := postgres.NewPrdRequestRepository(db)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	req := seedPrdRequest(t, ctx, repo, gen, "To Be Deleted Visibility", uuid.New(), uuid.New(), "tester")
	defer cleanupPrdRequest(t, ctx, db, req.ID())

	err := repo.Delete(ctx, req.ID(), "test-user")
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, req.ID())
	assert.ErrorIs(t, err, prdrequest.ErrNotFound)
	assert.Nil(t, got)
}

// TestPrdRequestRepository_Update_Success verifies status transitions through Update.
func TestPrdRequestRepository_Update_Success(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	repo := postgres.NewPrdRequestRepository(db)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	req := seedPrdRequest(t, ctx, repo, gen, "Update Success Request", uuid.New(), uuid.New(), "creator")
	defer cleanupPrdRequest(t, ctx, db, req.ID())

	// Transition OPEN → IN_REVIEW via Assign.
	assigneeID := uuid.New()
	err := req.Assign(assigneeID, "assigner")
	require.NoError(t, err)

	err = repo.Update(ctx, req)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, req.ID())
	require.NoError(t, err)
	assert.Equal(t, prdrequest.StatusInReview.String(), got.Status().String())
	assert.Equal(t, assigneeID, got.AssignedTo())
	assert.NotNil(t, got.UpdatedAt())

	// Transition IN_REVIEW → RESOLVED via Resolve.
	productID := uuid.New()
	err = req.Resolve(productID, "Looks good!", "resolver")
	require.NoError(t, err)

	err = repo.Update(ctx, req)
	require.NoError(t, err)

	got, err = repo.GetByID(ctx, req.ID())
	require.NoError(t, err)
	assert.Equal(t, prdrequest.StatusResolved.String(), got.Status().String())
	assert.Equal(t, productID, got.ResolvedProductID())
	assert.Equal(t, "Looks good!", got.ResolutionNote().String())
}

// TestPrdRequestRepository_Update_NotFound verifies that updating a non-existent
// request returns ErrNotFound.
func TestPrdRequestRepository_Update_NotFound(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	repo := postgres.NewPrdRequestRepository(db)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	// Build a detached request not in DB.
	period, err := prdrequest.PeriodForNow(time.Now())
	require.NoError(t, err)

	ticketNo, err := gen.Next(ctx, period)
	require.NoError(t, err)

	ghost, err := prdrequest.NewRequest(
		ticketNo,
		uuid.New(),
		"ghost-user",
		uuid.New(),
		"DEPT-GHOST",
		"Ghost Request",
		"",
		"",
		nil,
		"ghost-creator",
	)
	require.NoError(t, err)
	// Do not insert — just try to update
	err = repo.Update(ctx, ghost)
	assert.ErrorIs(t, err, prdrequest.ErrNotFound)
}

// TestPrdRequestRepository_List_Filters verifies each filter predicate independently.
func TestPrdRequestRepository_List_Filters(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	repo := postgres.NewPrdRequestRepository(db)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	requester1 := uuid.New()
	requester2 := uuid.New()
	dept1 := uuid.New()
	dept2 := uuid.New()

	// Seed 5 requests: 3 for requester1/dept1, 2 for requester2/dept2.
	requests := []*prdrequest.Request{
		seedPrdRequestFull(t, ctx, repo, gen, "Filter Request One", requester1, dept1, "user1"),
		seedPrdRequestFull(t, ctx, repo, gen, "Filter Request Two", requester1, dept1, "user1"),
		seedPrdRequestFull(t, ctx, repo, gen, "Filter Request Three", requester1, dept1, "user1"),
		seedPrdRequestFull(t, ctx, repo, gen, "Filter Request Four", requester2, dept2, "user2"),
		seedPrdRequestFull(t, ctx, repo, gen, "Filter Request Five", requester2, dept2, "user2"),
	}
	defer func() {
		for _, r := range requests {
			cleanupPrdRequest(t, ctx, db, r.ID())
		}
	}()

	t.Run("NoFilter_PaginationPage1PageSize2", func(t *testing.T) {
		items, total, err := repo.List(ctx, prdrequest.ListFilter{
			Page:     1,
			PageSize: 2,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 5)
		assert.Len(t, items, 2)
	})

	t.Run("FilterByStatus_OPEN", func(t *testing.T) {
		items, total, err := repo.List(ctx, prdrequest.ListFilter{
			Status:   "OPEN",
			Page:     1,
			PageSize: 20,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 5)
		for _, item := range items {
			assert.Equal(t, "OPEN", item.Status().String())
		}
	})

	t.Run("FilterByRequesterID", func(t *testing.T) {
		items, total, err := repo.List(ctx, prdrequest.ListFilter{
			RequesterID: &requester1,
			Page:        1,
			PageSize:    20,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 3)
		for _, item := range items {
			assert.Equal(t, requester1, item.RequesterID())
		}
	})

	t.Run("FilterByRequesterDeptID", func(t *testing.T) {
		items, total, err := repo.List(ctx, prdrequest.ListFilter{
			RequesterDeptID: &dept2,
			Page:            1,
			PageSize:        20,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 2)
		for _, item := range items {
			assert.Equal(t, dept2, item.RequesterDeptID())
		}
	})

	t.Run("Pagination_PageSize2", func(t *testing.T) {
		items, total, err := repo.List(ctx, prdrequest.ListFilter{
			Status:   "OPEN",
			Page:     1,
			PageSize: 2,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 5)
		assert.Len(t, items, 2)
	})
}

// TestPrdRequestRepository_List_FTS verifies full-text search against title/description.
func TestPrdRequestRepository_List_FTS(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	repo := postgres.NewPrdRequestRepository(db)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	req := seedPrdRequest(t, ctx, repo, gen, "UniquePrdFTSTitle Request", uuid.New(), uuid.New(), "fts-tester")
	defer cleanupPrdRequest(t, ctx, db, req.ID())

	items, total, err := repo.List(ctx, prdrequest.ListFilter{
		Search:   "UniquePrdFTSTitle",
		Page:     1,
		PageSize: 20,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, 1)
	assert.GreaterOrEqual(t, len(items), 1)
}

// TestPrdRequestRepository_List_SortField_Whitelist verifies that an invalid sort field
// returns an error.
func TestPrdRequestRepository_List_SortField_Whitelist(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	repo := postgres.NewPrdRequestRepository(db)
	ctx := context.Background()

	_, _, err := repo.List(ctx, prdrequest.ListFilter{
		SortField: "invalidColumn",
		Page:      1,
		PageSize:  10,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid sort field")
}

// TestPrdRequestRepository_Delete_RemovesFromGet verifies that deleting a request makes
// GetByID return ErrNotFound.
func TestPrdRequestRepository_Delete_RemovesFromGet(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	repo := postgres.NewPrdRequestRepository(db)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	req := seedPrdRequest(t, ctx, repo, gen, "To Delete Request", uuid.New(), uuid.New(), "tester")
	defer cleanupPrdRequest(t, ctx, db, req.ID())

	err := repo.Delete(ctx, req.ID(), "test-user")
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, req.ID())
	assert.ErrorIs(t, err, prdrequest.ErrNotFound)
	assert.Nil(t, got)
}

// =============================================================================
// TicketNoGenerator tests
// =============================================================================

// TestPrdRequestTicketNoGenerator_Sequential verifies that calling Next 3 times
// for the same period produces PR-YYYYMM-001, -002, -003.
func TestPrdRequestTicketNoGenerator_Sequential(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	// Use a test-specific period that won't conflict with real data.
	period := "209901"
	defer cleanupPrdRequestSequence(t, ctx, db, period)

	tn1, err := gen.Next(ctx, period)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("PR-%s-001", period), tn1.String())

	tn2, err := gen.Next(ctx, period)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("PR-%s-002", period), tn2.String())

	tn3, err := gen.Next(ctx, period)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("PR-%s-003", period), tn3.String())
}

// TestPrdRequestTicketNoGenerator_Concurrent verifies that 10 concurrent callers
// each get a unique, contiguous sequence number.
func TestPrdRequestTicketNoGenerator_Concurrent(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	period := "209902"
	defer cleanupPrdRequestSequence(t, ctx, db, period)

	const goroutines = 10
	results := make([]string, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			tn, err := gen.Next(ctx, period)
			results[i] = tn.String()
			errs[i] = err
		}()
	}
	wg.Wait()

	// All calls must succeed.
	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d failed", i)
	}

	// All ticket numbers must be unique.
	seen := make(map[string]bool, goroutines)
	for _, tn := range results {
		assert.NotEmpty(t, tn)
		assert.False(t, seen[tn], "duplicate ticket number: %s", tn)
		seen[tn] = true
	}

	// Sequence numbers must form a contiguous range [1..10].
	seqs := make([]int, 0, goroutines)
	for _, tn := range results {
		_, seq, err := prdrequest.ParseTicketNo(tn)
		require.NoError(t, err)
		seqs = append(seqs, seq)
	}
	sort.Ints(seqs)
	for i, seq := range seqs {
		assert.Equal(t, i+1, seq, "expected sequence %d, got %d", i+1, seq)
	}
}

// TestPrdRequestTicketNoGenerator_DifferentPeriods verifies that different periods
// have independent counters.
func TestPrdRequestTicketNoGenerator_DifferentPeriods(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	period1 := "209903"
	period2 := "209904"
	defer cleanupPrdRequestSequence(t, ctx, db, period1)
	defer cleanupPrdRequestSequence(t, ctx, db, period2)

	tn1, err := gen.Next(ctx, period1)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("PR-%s-001", period1), tn1.String())

	tn2, err := gen.Next(ctx, period2)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("PR-%s-001", period2), tn2.String())

	// Second call for period1 should be 002.
	tn3, err := gen.Next(ctx, period1)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("PR-%s-002", period1), tn3.String())

	// Second call for period2 should be 002.
	tn4, err := gen.Next(ctx, period2)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("PR-%s-002", period2), tn4.String())
}

// TestPrdRequestTicketNoGenerator_InvalidPeriod verifies that a period with wrong
// length returns ErrInvalidPeriod.
func TestPrdRequestTicketNoGenerator_InvalidPeriod(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openPrdRequestTestDB(t)
	gen := postgres.NewPrdRequestTicketNoGenerator(db)
	ctx := context.Background()

	badPeriods := []string{"", "2025", "20250101", "ABCDEF"}
	for _, p := range badPeriods {
		_, err := gen.Next(ctx, p)
		assert.ErrorIs(t, err, prdrequest.ErrInvalidPeriod, "period %q should return ErrInvalidPeriod", p)
	}
}

// =============================================================================
// Test helpers
// =============================================================================

// seedPrdRequest creates a minimal request using the generator and registers cleanup.
func seedPrdRequest(
	t *testing.T,
	ctx context.Context,
	repo prdrequest.Repository,
	gen prdrequest.TicketNoGenerator,
	title string,
	requesterID, deptID uuid.UUID,
	createdBy string,
) *prdrequest.Request {
	t.Helper()
	return seedPrdRequestFull(t, ctx, repo, gen, title, requesterID, deptID, createdBy)
}

// seedPrdRequestFull creates a request with all specified fields.
func seedPrdRequestFull(
	t *testing.T,
	ctx context.Context,
	repo prdrequest.Repository,
	gen prdrequest.TicketNoGenerator,
	title string,
	requesterID, deptID uuid.UUID,
	createdBy string,
) *prdrequest.Request {
	t.Helper()

	period, err := prdrequest.PeriodForNow(time.Now())
	require.NoError(t, err)

	ticketNo, err := gen.Next(ctx, period)
	require.NoError(t, err)

	req, err := prdrequest.NewRequest(
		ticketNo,
		requesterID,
		createdBy,
		deptID,
		"TEST-DEPT",
		title,
		"",
		"",
		nil,
		createdBy,
	)
	require.NoError(t, err)

	err = repo.Create(ctx, req)
	require.NoError(t, err)

	return req
}

// cleanupPrdRequest hard-deletes a prd_request row to restore database state after a test.
func cleanupPrdRequest(t *testing.T, ctx context.Context, db *postgres.DB, id uuid.UUID) {
	t.Helper()
	_, err := db.ExecContext(ctx, "DELETE FROM prd_request WHERE request_id = $1", id)
	if err != nil {
		t.Logf("warning: cleanup failed for prd_request %s: %v", id, err)
	}
}

// cleanupPrdRequestSequence deletes the sequence row for a test period.
func cleanupPrdRequestSequence(t *testing.T, ctx context.Context, db *postgres.DB, period string) {
	t.Helper()
	_, err := db.ExecContext(ctx, "DELETE FROM prd_request_sequence WHERE period = $1", period)
	if err != nil {
		t.Logf("warning: cleanup failed for prd_request_sequence period %s: %v", period, err)
	}
}
