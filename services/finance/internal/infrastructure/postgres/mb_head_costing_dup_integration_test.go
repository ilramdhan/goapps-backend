package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Gated by INTEGRATION_TEST=true. Fixtures prefixed ITEST-DUPNAME- and hard-deleted around the test.
func TestMBHead_DuplicateCostingName_AllowedAndDeterministic(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	raw, err := sql.Open("pgx", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		mbRegenEnvOrDefault("TEST_DB_HOST", "localhost"), mbRegenEnvOrDefault("TEST_DB_PORT", "5434"),
		mbRegenEnvOrDefault("TEST_DB_USER", "finance"), mbRegenEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		mbRegenEnvOrDefault("TEST_DB_NAME", "finance_db")))
	require.NoError(t, err)
	require.NoError(t, mbRegenWaitForDB(raw, 10*time.Second))
	db := NewDBFromSQL(raw)
	defer func() { _ = db.Close() }()
	repo := NewMBHeadRepository(db, NewMBCompositionRepository(db))

	const name = "ITEST-DUPNAME-X"
	clean := func() {
		_, e := db.ExecContext(ctx, `DELETE FROM mst_mb_head WHERE mbh_mb_costing = $1`, name)
		require.NoError(t, e)
	}
	clean()
	defer clean()

	first, second, gone := uuid.New(), uuid.New(), uuid.New()
	ins := func(id uuid.UUID, createdAt time.Time, deleted any) {
		_, e := db.ExecContext(ctx, `INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, created_at, deleted_at, created_by)
			VALUES ($1,$2,$3,$4,'itest')`, id, name, createdAt, deleted)
		require.NoError(t, e, "two live heads with the same name must be insertable")
	}
	now := time.Now()
	ins(second, now, nil)
	ins(first, now.Add(-time.Hour), nil)
	ins(gone, now.Add(-2*time.Hour), now) // soft-deleted: never counted nor returned

	n, err := repo.CountByMBCosting(ctx, name)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	got, err := repo.GetByMBCosting(ctx, name)
	require.NoError(t, err)
	require.Equal(t, first, got.ID(), "oldest live head wins")
}
