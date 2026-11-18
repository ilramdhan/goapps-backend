package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

// Runs on a throwaway testcontainers PostgreSQL (never a real DB); gated by INTEGRATION_TEST=true.
func TestMBActiveSyncSnapshotReader(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "mbas_snapshot", "")
	m, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, m.Up(ctx))

	exec := func(q string, args ...any) {
		t.Helper()
		_, err := raw.ExecContext(ctx, q, args...)
		require.NoError(t, err)
	}

	var poyType int64
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'POY'`).Scan(&poyType))
	var spParam uuid.UUID
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT id FROM mst_parameter WHERE param_code = 'MB_SP_CODE' AND deleted_at IS NULL LIMIT 1`).Scan(&spParam))

	var poyID, mbProd int64
	require.NoError(t, raw.QueryRowContext(ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_erp_item_code, cpm_shade_code, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ('ITEST-POY', $1, 'POY 380/144', 'POY-1', 'Y145T', TRUE, 'itest', 'itest') RETURNING cpm_product_sys_id`, poyType).Scan(&poyID))
	require.NoError(t, raw.QueryRowContext(ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ('ITEST-PROD', $1, 'prod', TRUE, 'itest', 'itest') RETURNING cpm_product_sys_id`, poyType).Scan(&mbProd))

	headLive, headDead := uuid.New(), uuid.New()
	exec(`INSERT INTO mst_mb_head (mbh_id, mbh_oracle_sys_id, mbh_mb_costing, mbh_is_active, mbh_check_status, mbh_current_version, mbh_entry_status, mbh_shade_code, mbh_code, created_by)
		VALUES ($1, 'SYS1', 'H-LIVE', TRUE, 'Valid', 1, 'VALIDATED', 'Y-145-T', 'C-145', 'itest')`, headLive)
	exec(`INSERT INTO mst_mb_head (mbh_id, mbh_oracle_sys_id, mbh_mb_costing, mbh_is_active, mbh_current_version, mbh_entry_status, created_by, deleted_at)
		VALUES ($1, 'SYS2', 'H-DEAD', FALSE, 1, 'VALIDATED', 'itest', NOW())`, headDead)

	headShared := uuid.New()
	exec(`INSERT INTO mst_mb_head (mbh_id, mbh_oracle_sys_id, mbh_mb_costing, mbh_is_active, mbh_check_status, mbh_current_version, mbh_entry_status, mbh_shade_code, created_by)
		VALUES ($1, 'SYS3', 'H-SHARED', TRUE, 'Valid', 1, 'VALIDATED', 'Y-145-T', 'itest')`, headShared)

	spin := uuid.New()
	delSpin := uuid.New()
	exec(`INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_orion_item_code, mbs_denier, mbs_filament, mbs_lesture,
			mbs_shade_code, mbs_run_ldr_pct, mbs_ldr_is_actual, mbs_fg_product_id, mbs_fg_link_source, mbs_is_active, created_at, created_by)
		VALUES ($1, $2, 'S1', 'S1', 'orion-1', 380, 144, 'TBL', 'Y-145-T', 7.5, TRUE, $3, 'RECIPE', TRUE, NOW(), 'itest')`, spin, headLive, poyID)
	// cross_section wins over a stale lesture (the sync writer sets only cross_section).
	csSpin := uuid.New()
	exec(`INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_lesture, mbs_cross_section, mbs_is_active, created_at, created_by)
		VALUES ($1, $2, 'CS', 'CS', 'TBL', 'RND', TRUE, NOW(), 'itest')`, csSpin, headLive)
	exec(`INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_is_active, created_at, created_by, deleted_at)
		VALUES ($1, $2, 'DEL', 'DEL', TRUE, NOW(), 'itest', NOW())`, delSpin, headLive)
	exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_value_mb_spin_id, cpp_filled_by, cpp_created_by)
		VALUES ($1, $2, 'ORION-1', $3, 'manual', 'itest')`, mbProd, spParam, spin)

	// Two spins sharing ORION-2: one pointer-less text row; a third pair sharing ORION-3 with no text rows.
	var textProd int64
	require.NoError(t, raw.QueryRowContext(ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ('ITEST-PROD2', $1, 'prod2', TRUE, 'itest', 'itest') RETURNING cpm_product_sys_id`, poyType).Scan(&textProd))
	sh2a, sh2b, sh3a, sh3b := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, x := range []struct {
		id   uuid.UUID
		code string
	}{{sh2a, "orion-2"}, {sh2b, "orion-2"}, {sh3a, "orion-3"}, {sh3b, "orion-3"}} {
		exec(`INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_orion_item_code, mbs_is_active, created_at, created_by)
			VALUES ($1, $2, 'SH', 'SH', $3, TRUE, NOW(), 'itest')`, x.id, headShared, x.code)
	}
	exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_filled_by, cpp_created_by)
		VALUES ($1, $2, 'ORION-2', 'manual', 'itest')`, textProd, spParam)

	reader := postgres.NewMBActiveSyncSnapshotReader(postgres.NewDBFromSQL(raw))
	snap, err := reader.Load(ctx)
	require.NoError(t, err)

	byID := map[uuid.UUID]int{}
	for i, h := range snap.Heads {
		byID[h.ID] = i
	}
	require.Contains(t, byID, headLive)
	require.Contains(t, byID, headDead)
	hl := snap.Heads[byID[headLive]]
	require.Equal(t, "SYS1", hl.SysID)
	require.True(t, hl.Live())
	require.Equal(t, "Valid", hl.CheckStatus)
	require.Equal(t, "Y-145-T", hl.ShadeCode)
	require.Equal(t, "C-145", hl.Code)
	require.True(t, snap.Heads[byID[headDead]].Deleted)
	require.False(t, snap.Heads[byID[headDead]].Live())

	var mine []int
	for i, sp := range snap.Spins {
		if sp.HeadID == headLive {
			mine = append(mine, i)
		}
	}
	require.Len(t, mine, 2, "deleted spin excluded")
	var s mbactivesync.SpinSnap
	for _, i := range mine {
		switch snap.Spins[i].ID {
		case spin:
			s = snap.Spins[i]
		case csSpin:
			require.Equal(t, "RND", snap.Spins[i].CrossSection, "cross_section preferred over lesture")
		}
	}
	require.Equal(t, spin, s.ID)
	require.Equal(t, "TBL", s.CrossSection)
	require.NotNil(t, s.Denier)
	require.InDelta(t, 380, *s.Denier, 1e-9)
	require.NotNil(t, s.RunLDRPct)
	require.InDelta(t, 7.5, *s.RunLDRPct, 1e-9)
	require.True(t, s.LDRIsActual)
	require.NotNil(t, s.FGProductID)
	require.Equal(t, poyID, *s.FGProductID)
	require.Equal(t, 1, s.PointerProducts)
	require.Equal(t, 0, s.TextProducts)

	byS := map[uuid.UUID]int{}
	for i, sp := range snap.Spins {
		byS[sp.ID] = i
	}
	require.NotContains(t, byS, delSpin, "deleted spin excluded")
	for _, id := range []uuid.UUID{sh2a, sh2b} {
		sp := snap.Spins[byS[id]]
		require.Equal(t, 1, sp.TextProducts, "shared key still reports text rows")
		require.True(t, sp.TextShared)
	}
	for _, id := range []uuid.UUID{sh3a, sh3b} {
		sp := snap.Spins[byS[id]]
		require.Equal(t, 0, sp.TextProducts)
		require.False(t, sp.TextShared, "shared key without text rows is not ambiguous")
	}

	var gotPOY bool
	for _, p := range snap.POYs {
		if p.SysID == poyID {
			gotPOY = true
			require.Equal(t, "POY-1", p.ERPItemCode)
			require.Equal(t, "Y145T", p.ShadeCode)
		}
	}
	require.True(t, gotPOY)
	require.Equal(t, "mbs_run_ldr_pct", snap.DozingSourceColumn)

	// The snapshot tx must be read-only: a write issued the same way is rejected.
	require.NoError(t, postgres.ProbeMBActiveSyncSnapshotTxReadOnly(ctx, postgres.NewDBFromSQL(raw)))
}
