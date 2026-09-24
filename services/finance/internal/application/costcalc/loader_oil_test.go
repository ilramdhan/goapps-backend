package costcalc

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadOilContext_Integration exercises loadOilContextQuery against a real,
// fully migrated database (>= 000521). It seeds its own oil product type, two
// RM groups (one oil-flagged default, one oil-flagged non-default, plus an
// UN-flagged group that must not appear in the allowed set), and three
// products: oil type with stored OIL_NAME, oil type with no OIL_NAME, and a
// non-oil type (must be absent from the result).
func TestLoadOilContext_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOr("TEST_DB_HOST", "localhost"), envOr("TEST_DB_PORT", "5434"),
		envOr("TEST_DB_USER", "finance"), envOr("TEST_DB_PASSWORD", "finance123"),
		envOr("TEST_DB_NAME", "finance_db"))
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, waitDB(db, 10*time.Second))
	t.Cleanup(func() { _ = db.Close() })

	var oilNameParamID string
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM mst_parameter WHERE param_code = 'OIL_NAME' AND deleted_at IS NULL`,
	).Scan(&oilNameParamID); err != nil {
		t.Skipf("OIL_NAME param not present in this database: %v", err)
	}

	f := seedOilContextFixture(ctx, t, db, oilNameParamID)

	got, err := NewProductLoader(db).LoadOilContext(ctx, []int64{f.withName, f.noName, f.nonOil})
	require.NoError(t, err)

	require.Contains(t, got, f.withName)
	w := got[f.withName]
	assert.Equal(t, OilClassSuperba, w.Class)
	assert.Equal(t, f.typeCode, w.TypeCode)
	assert.Equal(t, f.groupAlt, w.GroupCode, "stored OIL_NAME is returned trimmed")
	assert.Equal(t, f.groupDefault, w.DefaultGroup)
	assert.Equal(t, map[string]bool{f.groupDefault: true, f.groupAlt: true}, w.Allowed,
		"an un-flagged group mapped to the type must not be allowed")

	require.Contains(t, got, f.noName)
	assert.Empty(t, got[f.noName].GroupCode)
	assert.Equal(t, f.groupDefault, got[f.noName].DefaultGroup)

	assert.NotContains(t, got, f.nonOil, "a type without cpt_oil_class yields no oil context")

	empty, err := NewProductLoader(db).LoadOilContext(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

type oilContextFixture struct {
	typeCode, groupDefault, groupAlt string
	withName, noName, nonOil         int64
}

func seedOilContextFixture(ctx context.Context, t *testing.T, db *sql.DB, oilNameParamID string) oilContextFixture {
	t.Helper()
	const actor = "loader-oil-test"
	prefix := strings.ToUpper(uniqueCodePrefix(t, "O")) // "O" + 6 hex
	f := oilContextFixture{
		typeCode:     prefix[:5],
		groupDefault: prefix + "-DEF",
		groupAlt:     prefix + "-ALT",
	}
	groupOff := prefix + "-OFF"

	var oilTypeID, nonOilTypeID int
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name, cpt_oil_class)
		VALUES ($1, 'loader oil test', 'SUPERBA') RETURNING cpt_type_id`, f.typeCode).Scan(&oilTypeID))
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT cpt_type_id FROM cost_product_type WHERE cpt_oil_class IS NULL ORDER BY cpt_type_id LIMIT 1`,
	).Scan(&nonOilTypeID))

	insertGroup := func(code string, oil bool) string {
		var id string
		require.NoError(t, db.QueryRowContext(ctx, `
			INSERT INTO cst_rm_group_head (group_code, group_name, is_oil_group, created_by)
			VALUES ($1, $1, $2, $3) RETURNING group_head_id`, code, oil, actor).Scan(&id))
		return id
	}
	defID := insertGroup(f.groupDefault, true)
	altID := insertGroup(f.groupAlt, true)
	offID := insertGroup(groupOff, false)
	for _, m := range []struct {
		id  string
		def bool
	}{{defID, true}, {altID, false}, {offID, false}} {
		_, err := db.ExecContext(ctx, `
			INSERT INTO cost_product_type_oil_group (cptog_type_id, cptog_group_head_id, cptog_is_default, cptog_created_by)
			VALUES ($1, $2, $3, $4)`, oilTypeID, m.id, m.def, actor)
		require.NoError(t, err)
	}

	insertProduct := func(suffix string, typeID int) int64 {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx, `
			INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
			VALUES ($1, $2, 'loader oil test', $3, $3) RETURNING cpm_product_sys_id`,
			prefix+suffix, typeID, actor).Scan(&id))
		return id
	}
	f.withName = insertProduct("-A", oilTypeID)
	f.noName = insertProduct("-B", oilTypeID)
	f.nonOil = insertProduct("-C", nonOilTypeID)

	_, err := db.ExecContext(ctx, `
		INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_filled_by, cpp_created_by)
		VALUES ($1, $2, $3, $4, $4)`, f.withName, oilNameParamID, "  "+f.groupAlt+" ", actor)
	require.NoError(t, err)

	t.Cleanup(func() {
		ids := []int64{f.withName, f.noName, f.nonOil}
		for _, id := range ids {
			_, _ = db.ExecContext(ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = $1`, id)
		}
		_, _ = db.ExecContext(ctx, `DELETE FROM cost_product_type WHERE cpt_type_id = $1`, oilTypeID)
		_, _ = db.ExecContext(ctx, `DELETE FROM cst_rm_group_head WHERE created_by = $1`, actor)
	})
	return f
}
