//go:build stress

// seed_calc_corpus.go — S8e.1 stress fixture generator for the calc engine.
//
// Generates a synthetic 12k-product textile FG corpus that mirrors the shape
// of production data: cost_product_master + cost_route_head/seq/rm + 150
// mst_parameter + 30 mst_formula + per-product CAPP/CPP rows. Designed to
// validate that the calc engine scales to ~12_000 products * 150 params + 30
// formulas in roughly 5 minutes wall-clock with 50 worker pods.
//
// All rows are tagged created_by='stress_fixture' so they can be torn down
// atomically by the matching `stress-clean` Makefile target.
//
// CLI:
//
//	go run -tags=stress ./tests/stress/fixtures/seed_calc_corpus.go \
//	    -products=12000 -params=150 -formulas=30 -period=202604
//
// or via Makefile:
//
//	make seed-stress PRODUCTS=12000 PARAMS=150 FORMULAS=30 PERIOD=202604
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	stressOwner = "stress_fixture"
)

type config struct {
	products    int
	params      int
	formulas    int
	period      string
	databaseURL string
}

func main() {
	cfg := parseFlags()

	db, err := sql.Open("postgres", cfg.databaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Larger connection budget — bulk COPY benefits from it.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)

	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	ctx := context.Background()
	start := time.Now()

	log.Printf("==> stress seed start: products=%d params=%d formulas=%d period=%s",
		cfg.products, cfg.params, cfg.formulas, cfg.period)

	productTypes, err := loadProductTypeIDs(ctx, db)
	if err != nil {
		log.Fatalf("load product types: %v", err)
	}

	topRMCodes, err := loadTopRMCodes(ctx, db, 10)
	if err != nil {
		log.Fatalf("load top rm codes: %v", err)
	}
	if len(topRMCodes) == 0 {
		log.Printf("WARN: no rows in cst_rm_cost — ITEM RMs in routes will use synthetic codes")
		topRMCodes = []string{"STRESS_ITEM_001", "STRESS_ITEM_002", "STRESS_ITEM_003"}
	}

	rng := rand.New(rand.NewSource(42))

	paramIDs, err := seedParameters(ctx, db, cfg.params, cfg.formulas, rng)
	if err != nil {
		log.Fatalf("seed parameters: %v", err)
	}
	log.Printf("  - parameters: %d", len(paramIDs))

	if err := seedFormulas(ctx, db, paramIDs, cfg.formulas, rng); err != nil {
		log.Fatalf("seed formulas: %v", err)
	}
	log.Printf("  - formulas: %d", cfg.formulas)

	products, err := seedProducts(ctx, db, cfg.products, productTypes, rng)
	if err != nil {
		log.Fatalf("seed products: %v", err)
	}
	log.Printf("  - products: %d (FG=%d, INTER=%d)", len(products), countFG(products), len(products)-countFG(products))

	if err := seedRoutes(ctx, db, products, topRMCodes, rng); err != nil {
		log.Fatalf("seed routes: %v", err)
	}

	// CAPP/CPP cover the generated S_PARAM_* set. The stress runner isolates the
	// active formula catalog to the S_FORMULA_* set (deactivating textile-demo
	// formulas for the run), so these params are the full universe referenced.
	if err := seedCAPP(ctx, db, products, paramIDs); err != nil {
		log.Fatalf("seed capp: %v", err)
	}

	if err := seedCPP(ctx, db, products, paramIDs, rng); err != nil {
		log.Fatalf("seed cpp: %v", err)
	}

	log.Printf("==> stress seed done in %s", time.Since(start).Round(time.Second))
}

func parseFlags() config {
	cfg := config{}
	flag.IntVar(&cfg.products, "products", 12000, "number of cost_product_master rows to generate")
	flag.IntVar(&cfg.params, "params", 150, "number of mst_parameter rows to generate")
	flag.IntVar(&cfg.formulas, "formulas", 30, "number of mst_formula rows to generate")
	flag.StringVar(&cfg.period, "period", "202604", "YYYYMM period tag (informational)")
	flag.StringVar(&cfg.databaseURL, "db", envDefault("DATABASE_URL",
		"postgres://finance:finance123@localhost:5434/finance_db?sslmode=disable"),
		"Postgres connection string")
	flag.Parse()
	if cfg.products <= 0 || cfg.params <= 0 || cfg.formulas <= 0 {
		fmt.Fprintln(os.Stderr, "products, params, formulas must all be > 0")
		os.Exit(2)
	}
	return cfg
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// productRow captures the minimum we need to remember about each generated
// cost_product_master row for downstream wiring (routes, CAPP, CPP).
type productRow struct {
	sysID     int64
	code      string
	isFG      bool
	typeID    int
	productNm string
}

func countFG(ps []productRow) int {
	n := 0
	for _, p := range ps {
		if p.isFG {
			n++
		}
	}
	return n
}

// loadProductTypeIDs returns the {type_code -> type_id} map for the two types
// we care about: FG and INTER. Falls back to any active type when missing.
func loadProductTypeIDs(ctx context.Context, db *sql.DB) (map[string]int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT cpt_type_code, cpt_type_id FROM cost_product_type WHERE cpt_is_active = TRUE`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var code string
		var id int
		if err := rows.Scan(&code, &id); err != nil {
			return nil, err
		}
		out[code] = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, ok := out["FG"]; !ok {
		return nil, fmt.Errorf("cost_product_type missing FG row (run migration 000211)")
	}
	if _, ok := out["INTER"]; !ok {
		return nil, fmt.Errorf("cost_product_type missing INTER row (run migration 000211)")
	}
	return out, nil
}

// loadTopRMCodes returns the most recent priced rm_codes for ITEM-RM references
// in routes. Capped to `limit` rows for variety.
func loadTopRMCodes(ctx context.Context, db *sql.DB, limit int) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT rm_code FROM cst_rm_cost
		 WHERE rm_type = 'ITEM' AND rm_code IS NOT NULL
		 ORDER BY effective_at DESC NULLS LAST
		 LIMIT $1`, limit)
	if err != nil {
		// effective_at may not exist on older schemas — try without ordering.
		rows2, err2 := db.QueryContext(ctx,
			`SELECT rm_code FROM cst_rm_cost WHERE rm_code IS NOT NULL LIMIT $1`, limit)
		if err2 != nil {
			return nil, err
		}
		rows = rows2
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// seedParameters bulk-inserts `count` mst_parameter rows tagged with stressOwner.
// Returns the generated IDs in deterministic order, partitioned by category:
// ~70% INPUT, ~15% RATE, remainder CALCULATED. The CALCULATED partition is
// floored at minCalculated so there is always at least one distinct result
// param per formula (mst_formula.result_param_id is unique).
func seedParameters(ctx context.Context, db *sql.DB, count, minCalculated int, rng *rand.Rand) ([]paramSpec, error) {
	specs := make([]paramSpec, count)
	calcCount := count * 15 / 100
	if calcCount < minCalculated {
		calcCount = minCalculated
	}
	rateCount := count * 15 / 100
	inputUntil := count - calcCount - rateCount
	rateUntil := inputUntil + rateCount
	for i := 0; i < count; i++ {
		cat := "CALCULATED"
		if i < inputUntil {
			cat = "INPUT"
		} else if i < rateUntil {
			cat = "RATE"
		}
		specs[i] = paramSpec{
			id:       uuid.New(),
			code:     fmt.Sprintf("S_PARAM_%03d", i+1),
			name:     fmt.Sprintf("Stress Parameter %03d", i+1),
			category: cat,
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	rolled := false
	defer func() {
		if !rolled {
			return
		}
	}()

	stmt, err := tx.PrepareContext(ctx, pq.CopyIn(
		"mst_parameter",
		"id", "param_code", "param_name", "param_short_name",
		"data_type", "param_category",
		"default_value", "is_active", "created_by",
	))
	if err != nil {
		rolled = true
		_ = tx.Rollback()
		return nil, fmt.Errorf("prepare mst_parameter COPY: %w", err)
	}

	for i, s := range specs {
		def := float64(100 + rng.Intn(900))
		if _, err := stmt.ExecContext(ctx,
			s.id, s.code, s.name, fmt.Sprintf("P%03d", i+1),
			"NUMBER", s.category,
			def, true, stressOwner,
		); err != nil {
			rolled = true
			_ = stmt.Close()
			_ = tx.Rollback()
			return nil, fmt.Errorf("exec mst_parameter COPY: %w", err)
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		rolled = true
		_ = stmt.Close()
		_ = tx.Rollback()
		return nil, fmt.Errorf("flush mst_parameter COPY: %w", err)
	}
	if err := stmt.Close(); err != nil {
		rolled = true
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return specs, nil
}

type paramSpec struct {
	id       uuid.UUID
	code     string
	name     string
	category string // INPUT | RATE | CALCULATED
}

// seedFormulas writes `count` chained formulas. F1 picks 2-5 INPUT/RATE inputs
// and writes to the first CALCULATED param; F2 reads inputs + F1's output and
// writes to the second CALCULATED; and so on. Topological order is preserved
// by referencing only earlier-indexed CALCULATED params.
func seedFormulas(ctx context.Context, db *sql.DB, params []paramSpec, count int, rng *rand.Rand) error {
	// Partition param list once.
	var inputs, calculated []paramSpec
	for _, p := range params {
		switch p.category {
		case "INPUT", "RATE":
			inputs = append(inputs, p)
		case "CALCULATED":
			calculated = append(calculated, p)
		}
	}
	if len(calculated) < count {
		return fmt.Errorf("not enough CALCULATED params (%d) for %d formulas", len(calculated), count)
	}
	if len(inputs) < 5 {
		return fmt.Errorf("not enough INPUT/RATE params (%d) for stress fixture", len(inputs))
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	type formulaRow struct {
		id       uuid.UUID
		resultID uuid.UUID
		inputIDs []uuid.UUID
		code     string
		name     string
		expr     string
	}
	rows := make([]formulaRow, count)

	for i := 0; i < count; i++ {
		// 2-5 inputs from INPUTS/RATES, optionally including earlier CALCULATED.
		nInputs := 2 + rng.Intn(4) // 2..5
		pickedIDs := make([]uuid.UUID, 0, nInputs)
		exprParts := make([]string, 0, nInputs)
		seen := make(map[uuid.UUID]bool, nInputs)

		for j := 0; j < nInputs; j++ {
			var p paramSpec
			// 1-in-3 chance to reference an earlier CALCULATED (if available).
			if i > 0 && rng.Intn(3) == 0 {
				p = calculated[rng.Intn(i)]
			} else {
				p = inputs[rng.Intn(len(inputs))]
			}
			// formula_param is UNIQUE(formula_id, param_id) — skip dupes within a formula.
			if seen[p.id] {
				continue
			}
			seen[p.id] = true
			pickedIDs = append(pickedIDs, p.id)
			exprParts = append(exprParts, p.code)
		}
		expr := strings.Join(exprParts, " + ")
		rows[i] = formulaRow{
			id:       uuid.New(),
			resultID: calculated[i].id,
			inputIDs: pickedIDs,
			code:     fmt.Sprintf("S_FORMULA_%03d", i+1),
			name:     fmt.Sprintf("Stress Formula %03d", i+1),
			expr:     expr,
		}
	}

	// Write mst_formula via COPY.
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn(
		"mst_formula",
		"id", "formula_code", "formula_name", "formula_type",
		"expression", "result_param_id", "description",
		"version", "is_active", "created_by",
	))
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx,
			r.id, r.code, r.name, "CALCULATION",
			r.expr, r.resultID, "stress fixture",
			1, true, stressOwner,
		); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		_ = stmt.Close()
		_ = tx.Rollback()
		return err
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}

	// Write formula_param via COPY. Note: formula_param has no created_by column;
	// cleanup is by FK to mst_formula.
	fpStmt, err := tx.PrepareContext(ctx, pq.CopyIn(
		"formula_param", "id", "formula_id", "param_id", "sort_order",
	))
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, r := range rows {
		for idx, pid := range r.inputIDs {
			if _, err := fpStmt.ExecContext(ctx, uuid.New(), r.id, pid, idx+1); err != nil {
				_ = fpStmt.Close()
				_ = tx.Rollback()
				return err
			}
		}
	}
	if _, err := fpStmt.ExecContext(ctx); err != nil {
		_ = fpStmt.Close()
		_ = tx.Rollback()
		return err
	}
	if err := fpStmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// seedProducts inserts `count` cost_product_master rows (40% FG, 60% INTER) via
// COPY. Returns a slice of productRow with the assigned BIGSERIAL sys IDs in
// the SAME ORDER as they were inserted (re-read after COPY).
func seedProducts(
	ctx context.Context, db *sql.DB, count int,
	typeIDs map[string]int, _ *rand.Rand,
) ([]productRow, error) {
	fgID := typeIDs["FG"]
	intID := typeIDs["INTER"]

	fgCount := count * 40 / 100
	type stage struct {
		prefix string
		typeID int
		isFG   bool
		n      int
	}
	stages := []stage{
		{"STRESS_FG_", fgID, true, fgCount},
		{"STRESS_INT_", intID, false, count - fgCount},
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	stmt, err := tx.PrepareContext(ctx, pq.CopyIn(
		"cost_product_master",
		"cpm_product_code", "cpm_product_type_id", "cpm_product_name",
		"cpm_grade_code", "cpm_is_active",
		"cpm_created_by", "cpm_updated_by",
	))
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	// Track insertion order via codes so we can re-read sys IDs deterministically.
	codes := make([]string, 0, count)
	flags := make([]bool, 0, count)
	typeIDsByIdx := make([]int, 0, count)

	for _, s := range stages {
		for i := 0; i < s.n; i++ {
			code := fmt.Sprintf("%s%05d", s.prefix, i+1)
			name := fmt.Sprintf("Stress product %s", code)
			if _, err := stmt.ExecContext(ctx,
				code, s.typeID, name, "AX", true, stressOwner, stressOwner,
			); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return nil, err
			}
			codes = append(codes, code)
			flags = append(flags, s.isFG)
			typeIDsByIdx = append(typeIDsByIdx, s.typeID)
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		_ = stmt.Close()
		_ = tx.Rollback()
		return nil, err
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Re-read sys IDs in code order.
	rows, err := db.QueryContext(ctx,
		`SELECT cpm_product_code, cpm_product_sys_id
		 FROM cost_product_master
		 WHERE cpm_created_by = $1
		 ORDER BY cpm_product_sys_id`, stressOwner)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	sysByCode := make(map[string]int64, count)
	for rows.Next() {
		var c string
		var sid int64
		if err := rows.Scan(&c, &sid); err != nil {
			return nil, err
		}
		sysByCode[c] = sid
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]productRow, 0, count)
	for i, code := range codes {
		sid, ok := sysByCode[code]
		if !ok {
			return nil, fmt.Errorf("missing sys_id for %s after COPY", code)
		}
		out = append(out, productRow{
			sysID:     sid,
			code:      code,
			isFG:      flags[i],
			typeID:    typeIDsByIdx[i],
			productNm: fmt.Sprintf("Stress product %s", code),
		})
	}
	return out, nil
}

// maxStages is the global number of stage bands intermediates are partitioned
// into. Each route picks a random 10..maxStages subset of stages, deepest-first
// — modelling production textile process depth where simpler FGs traverse ~10
// intermediates and complex ones up to 20. Because every PRODUCT-RM edge goes
// from stage S to stage S-1, the GLOBAL product DAG depth is bounded at maxStages
// regardless of corpus size — yielding shallow, wide waves (thousands of
// independent FGs per wave). Sharing band members across routes is what
// produces split/merge.
const maxStages = 20

// minStages is the lowest route depth (intermediates per FG).
const minStages = 10

// pickRouteStages returns a random subset of [0, total) of size between minN
// and total (inclusive), sorted descending (deepest stage first). Used so each
// FG route walks 10..maxStages stages in deepest-first order — keeping global
// DAG depth bounded while still varying per-route complexity.
func pickRouteStages(rng *rand.Rand, total, minN int) []int {
	if minN > total {
		minN = total
	}
	count := minN + rng.Intn(total-minN+1) // minN..total inclusive
	used := make(map[int]bool, count)
	out := make([]int, 0, count)
	for len(out) < count {
		s := rng.Intn(total)
		if used[s] {
			continue
		}
		used[s] = true
		out = append(out, s)
	}
	// Sort descending so deepest stage (lowest index) emits last (= isDeepest).
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[i] < out[j] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// seedRoutes builds one cost_route_head per FG. Each route picks a random
// 10..maxStages distinct stage bands, then one intermediate per chosen band in
// deepest-first order. Total seqs per route = 1 (FG) + picked-count. Deepest
// seq carries 2-4 ITEM RMs (real priced cst_rm_cost codes); every other seq
// references the next-shallower product as a PRODUCT-RM. Total seqs capped at
// ~60_000.
func seedRoutes(ctx context.Context, db *sql.DB, products []productRow, rmCodes []string, rng *rand.Rand) error {
	// 12k FG × up to 20 stages = 240k seqs; allow up to 300k.
	const maxSeqs = 300_000

	var fgs, inters []productRow
	for _, p := range products {
		if p.isFG {
			fgs = append(fgs, p)
		} else {
			inters = append(inters, p)
		}
	}
	if len(inters) == 0 {
		return fmt.Errorf("no intermediate products generated — cannot build routes")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// 1) Insert one cost_route_head per FG. Re-read assigned head IDs by
	// (crh_product_sys_id, created_by) afterwards to wire seqs.
	headStmt, err := tx.PrepareContext(ctx, pq.CopyIn(
		"cost_route_head",
		"crh_product_sys_id", "crh_routing_status", "crh_version",
		"crh_created_by", "crh_updated_by",
	))
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, fg := range fgs {
		// COMPLETE so the calc engine's DAG builder (filters COMPLETE/LOCKED) picks them up.
		if _, err := headStmt.ExecContext(ctx, fg.sysID, "COMPLETE", 1, stressOwner, stressOwner); err != nil {
			_ = headStmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	if _, err := headStmt.ExecContext(ctx); err != nil {
		_ = headStmt.Close()
		_ = tx.Rollback()
		return err
	}
	if err := headStmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	// Re-read head IDs.
	headRows, err := db.QueryContext(ctx,
		`SELECT crh_product_sys_id, crh_head_id
		 FROM cost_route_head
		 WHERE crh_created_by = $1`, stressOwner)
	if err != nil {
		return err
	}
	headByProduct := make(map[int64]int64, len(fgs))
	for headRows.Next() {
		var pid, hid int64
		if err := headRows.Scan(&pid, &hid); err != nil {
			_ = headRows.Close()
			return err
		}
		headByProduct[pid] = hid
	}
	_ = headRows.Close()

	// 2) Build seqs in memory then COPY in one shot (capped at maxSeqs).
	type pendingSeq struct {
		headID    int64
		productID int64
		productCd string
		productNm string
		level     int
		seq       int
		isDeepest bool
	}
	// Partition intermediates into maxStages contiguous bands by index. band(s)
	// returns the slice for stage s (0 = deepest). A product's band is fixed, so
	// every PRODUCT-RM edge moves exactly one band shallower and the global DAG
	// depth stays bounded at maxStages.
	bandSize := len(inters) / maxStages
	if bandSize < 1 {
		bandSize = 1
	}
	band := func(stage int) []productRow {
		lo := stage * bandSize
		hi := lo + bandSize
		if stage == maxStages-1 || hi > len(inters) {
			hi = len(inters)
		}
		if lo >= len(inters) {
			lo = len(inters) - 1
		}
		return inters[lo:hi]
	}

	totalEstimated := 0
	var pending []pendingSeq
	for _, fg := range fgs {
		hid := headByProduct[fg.sysID]
		// Level 1: FG seq.
		pending = append(pending, pendingSeq{
			headID: hid, productID: fg.sysID, productCd: fg.code, productNm: fg.productNm,
			level: 1, seq: 1, isDeepest: false,
		})
		// Pick a random 10..maxStages distinct stage indices for THIS route, then
		// emit one intermediate per chosen stage in deepest-first order. Variable
		// depth per route models the real mix of simple/complex FGs.
		picked := pickRouteStages(rng, maxStages, minStages)
		level := 2
		for i, stage := range picked {
			b := band(stage)
			ip := b[rng.Intn(len(b))]
			pending = append(pending, pendingSeq{
				headID: hid, productID: ip.sysID, productCd: ip.code, productNm: ip.productNm,
				level: level, seq: 1, isDeepest: i == len(picked)-1,
			})
			level++
			totalEstimated++
			if len(pending) >= maxSeqs {
				break
			}
		}
		if len(pending) >= maxSeqs {
			break
		}
	}

	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	seqStmt, err := tx2.PrepareContext(ctx, pq.CopyIn(
		"cost_route_seq",
		"crs_head_id", "crs_product_sys_id", "crs_route_level", "crs_route_seq",
		"crs_route_name", "crs_created_by", "crs_updated_by",
	))
	if err != nil {
		_ = tx2.Rollback()
		return err
	}
	for _, p := range pending {
		if _, err := seqStmt.ExecContext(ctx,
			p.headID, p.productID, p.level, p.seq,
			p.productNm, stressOwner, stressOwner,
		); err != nil {
			_ = seqStmt.Close()
			_ = tx2.Rollback()
			return err
		}
	}
	if _, err := seqStmt.ExecContext(ctx); err != nil {
		_ = seqStmt.Close()
		_ = tx2.Rollback()
		return err
	}
	if err := seqStmt.Close(); err != nil {
		_ = tx2.Rollback()
		return err
	}
	if err := tx2.Commit(); err != nil {
		return err
	}
	log.Printf("  - route seqs: %d", len(pending))

	// 3) Re-read seq IDs grouped by head, level so we can wire route_rm rows.
	rows3, err := db.QueryContext(ctx,
		`SELECT crs_seq_id, crs_head_id, crs_product_sys_id, crs_route_level
		 FROM cost_route_seq
		 WHERE crs_created_by = $1
		 ORDER BY crs_head_id, crs_route_level`, stressOwner)
	if err != nil {
		return err
	}
	type seqRow struct {
		seqID, headID, productID int64
		level                    int
	}
	byHead := map[int64][]seqRow{}
	for rows3.Next() {
		var s seqRow
		if err := rows3.Scan(&s.seqID, &s.headID, &s.productID, &s.level); err != nil {
			_ = rows3.Close()
			return err
		}
		byHead[s.headID] = append(byHead[s.headID], s)
	}
	_ = rows3.Close()

	// 4) cost_route_rm via COPY.
	tx3, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rmStmt, err := tx3.PrepareContext(ctx, pq.CopyIn(
		"cost_route_rm",
		"crm_seq_id", "crm_parent_product_sys_id",
		"crm_rm_product_sys_id", "crm_rm_item_code", "crm_rm_type",
		"crm_route_rm_name", "crm_route_rm_ratio",
		"crm_created_by", "crm_updated_by",
	))
	if err != nil {
		_ = tx3.Rollback()
		return err
	}
	rmCount := 0
	for _, seqs := range byHead {
		// Within each head, seqs already sorted by level (1, 2, 3, ...).
		// FG (level=1) -> 1 PRODUCT-RM pointing at level=2 product.
		// Intermediate (levels 2..max-1) -> 1 PRODUCT-RM pointing at next level.
		// Deepest (level=max) -> 2-4 ITEM RMs from rmCodes.
		for i, s := range seqs {
			if i < len(seqs)-1 {
				up := seqs[i+1]
				if _, err := rmStmt.ExecContext(ctx,
					s.seqID, s.productID,
					up.productID, nil, "PRODUCT",
					nil, 1.0, stressOwner, stressOwner,
				); err != nil {
					_ = rmStmt.Close()
					_ = tx3.Rollback()
					return err
				}
				rmCount++
				continue
			}
			// Deepest seq: 2-4 ITEM RMs.
			n := 2 + rng.Intn(3)
			for j := 0; j < n; j++ {
				code := rmCodes[rng.Intn(len(rmCodes))]
				if _, err := rmStmt.ExecContext(ctx,
					s.seqID, s.productID,
					nil, code, "ITEM",
					code, 0.25+float64(rng.Intn(75))/100.0,
					stressOwner, stressOwner,
				); err != nil {
					_ = rmStmt.Close()
					_ = tx3.Rollback()
					return err
				}
				rmCount++
			}
		}
	}
	if _, err := rmStmt.ExecContext(ctx); err != nil {
		_ = rmStmt.Close()
		_ = tx3.Rollback()
		return err
	}
	if err := rmStmt.Close(); err != nil {
		_ = tx3.Rollback()
		return err
	}
	if err := tx3.Commit(); err != nil {
		return err
	}
	log.Printf("  - route rms: %d", rmCount)
	return nil
}

// seedCAPP writes one cost_product_applicable_param row per (product, param)
// pair. For PRODUCTS=12000, PARAMS=150 this is 1.8M rows — bulk COPY in chunks.
func seedCAPP(ctx context.Context, db *sql.DB, products []productRow, params []paramSpec) error {
	const chunkSize = 200_000
	totalPairs := len(products) * len(params)
	log.Printf("  - capp: writing %d pairs in chunks of %d", totalPairs, chunkSize)

	written := 0
	for start := 0; start < len(products); {
		// Determine how many products fit into one chunk by pair count.
		productsPerChunk := chunkSize / max(1, len(params))
		if productsPerChunk < 1 {
			productsPerChunk = 1
		}
		end := start + productsPerChunk
		if end > len(products) {
			end = len(products)
		}
		if err := copyCAPPChunk(ctx, db, products[start:end], params); err != nil {
			return err
		}
		written += (end - start) * len(params)
		log.Printf("    capp progress: %d / %d", written, totalPairs)
		start = end
	}
	return nil
}

func copyCAPPChunk(ctx context.Context, db *sql.DB, products []productRow, params []paramSpec) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn(
		"cost_product_applicable_param",
		"capp_product_sys_id", "capp_param_id",
		"capp_is_required", "capp_created_by",
	))
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, p := range products {
		for _, pa := range params {
			if _, err := stmt.ExecContext(ctx, p.sysID, pa.id, false, stressOwner); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return err
			}
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		_ = stmt.Close()
		_ = tx.Rollback()
		return err
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// seedCPP writes one cost_product_parameter row per (product, param) pair for
// NUMERIC params. CALCULATED params get NULL (engine populates).
func seedCPP(ctx context.Context, db *sql.DB, products []productRow, params []paramSpec, rng *rand.Rand) error {
	const chunkSize = 200_000
	// Only INPUT and RATE rows actually receive a stored value.
	var stored []paramSpec
	for _, p := range params {
		if p.category != "CALCULATED" {
			stored = append(stored, p)
		}
	}
	totalPairs := len(products) * len(stored)
	log.Printf("  - cpp: writing %d numeric pairs in chunks of %d", totalPairs, chunkSize)

	written := 0
	for start := 0; start < len(products); {
		productsPerChunk := chunkSize / max(1, len(stored))
		if productsPerChunk < 1 {
			productsPerChunk = 1
		}
		end := start + productsPerChunk
		if end > len(products) {
			end = len(products)
		}
		if err := copyCPPChunk(ctx, db, products[start:end], stored, rng); err != nil {
			return err
		}
		written += (end - start) * len(stored)
		log.Printf("    cpp progress: %d / %d", written, totalPairs)
		start = end
	}
	return nil
}

func copyCPPChunk(ctx context.Context, db *sql.DB, products []productRow, params []paramSpec, rng *rand.Rand) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn(
		"cost_product_parameter",
		"cpp_product_sys_id", "cpp_param_id",
		"cpp_value_numeric", "cpp_value_text", "cpp_value_flag",
		"cpp_filled_by", "cpp_created_by",
	))
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, p := range products {
		for _, pa := range params {
			v := 1.0 + float64(rng.Intn(49_999))
			if _, err := stmt.ExecContext(ctx,
				p.sysID, pa.id,
				v, nil, nil,
				stressOwner, stressOwner,
			); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return err
			}
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		_ = stmt.Close()
		_ = tx.Rollback()
		return err
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
