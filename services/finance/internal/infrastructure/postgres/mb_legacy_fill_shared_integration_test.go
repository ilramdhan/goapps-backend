package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

// lfShared is the one container + migrated template database every legacy-fill case
// clones from. Starting a container and running every migration per case (~8.5s each)
// pushed the package past the CI `go test` 10-minute limit.
type lfShared struct {
	srv      *pgcontainer.Server
	template string
}

var lfSharedSrv *lfShared

// newLFFixture clones the shared migrated template (inside TestLegacyFillIntegration) or
// falls back to a fresh container when a case is run on its own.
func newLFFixture(t *testing.T, name string) *mbasFixture {
	t.Helper()
	s := lfSharedSrv
	if s == nil {
		return newMBASFixture(t, name)
	}
	ctx := context.Background()
	raw := s.srv.CreateDatabase(t, name, s.template)
	f := &mbasFixture{t: t, ctx: ctx, raw: raw, w: postgres.NewMBActiveSyncWriter(postgres.NewDBFromSQL(raw))}
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'POY'`).Scan(&f.poyTyp))
	return f
}

// TestLegacyFillIntegration runs every legacy-fill integration case against one
// container; each case gets its own database cloned from a migrated template.
func TestLegacyFillIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	const tpl = "mblf_template"
	tdb := srv.CreateDatabase(t, tpl, "")
	m, err := pgcontainer.NewMigrator(ctx, tdb, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, m.Up(ctx))
	require.NoError(t, tdb.Close()) // a template must have no open connections
	lfSharedSrv = &lfShared{srv: srv, template: tpl}
	t.Cleanup(func() { lfSharedSrv = nil })

	cases := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"MBLegacyFillReader_Integration", lfCaseMBLegacyFillReader_Integration},
		{"LegacyFillRevert_RestoresAndIdempotent", lfCaseLegacyFillRevert_RestoresAndIdempotent},
		{"LegacyFillRevert_ChangedSinceApply", lfCaseLegacyFillRevert_ChangedSinceApply},
		{"LegacyFillRevert_NonAppliedAndShadeCount", lfCaseLegacyFillRevert_NonAppliedAndShadeCount},
		{"LegacyFillRevert_CascadeTargetChangedIsPartial", lfCaseLegacyFillRevert_CascadeTargetChangedIsPartial},
		{"LegacyFillWriter_ClassL", lfCaseLegacyFillWriter_ClassL},
		{"LegacyFillWriter_ClassS", lfCaseLegacyFillWriter_ClassS},
		{"LegacyFillWriter_ClassD_CascadeSpinWins", lfCaseLegacyFillWriter_ClassD_CascadeSpinWins},
		{"LegacyFillWriter_ClassD_UniquenessReject", lfCaseLegacyFillWriter_ClassD_UniquenessReject},
		{"LegacyFillWriter_ClassD_CostingDuplicateAllowed", lfCaseLegacyFillWriter_ClassD_CostingDuplicateAllowed},
		{"LegacyFillWriter_ClassS_Guards", lfCaseLegacyFillWriter_ClassS_Guards},
		{"LegacyFillWriter_SavepointIsolation", lfCaseLegacyFillWriter_SavepointIsolation},
		{"LegacyFillWriter_MissingAndDeletedSpin", lfCaseLegacyFillWriter_MissingAndDeletedSpin},
		{"LegacyFillWriter_HeadFirstOrdering", lfCaseLegacyFillWriter_HeadFirstOrdering},
		{"LegacyFillWriter_ClassD_CodeSharedAndFgShadeGuard", lfCaseLegacyFillWriter_ClassD_CodeSharedAndFgShadeGuard},
		{"LegacyFillWriter_Targeted_DevOnlyTouchesNoSpin", lfCaseLegacyFillWriter_Targeted_DevOnlyTouchesNoSpin},
		{"LegacyFillWriter_Targeted_DenierFollowersOnly_AndRevert", lfCaseLegacyFillWriter_Targeted_DenierFollowersOnly_AndRevert},
		{"LegacyFillWriter_Targeted_CostingRename", lfCaseLegacyFillWriter_Targeted_CostingRename},
		{"LegacyFillWriter_CostingRename_NameFollowsAndRevert", lfCaseLegacyFillWriter_CostingRename_NameFollowsAndRevert},
		{"LegacyFillWriter_HeadMgtNameRejected", lfCaseLegacyFillWriter_HeadMgtNameRejected},
		{"LegacyFillWriter_ClassD_SpinOrionUniquenessReject", lfCaseLegacyFillWriter_ClassD_SpinOrionUniquenessReject},
	}
	for _, c := range cases {
		t.Run(c.name, c.fn)
	}
}
