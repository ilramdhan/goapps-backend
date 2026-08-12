package orchestrator

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/pkg/costcalc"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("INTEGRATION_TEST not set")
	}
	host := envOr("TEST_DB_HOST", "localhost")
	port := envOr("TEST_DB_PORT", "5434")
	user := envOr("TEST_DB_USER", "finance")
	pass := envOr("TEST_DB_PASSWORD", "finance123")
	name := envOr("TEST_DB_NAME", "finance_db")
	dsn := "host=" + host + " port=" + port + " user=" + user + " password=" + pass + " dbname=" + name + " sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, db.Ping())
	return db
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func TestDagBuilder_SingleProduct_HappyPath(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	b := NewDagBuilder(db)

	// Find an FG product whose route has at least one PRODUCT-type RM.
	var fgID int64
	err := db.QueryRowContext(context.Background(), `
		SELECT DISTINCT crs.crs_product_sys_id
		FROM cost_route_head crh
		JOIN cost_route_seq crs ON crs.crs_head_id = crh.crh_head_id
		JOIN cost_route_rm crm ON crm.crm_seq_id = crs.crs_seq_id
		WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		  AND crh.crh_deleted_at IS NULL
		  AND crm.crm_rm_type = 'PRODUCT'
		  AND crm.crm_rm_product_sys_id IS NOT NULL
		LIMIT 1
	`).Scan(&fgID)
	if err == sql.ErrNoRows {
		t.Skip("no FG with PRODUCT-type RM found in dev DB; skip")
	}
	require.NoError(t, err)

	g, nodes, err := b.Build(context.Background(), ScopeInput{
		Scope:        costcalc.ScopeSingleProduct,
		ProductSysID: fgID,
	})
	require.NoError(t, err)
	require.NotNil(t, g)
	require.GreaterOrEqual(t, len(nodes), 2, "expected at least FG + 1 upstream")
	require.True(t, g.HasNode(fgID), "FG must be in graph")
	require.NotEmpty(t, g.Upstream(fgID))
}

func TestDagBuilder_All_TraversesEverything(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	b := NewDagBuilder(db)
	g, nodes, err := b.Build(context.Background(), ScopeInput{Scope: costcalc.ScopeAll})
	require.NoError(t, err)
	require.NotNil(t, g)
	if len(nodes) == 0 {
		t.Skip("no active routes in dev DB")
	}
	for _, n := range nodes {
		require.True(t, g.HasNode(n))
	}
}

// TestDagBuilder_All_NoHeadlessNodes asserts the invariant that makes the
// cal_job_product persist safe: every node in a ScopeAll graph resolves to an
// active route head. A PRODUCT-type RM target with no active route (a raw cost
// input) must be excluded from the graph by loadProductRMEdges — otherwise it
// becomes a headless node and the bulk insert hits cjp_route_head_id NOT NULL,
// failing the whole job (the production 202605 failure).
func TestDagBuilder_All_NoHeadlessNodes(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	b := NewDagBuilder(db)
	_, nodes, err := b.Build(context.Background(), ScopeInput{Scope: costcalc.ScopeAll})
	require.NoError(t, err)
	if len(nodes) == 0 {
		t.Skip("no active routes in dev DB")
	}

	routeMap, err := NewJobProductRepo(db).ResolveProductRouteMap(context.Background(), nodes)
	require.NoError(t, err)

	var headless []int64
	for _, n := range nodes {
		if _, ok := routeMap[n]; !ok {
			headless = append(headless, n)
		}
	}
	require.Empty(t, headless, "every ScopeAll node must resolve to an active route head (no headless RM-input leaves in the graph)")
}

func TestDagBuilder_SingleProduct_NoRoute_EmptyGraph(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	b := NewDagBuilder(db)
	g, nodes, err := b.Build(context.Background(), ScopeInput{
		Scope:        costcalc.ScopeSingleProduct,
		ProductSysID: 999999999, // sure-not-exist
	})
	require.NoError(t, err)
	require.NotNil(t, g)
	require.Equal(t, []int64{999999999}, nodes)
	require.Empty(t, g.Upstream(999999999))
}

func TestDagBuilder_UnknownScope_Errors(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	b := NewDagBuilder(db)
	_, _, err := b.Build(context.Background(), ScopeInput{Scope: "BOGUS"})
	require.Error(t, err)
}

func TestDagBuilder_Filtered_RequiresTypeID(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	b := NewDagBuilder(db)
	_, _, err := b.Build(context.Background(), ScopeInput{Scope: costcalc.ScopeFiltered})
	require.Error(t, err)
}

// mbTypeID returns the MB product type id, skipping when the dev DB has none.
func mbTypeID(t *testing.T, db *sql.DB) int32 {
	t.Helper()
	var id int32
	err := db.QueryRowContext(context.Background(),
		`SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = $1`, typeCodeMB).Scan(&id)
	if err == sql.ErrNoRows {
		t.Skip("no MB product type in dev DB; skip")
	}
	require.NoError(t, err)
	return id
}

// A FILTERED job naming the MB type is a user mistake, not an empty result: MB is owned
// by the MB Batch path. It must fail loudly rather than report SUCCESS having computed
// nothing.
func TestDagBuilder_FilteredByMBType_Rejected(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	b := NewDagBuilder(db)

	_, _, err := b.Build(context.Background(), ScopeInput{
		Scope:               costcalc.ScopeFiltered,
		ProductTypeIDFilter: mbTypeID(t, db),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMBScopeNotAllowed)
}

// Same rule when the MB is named directly by product id.
func TestDagBuilder_SingleProductMB_Rejected(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	var mbProduct int64
	err := db.QueryRowContext(context.Background(), `
		SELECT pm.cpm_product_sys_id
		FROM cost_product_master pm
		JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
		WHERE pt.cpt_type_code = $1
		LIMIT 1`, typeCodeMB).Scan(&mbProduct)
	if err == sql.ErrNoRows {
		t.Skip("no MB products in dev DB; skip")
	}
	require.NoError(t, err)

	b := NewDagBuilder(db)
	_, _, err = b.Build(context.Background(), ScopeInput{
		Scope:        costcalc.ScopeSingleProduct,
		ProductSysID: mbProduct,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMBScopeNotAllowed)
}

// ALL excludes MB silently — the user asked for the calc engine's own population, which
// does not include MB. This is the assertion that the exclusion predicate actually bites.
func TestDagBuilder_All_ExcludesMBFromSeedSet(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	seed, err := NewDagBuilder(db).allActiveProducts(context.Background())
	require.NoError(t, err)
	if len(seed) == 0 {
		t.Skip("no active routes in dev DB")
	}

	rows, err := db.QueryContext(context.Background(), `
		SELECT pm.cpm_product_sys_id
		FROM cost_product_master pm
		JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
		WHERE pt.cpt_type_code = $1`, typeCodeMB)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	mb := map[int64]bool{}
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		mb[id] = true
	}
	require.NoError(t, rows.Err())
	if len(mb) == 0 {
		t.Skip("no MB products in dev DB; nothing to exclude")
	}

	for _, id := range seed {
		require.False(t, mb[id], "MB product %d must not be in the ScopeAll seed set", id)
	}
}

// The other product types must be entirely unaffected — this is the guarantee that the MB
// exclusion does not disturb the yarn path. Every non-MB product with an active route is
// still selected.
func TestDagBuilder_All_KeepsEveryNonMBProduct(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	seed, err := NewDagBuilder(db).allActiveProducts(context.Background())
	require.NoError(t, err)
	if len(seed) == 0 {
		t.Skip("no active routes in dev DB")
	}
	got := map[int64]bool{}
	for _, id := range seed {
		got[id] = true
	}

	rows, err := db.QueryContext(context.Background(), `
		SELECT DISTINCT crh.crh_product_sys_id
		FROM cost_route_head crh
		WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		  AND crh.crh_deleted_at IS NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM cost_product_master pm
		    JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
		    WHERE pm.cpm_product_sys_id = crh.crh_product_sys_id
		      AND pt.cpt_type_code = $1
		  )`, typeCodeMB)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		require.True(t, got[id], "non-MB product %d with an active route must still be selected", id)
	}
	require.NoError(t, rows.Err())
}
