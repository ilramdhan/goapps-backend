// Migration round-trip test for 000575 (product_lookup_sync job type).
// Local PostgreSQL only, via internal/testutil/pgcontainer.
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

const (
	preLookupSyncJob uint64 = 574
	lookupSyncJobVer uint64 = 575
)

func TestLookupSyncJobTypeMigration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "lookup_sync_job_type", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), lookupSyncJobVer)
	require.NoError(t, m.UpTo(ctx, preLookupSyncJob))

	insert := `INSERT INTO job_execution (job_code, job_type, created_by) VALUES ('JOB-LS-1', 'product_lookup_sync', 'test')`
	assertExecFails(ctx, t, db, insert, "chk_job_type")

	require.NoError(t, m.UpTo(ctx, lookupSyncJobVer))
	mustExec(ctx, t, db, insert)
	mustExec(ctx, t, db, `INSERT INTO job_execution (job_code, job_type, created_by) VALUES ('JOB-LS-2', 'erp_master_sync', 'test')`)

	mustExec(ctx, t, db, `DELETE FROM job_execution WHERE job_code = 'JOB-LS-1'`)
	require.NoError(t, m.DownTo(ctx, preLookupSyncJob))
	assertExecFails(ctx, t, db, insert, "chk_job_type")

	require.NoError(t, m.UpTo(ctx, lookupSyncJobVer))
	mustExec(ctx, t, db, insert)
}
