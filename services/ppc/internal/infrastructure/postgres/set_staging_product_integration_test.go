// Gated by INTEGRATION_TEST=true; requires a reachable, migrated ppc_db.
// Covers the manual-pick write path on sales_order_staging: the row flips to
// MANUAL, and ApplyStagingResolutions must then leave it alone.
package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

// seedStagingRow inserts one unpulled staging row and returns its id.
func seedStagingRow(ctx context.Context, t *testing.T, db *postgres.DB, suffix int64, pulledToDemandID *int64) int64 {
	t.Helper()
	var sosID int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO sales_order_staging (
			sos_contract_no, sos_customer_code, sos_customer_name,
			sos_item_code, sos_shade_code, sos_qty_ordered, sos_qty_delivered,
			sos_qty_remaining, sos_deadline, sos_pulled_to_demand_id, sos_match_status
		) VALUES ($1, 'ITST', 'Integration Test', $2, 'SH1', 100, 0, 100, CURRENT_DATE + 30, $3, 'NOT_FOUND')
		RETURNING sos_id`,
		fmt.Sprintf("ITEST-%d", suffix),
		fmt.Sprintf("ITEM%d", suffix),
		pulledToDemandID,
	).Scan(&sosID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM sales_order_staging WHERE sos_id = $1`, sosID)
	})
	return sosID
}

func TestSetStagingProduct_MarksManualAndBlocksReresolution_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	db := openPPCDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	repo := postgres.NewDemandRepository(db)

	sosID := seedStagingRow(ctx, t, db, suffix, nil)

	row, err := repo.SetStagingProduct(ctx, sosID, 97073)
	require.NoError(t, err)
	require.NotNil(t, row.CpmProductSysID)
	assert.Equal(t, int64(97073), *row.CpmProductSysID)
	assert.Equal(t, demanddomain.MatchStatusManual, row.MatchStatus)
	assert.Equal(t, int32(1), row.MatchCount)
	assert.NotNil(t, row.MatchedAt)

	// A subsequent automatic resolution for the same (item, shade) pair must not
	// overwrite the planner's pick.
	other := int64(111111)
	_, err = repo.ApplyStagingResolutions(ctx, []demanddomain.ProductResolution{{
		Pair:            demanddomain.StagingPair{ItemCode: fmt.Sprintf("ITEM%d", suffix), ShadeCode: "SH1"},
		MatchCount:      1,
		CpmProductSysID: &other,
	}})
	require.NoError(t, err)

	after, err := repo.GetStagingByIDs(ctx, []int64{sosID})
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.NotNil(t, after[0].CpmProductSysID)
	assert.Equal(t, int64(97073), *after[0].CpmProductSysID, "MANUAL pick must survive automatic re-resolution")
	assert.Equal(t, demanddomain.MatchStatusManual, after[0].MatchStatus)
}

func TestSetStagingProduct_RejectsPulledRow_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	db := openPPCDB(t)
	ctx := context.Background()
	repo := postgres.NewDemandRepository(db)

	_, err := repo.SetStagingProduct(ctx, -1, 97073)
	require.ErrorIs(t, err, demanddomain.ErrStagingNotUpdatable)
}
