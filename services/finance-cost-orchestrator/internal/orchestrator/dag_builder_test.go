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
