// Integration coverage for rm-ratio-denier repositories: cost_product_type.cpt_rm_ratio_mode
// and cst_rm_group_head.denier round-trips. Gated by INTEGRATION_TEST=true.
//
// SAFETY: fixtures use the ITRRD prefix and are hard-deleted on cleanup.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	cptdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func openRRDDB(t *testing.T) (*sql.DB, *postgres.DB) {
	t.Helper()
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	require.NoError(t, waitForDB(raw, 10*time.Second))
	t.Cleanup(func() { _ = raw.Close() })
	return raw, postgres.NewDBFromSQL(raw)
}

func TestCostProductTypeRMRatioMode_RoundTrip(t *testing.T) {
	raw, db := openRRDDB(t)
	ctx := context.Background()
	repo := postgres.NewCostProductTypeRepository(db)

	code := fmt.Sprintf("T%d", 1000+rand.IntN(8999))
	ct, err := cptdomain.New(code, "ITRRD type", cptdomain.RMRatioModeDenier)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, ct))
	t.Cleanup(func() {
		_, _ = raw.ExecContext(ctx, `DELETE FROM cost_product_type WHERE cpt_type_id = $1`, ct.TypeID())
	})

	got, err := repo.GetByID(ctx, ct.TypeID())
	require.NoError(t, err)
	require.Equal(t, cptdomain.RMRatioModeDenier, got.RMRatioMode())

	require.NoError(t, got.Update("ITRRD type", true, cptdomain.RMRatioModeInput))
	require.NoError(t, repo.Update(ctx, got))
	got, err = repo.GetByCode(ctx, code)
	require.NoError(t, err)
	require.Equal(t, cptdomain.RMRatioModeInput, got.RMRatioMode())

	def, err := cptdomain.New(fmt.Sprintf("U%d", 1000+rand.IntN(8999)), "ITRRD default", "")
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, def))
	t.Cleanup(func() {
		_, _ = raw.ExecContext(ctx, `DELETE FROM cost_product_type WHERE cpt_type_id = $1`, def.TypeID())
	})
	got, err = repo.GetByID(ctx, def.TypeID())
	require.NoError(t, err)
	require.Equal(t, cptdomain.RMRatioModeInput, got.RMRatioMode())
}

func TestRMGroupHeadDenier_RoundTrip(t *testing.T) {
	raw, db := openRRDDB(t)
	ctx := context.Background()
	repo := postgres.NewRMGroupRepository(db)

	code, err := rmgroup.NewCode(fmt.Sprintf("ITRRD-%d", rand.IntN(1_000_000)))
	require.NoError(t, err)
	head, err := rmgroup.NewHead(code, "ITRRD spandex", "", 0, 0, "itest")
	require.NoError(t, err)
	d := 40.0
	require.NoError(t, head.SetDenier(&d))
	head.SetDenierUseDraft(true)
	require.NoError(t, repo.CreateHead(ctx, head))
	t.Cleanup(func() {
		_, _ = raw.ExecContext(ctx, `DELETE FROM cst_rm_group_head_audit WHERE group_head_id = $1`, head.ID())
		_, _ = raw.ExecContext(ctx, `DELETE FROM cst_rm_group_head WHERE group_head_id = $1`, head.ID())
	})

	got, err := repo.GetHeadByID(ctx, head.ID())
	require.NoError(t, err)
	require.NotNil(t, got.Denier())
	require.InDelta(t, 40, *got.Denier(), 1e-9)
	require.True(t, got.DenierUseDraft())

	require.NoError(t, got.SetDenier(nil))
	got.SetDenierUseDraft(false)
	require.NoError(t, repo.UpdateHead(ctx, got))
	got, err = repo.GetHeadByID(ctx, head.ID())
	require.NoError(t, err)
	require.Nil(t, got.Denier())
	require.False(t, got.DenierUseDraft())
}
