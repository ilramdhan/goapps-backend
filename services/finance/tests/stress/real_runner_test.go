//go:build stress

// real_runner_test.go — S8e.2 REAL in-process stress runner.
//
// Unlike the S1-S9 stubs in scenarios_test.go, this test wires the actual calc
// engine (costcalc.Service + postgres repos + ProductLoader + evaluator) in
// process and drives it over the seeded stress corpus exactly the way the
// orchestrator + worker fleet do in production:
//
//  1. resolve the product set (all stress_fixture FG route heads),
//  2. build the product-level dependency DAG (PRODUCT-RM edges, same query the
//     orchestrator's DagBuilder uses),
//  3. plan waves with Kahn's algorithm (pkg/costcalc.PlanWaves),
//  4. pack each wave into ~chunkSize chunks and run Service.ProcessChunk
//     sequentially (single in-process "worker"), timing every chunk,
//  5. aggregate per-chunk / per-product / total wall + extrapolate the wall a
//     fleet of N parallel workers would achieve (chunks within a wave are
//     independent).
//
// This measures the true compute+persist throughput of the engine against a
// real Postgres, which is the dominant cost at 12k scale. Network/RMQ hop and
// HPA reaction are additive and validated separately in-cluster (S9).
//
// Run:
//
//	make seed-stress PRODUCTS=12000 PARAMS=150 FORMULAS=30 PERIOD=202604
//	STRESS_TEST=true STRESS_PERIOD=202604 go test -tags=stress -timeout 30m \
//	    -run TestStressRealRun ./tests/stress/... -v
//	make stress-clean
//
// STRESS_PERIOD must match a period that has priced rows in cst_rm_cost
// (production data is period 202604) or every product BLOCKs on MISSING_RM_COST.
package stress

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"

	ccpkg "github.com/mutugading/goapps-backend/pkg/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const (
	stressActor     = "stress_fixture"
	defaultChunk    = 50
	defaultParallel = 50 // production worker HPA ceiling
)

// engine bundles the in-process calc service + its loader.
type engine struct {
	svc    *costcalc.Service
	loader costcalc.ProductLoader
	db     *sql.DB
}

func openStressDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://finance:finance123@localhost:5434/finance_db?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	return db
}

func buildEngine(t *testing.T, db *sql.DB) *engine {
	t.Helper()
	pg := postgres.NewDBFromSQL(db)
	loader := costcalc.NewProductLoader(db)
	svc := costcalc.NewService(
		postgres.NewCostCalcJobRepository(pg),
		postgres.NewCostCalcChunkRepository(pg),
		postgres.NewCostCalcJobProductRepository(pg),
		postgres.NewCostResultRepository(pg),
		postgres.NewCostAuditHistoryRepository(pg),
		loader,
		evaluator.NewCache(),
		nil, // auditEmitter — skip side channel
		nil, // jobTriggerPub — inline, no orchestrator
	)
	return &engine{svc: svc, loader: loader, db: db}
}

// resolveStressFGs returns the FG product_sys_ids that have a COMPLETE/LOCKED
// stress_fixture route head — the DAG traversal seed set.
func resolveStressFGs(ctx context.Context, t *testing.T, db *sql.DB) []int64 {
	t.Helper()
	const q = `
		SELECT DISTINCT crh.crh_product_sys_id
		FROM cost_route_head crh
		WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		  AND crh.crh_deleted_at IS NULL
		  AND crh.crh_created_by = $1
		ORDER BY 1`
	return scanInt64Col(ctx, t, db, q, stressActor)
}

// loadProductRMEdges mirrors the orchestrator's DagBuilder edge query: PRODUCT-
// type RM links downstream(seq product) -> upstream(rm product).
func loadProductRMEdges(ctx context.Context, t *testing.T, db *sql.DB, batch []int64) [][2]int64 {
	t.Helper()
	const q = `
		SELECT crs.crs_product_sys_id, crm.crm_rm_product_sys_id
		FROM cost_route_head crh
		JOIN cost_route_seq crs ON crs.crs_head_id = crh.crh_head_id
		JOIN cost_route_rm  crm ON crm.crm_seq_id  = crs.crs_seq_id
		WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		  AND crh.crh_deleted_at IS NULL
		  AND crs.crs_deleted_at IS NULL
		  AND crs.crs_product_sys_id = ANY($1)
		  AND crm.crm_rm_type = 'PRODUCT'
		  AND crm.crm_rm_product_sys_id IS NOT NULL`
	rows, err := db.QueryContext(ctx, q, pq.Array(batch))
	if err != nil {
		t.Fatalf("load edges: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out [][2]int64
	for rows.Next() {
		var d, u int64
		if err := rows.Scan(&d, &u); err != nil {
			t.Fatalf("scan edge: %v", err)
		}
		out = append(out, [2]int64{d, u})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("edge rows: %v", err)
	}
	return out
}

// buildGraph BFS-traverses PRODUCT-RM edges from the FG seed set, exactly like
// the orchestrator DagBuilder, returning the full product-level dependency DAG.
func buildGraph(ctx context.Context, t *testing.T, db *sql.DB, seed []int64) *ccpkg.DependencyGraph {
	t.Helper()
	g := ccpkg.NewDependencyGraph()
	visited := map[int64]bool{}
	frontier := append([]int64{}, seed...)
	for len(frontier) > 0 {
		batch := make([]int64, 0, len(frontier))
		for _, pid := range frontier {
			if visited[pid] {
				continue
			}
			visited[pid] = true
			g.AddNode(pid)
			batch = append(batch, pid)
		}
		if len(batch) == 0 {
			break
		}
		var next []int64
		for _, e := range loadProductRMEdges(ctx, t, db, batch) {
			g.AddEdge(e[0], e[1])
			if !visited[e[1]] {
				next = append(next, e[1])
			}
		}
		frontier = next
	}
	return g
}

// isolateFormulas makes the active formula catalog deterministic for the run:
//   - soft-deletes every active, non-stress formula (so LoadFormulas returns
//     only the S_FORMULA_* set and the COST_STAGE_OUT result-param unique index
//     is freed),
//   - repoints one stress formula to produce COST_STAGE_OUT (the param code
//     ComputeProduct extracts as the final cost).
//
// It returns a restore func (idempotent) that re-points the stress formula and
// un-deletes the foreign formulas. Registered via t.Cleanup so the textile demo
// catalog is left exactly as found, even on failure.
func isolateFormulas(ctx context.Context, t *testing.T, db *sql.DB) func() {
	t.Helper()
	foreignIDs := scanStringCol(ctx, t, db,
		`SELECT id::text FROM mst_formula
		 WHERE created_by <> $1 AND deleted_at IS NULL AND is_active = TRUE`, stressActor)
	if len(foreignIDs) > 0 {
		if _, err := db.ExecContext(ctx,
			`UPDATE mst_formula SET deleted_at = now() WHERE id::text = ANY($1)`,
			pq.Array(foreignIDs)); err != nil {
			t.Fatalf("soft-delete foreign formulas: %v", err)
		}
	}
	t.Logf("isolated formula catalog: deactivated %d foreign formulas", len(foreignIDs))

	var costParamID string
	if err := db.QueryRowContext(ctx,
		`SELECT id::text FROM mst_parameter WHERE param_code = 'COST_STAGE_OUT' AND deleted_at IS NULL LIMIT 1`).
		Scan(&costParamID); err != nil {
		t.Fatalf("find COST_STAGE_OUT param: %v", err)
	}

	var stressFID, oldResultID string
	if err := db.QueryRowContext(ctx,
		`SELECT id::text, result_param_id::text FROM mst_formula
		 WHERE created_by = $1 AND deleted_at IS NULL
		 ORDER BY formula_code DESC LIMIT 1`, stressActor).Scan(&stressFID, &oldResultID); err != nil {
		t.Fatalf("pick stress formula: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE mst_formula SET result_param_id = $1 WHERE id = $2`, costParamID, stressFID); err != nil {
		t.Fatalf("repoint stress formula to COST_STAGE_OUT: %v", err)
	}
	t.Logf("repointed stress formula %s -> COST_STAGE_OUT", stressFID)

	return func() {
		bg := context.Background()
		if _, err := db.ExecContext(bg,
			`UPDATE mst_formula SET result_param_id = $1 WHERE id = $2`, oldResultID, stressFID); err != nil {
			t.Logf("restore stress formula result_param: %v", err)
		}
		if len(foreignIDs) > 0 {
			if _, err := db.ExecContext(bg,
				`UPDATE mst_formula SET deleted_at = NULL WHERE id::text = ANY($1)`, pq.Array(foreignIDs)); err != nil {
				t.Logf("restore foreign formulas: %v", err)
			}
		}
	}
}

func scanStringCol(ctx context.Context, t *testing.T, db *sql.DB, q string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, v)
	}
	return out
}

// resolveProductHeadMap returns product_sys_id -> route_head_id for every stress
// product, using the same UNION the orchestrator uses: an FG resolves to its own
// head (rank 0); an intermediate resolves to a head of some route that contains
// it as a sequence (rank 1).
func resolveProductHeadMap(ctx context.Context, t *testing.T, db *sql.DB) map[int64]int64 {
	t.Helper()
	const q = `
		SELECT product_sys_id, head_id FROM (
			SELECT crh.crh_product_sys_id AS product_sys_id, crh.crh_head_id AS head_id, 0 AS rank
			FROM cost_route_head crh
			WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED') AND crh.crh_deleted_at IS NULL
			  AND crh.crh_created_by = $1
			UNION ALL
			SELECT crs.crs_product_sys_id AS product_sys_id, crs.crs_head_id AS head_id, 1 AS rank
			FROM cost_route_seq crs
			JOIN cost_route_head crh ON crh.crh_head_id = crs.crs_head_id
			WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED') AND crs.crs_deleted_at IS NULL
			  AND crs.crs_created_by = $1
		) ranked
		ORDER BY product_sys_id, rank`
	rows, err := db.QueryContext(ctx, q, stressActor)
	if err != nil {
		t.Fatalf("head map: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]int64{}
	for rows.Next() {
		var pid, hid int64
		if err := rows.Scan(&pid, &hid); err != nil {
			t.Fatalf("scan head map: %v", err)
		}
		if _, ok := out[pid]; !ok { // first (lowest rank) wins
			out[pid] = hid
		}
	}
	return out
}

func scanInt64Col(ctx context.Context, t *testing.T, db *sql.DB, q string, args ...any) []int64 {
	t.Helper()
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, v)
	}
	return out
}

// cleanupRun removes the cst_product_cost + cal_job rows this run produced so
// re-runs start clean and `make stress-clean` (which targets the corpus) leaves
// nothing orphaned.
func cleanupRun(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	// Delete everything tied to stress cal_job rows in FK-safe order.
	const jobSel = `SELECT cj_job_id FROM cal_job WHERE cj_created_by = $1`
	for _, q := range []string{
		`DELETE FROM aud_cost_history WHERE ach_new_job_id IN (` + jobSel + `) OR ach_old_job_id IN (` + jobSel + `)`,
		`DELETE FROM cal_job_product  WHERE cjp_job_id IN (` + jobSel + `)`,
		`DELETE FROM cal_job_chunk    WHERE cjc_job_id IN (` + jobSel + `)`,
		`DELETE FROM cst_product_cost WHERE cpc_job_id IN (` + jobSel + `)`,
		`DELETE FROM cal_job          WHERE cj_created_by = $1`,
	} {
		if _, err := db.ExecContext(ctx, q, stressActor); err != nil {
			t.Logf("cleanup (%s): %v", q, err)
		}
	}
}

// runResult captures the aggregate measurements of one full run.
type runResult struct {
	products     int
	acyclic      int
	cyclic       int
	waves        int
	maxWaveWidth int
	chunks       int
	chunkMS      []float64
	success      int
	blocked      int
	failed       int
	totalCompute time.Duration // sum of chunk wall (single worker)
	totalWall    time.Duration // includes planning + persistence
}

// TestStressRealRun is the headline measurement. It runs the full seeded corpus
// through the real engine and reports timing + scale, writing a JSON report and
// updating nothing in baseline.json automatically (operator copies numbers in).
func TestStressRealRun(t *testing.T) {
	requireStress(t)
	period := os.Getenv("STRESS_PERIOD")
	if period == "" {
		period = "202604"
	}
	chunkSize := defaultChunk

	ctx := context.Background()
	db := openStressDB(t)
	// LIFO: close registered first (runs last); row cleanup registered second
	// (runs before close) so it executes against a live connection.
	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() { cleanupRun(context.Background(), t, db) })
	// Start from a clean slate in case a prior run left rows.
	cleanupRun(ctx, t, db)

	eng := buildEngine(t, db)

	// Isolate the active formula catalog to the stress set so every product runs
	// the same ~30 formulas and one of them produces COST_STAGE_OUT. Restored on
	// cleanup so the textile demo data is left intact.
	restore := isolateFormulas(ctx, t, db)
	t.Cleanup(restore)

	wallStart := time.Now()

	fgs := resolveStressFGs(ctx, t, db)
	if len(fgs) == 0 {
		t.Skip("no stress_fixture COMPLETE route heads — run `make seed-stress PERIOD=202604` first")
	}
	t.Logf("stress FG seed set: %d", len(fgs))

	g := buildGraph(ctx, t, db, fgs)
	plan := ccpkg.PlanWaves(g)

	res := runResult{
		products: len(g.Nodes()),
		cyclic:   len(plan.Cyclic),
		waves:    len(plan.Waves),
	}
	for _, w := range plan.Waves {
		res.acyclic += len(w.Products)
		if len(w.Products) > res.maxWaveWidth {
			res.maxWaveWidth = len(w.Products)
		}
	}
	t.Logf("DAG: %d products | %d acyclic in %d waves | %d cyclic | widest wave=%d",
		res.products, res.acyclic, res.waves, res.cyclic, res.maxWaveWidth)

	// Create the cal_job so persisted cost rows have a valid FK target.
	job, err := costcalcdom.NewJob(period, costcalcdom.CalcTypeActual, costcalcdom.ScopeAll, nil, "STRESS", stressActor)
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	if err := postgres.NewCostCalcJobRepository(postgres.NewDBFromSQL(db)).Create(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	t.Logf("cal_job %s (id=%d) period=%s", job.Code(), job.ID(), period)

	// Create cal_job_product rows (resolving head per product) so MarkSuccess /
	// MarkBlocked record real status + block reasons we can aggregate afterward.
	headMap := resolveProductHeadMap(ctx, t, db)
	productRepo := postgres.NewCostCalcJobProductRepository(postgres.NewDBFromSQL(db))

	// Drive waves in order; chunk each wave; time every chunk.
	var perWaveWidths []int
	for _, w := range plan.Waves {
		perWaveWidths = append(perWaveWidths, len(w.Products))
		jps := make([]*costcalcdom.JobProduct, 0, len(w.Products))
		for _, pid := range w.Products {
			jps = append(jps, costcalcdom.NewJobProduct(job.ID(), pid, headMap[pid], w.Number))
		}
		if err := productRepo.BulkCreate(ctx, jps); err != nil {
			t.Fatalf("bulk create job_product wave=%d: %v", w.Number, err)
		}
		for start := 0; start < len(w.Products); start += chunkSize {
			end := start + chunkSize
			if end > len(w.Products) {
				end = len(w.Products)
			}
			chunkProducts := w.Products[start:end]

			t0 := time.Now()
			out, perr := eng.svc.ProcessChunk(ctx, costcalc.ProcessChunkInput{
				JobID:    job.ID(),
				ChunkID:  0,
				Period:   period,
				CalcType: costcalcdom.CalcTypeActual,
				Products: chunkProducts,
				Actor:    stressActor,
			})
			elapsed := time.Since(t0)
			if perr != nil {
				t.Fatalf("ProcessChunk wave=%d [%d:%d]: %v", w.Number, start, end, perr)
			}
			res.chunkMS = append(res.chunkMS, float64(elapsed.Microseconds())/1000.0)
			res.totalCompute += elapsed
			res.chunks++
			res.success += out.Success
			res.blocked += out.Blocked
			res.failed += out.Failed
		}
	}
	res.totalWall = time.Since(wallStart)

	logBlockReasons(ctx, t, db, job.ID())
	reportRun(t, period, chunkSize, res, perWaveWidths)
}

// logBlockReasons prints the distribution of cjp_block_reason for a job so we
// can see WHY products blocked (MISSING_RM_COST, FORMULA_ERROR, etc).
func logBlockReasons(ctx context.Context, t *testing.T, db *sql.DB, jobID int64) {
	t.Helper()
	const q = `
		SELECT COALESCE(cjp_status,'?'), COALESCE(cjp_block_reason,''), count(*)
		FROM cal_job_product WHERE cjp_job_id = $1
		GROUP BY 1,2 ORDER BY 3 DESC`
	rows, err := db.QueryContext(ctx, q, jobID)
	if err != nil {
		t.Logf("block reasons: %v", err)
		return
	}
	defer func() { _ = rows.Close() }()
	t.Logf("---- cal_job_product status/reason breakdown ----")
	for rows.Next() {
		var status, reason string
		var n int
		if err := rows.Scan(&status, &reason, &n); err != nil {
			t.Logf("scan: %v", err)
			return
		}
		t.Logf("  %-12s %-22s %d", status, reason, n)
	}
	// Surface one sample FORMULA_ERROR log so we can see the eval failure.
	var sample sql.NullString
	_ = db.QueryRowContext(ctx,
		`SELECT cjp_calculation_log::text FROM cal_job_product
		 WHERE cjp_job_id = $1 AND cjp_block_reason = 'FORMULA_ERROR' LIMIT 1`, jobID).Scan(&sample)
	if sample.Valid {
		t.Logf("  sample FORMULA_ERROR log: %s", sample.String)
	}
}

func reportRun(t *testing.T, period string, chunkSize int, res runResult, waveWidths []int) {
	t.Helper()
	p50 := percentile(append([]float64{}, res.chunkMS...), 50)
	p95 := percentile(append([]float64{}, res.chunkMS...), 95)
	p99 := percentile(append([]float64{}, res.chunkMS...), 99)
	var sumMS float64
	for _, v := range res.chunkMS {
		sumMS += v
	}
	processed := res.success + res.blocked + res.failed
	perProductMS := 0.0
	if processed > 0 {
		perProductMS = sumMS / float64(processed)
	}
	singleThroughput := 0.0
	if res.totalCompute > 0 {
		singleThroughput = float64(processed) / res.totalCompute.Seconds()
	}

	// Parallel extrapolation: chunks within a wave are independent, so a fleet of
	// W workers clears a wave of C chunks in ceil(C/W) "chunk slots". Using the
	// per-wave chunk count and the mean chunk wall gives a realistic fleet wall.
	meanChunkMS := 0.0
	if res.chunks > 0 {
		meanChunkMS = sumMS / float64(res.chunks)
	}
	fleetWall := func(workers int) time.Duration {
		var slots int
		for _, width := range waveWidths {
			chunksInWave := (width + chunkSize - 1) / chunkSize
			slotsInWave := (chunksInWave + workers - 1) / workers
			slots += slotsInWave
		}
		return time.Duration(float64(slots) * meanChunkMS * float64(time.Millisecond))
	}

	t.Logf("================ STRESS REAL RUN RESULT ================")
	t.Logf("period               : %s", period)
	t.Logf("chunk size           : %d products", chunkSize)
	t.Logf("products (DAG nodes)  : %d", res.products)
	t.Logf("  acyclic / waves     : %d in %d waves (widest %d)", res.acyclic, res.waves, res.maxWaveWidth)
	t.Logf("  cyclic (skipped)    : %d", res.cyclic)
	t.Logf("chunks executed       : %d", res.chunks)
	t.Logf("outcomes              : SUCCESS=%d BLOCKED=%d FAILED=%d", res.success, res.blocked, res.failed)
	t.Logf("-------------------------------------------------------")
	t.Logf("total wall (1 worker) : %s", res.totalWall.Round(time.Millisecond))
	t.Logf("  compute sum         : %s", res.totalCompute.Round(time.Millisecond))
	t.Logf("per-product mean      : %.3f ms", perProductMS)
	t.Logf("chunk p50/p95/p99 ms  : %.1f / %.1f / %.1f", p50, p95, p99)
	t.Logf("mean chunk ms         : %.1f", meanChunkMS)
	t.Logf("throughput (1 worker) : %.0f products/sec", singleThroughput)
	t.Logf("-------------------------------------------------------")
	t.Logf("extrapolated fleet wall (chunks parallel within wave):")
	for _, w := range []int{2, 10, 25, defaultParallel} {
		t.Logf("  %2d workers          : %s  (%.0f products/sec)", w, fleetWall(w).Round(time.Millisecond),
			float64(res.success+res.blocked+res.failed)/fleetWall(w).Seconds())
	}
	t.Logf("=======================================================")

	writeReport(t, ScenarioMetrics{
		Scenario:     "real_full_run",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		ProductCount: res.products,
		ChunkCount:   res.chunks,
		P50ChunkMS:   p50,
		P95ChunkMS:   p95,
		P99ChunkMS:   p99,
		TotalMS:      float64(res.totalWall.Milliseconds()),
		SuccessCount: res.success,
		BlockedCount: res.blocked,
		FailedCount:  res.failed,
		Notes: fmt.Sprintf("waves=%d widest=%d cyclic=%d perProductMs=%.3f fleet50=%s",
			res.waves, res.maxWaveWidth, res.cyclic, perProductMS, fleetWall(defaultParallel).Round(time.Millisecond)),
	})
}
