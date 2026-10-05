// E2E lifecycle test for the ERP cost integration batch (plan-06 P5-T7a):
// DRAFT -> DEMAND_LOADED -> COVERED -> DERIVED -> VALIDATED on a LOCAL
// PostgreSQL (internal/testutil/pgcontainer) with fake Oracle readers, then a
// re-run on a fresh batch asserting identical hashes and totals. No Oracle,
// push/valuate/recon/approve/lock (T7b) against oracle.FakeWriter and in-test
// Oracle READ fakes. No real Oracle. Skipped unless E2E_TEST=true.
package e2e

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

const erpLifecyclePeriod = "202608"

type erpFakeDemandReader struct{ rows []domain.ErpDemandRow }

func (r erpFakeDemandReader) LoadAdjDemand(context.Context, string) ([]domain.ErpDemandRow, error) {
	return r.rows, nil
}

// erpFakeProber reports posted ADJ heads only after the approve step.
type erpFakeProber struct{ posted *int64 }

func (erpFakeProber) ProbeHeads(context.Context, string) (domain.AdjHeadCounts, error) {
	return domain.AdjHeadCounts{Heads: 2, Eligible: 2}, nil
}

func (p erpFakeProber) ProbePosted(context.Context, string) (int64, error) {
	if p.posted == nil {
		return 0, nil
	}
	return *p.posted, nil
}

// erpFakeSnapReader is a stateful Oracle ADJ snapshot: valuate stamps the
// batch id and rate, approve sets the head approval status.
type erpFakeSnapReader struct{ rows []domain.AdjSnapshotRow }

func (r *erpFakeSnapReader) SnapshotAdjRows(context.Context, string) ([]domain.AdjSnapshotRow, error) {
	return append([]domain.AdjSnapshotRow(nil), r.rows...), nil
}

func (r *erpFakeSnapReader) valuate(batchID int64, std []domain.StdRow) {
	byKey := domain.StdRowsByKey(std)
	id := strconv.FormatInt(batchID, 10)
	for i := range r.rows {
		if s, ok := byKey[r.rows[i].Key()]; ok {
			r.rows[i].Rate = s.StdCost
		}
		r.rows[i].Flex[12] = &id
	}
}

func (r *erpFakeSnapReader) approve() {
	three := int64(3)
	for i := range r.rows {
		r.rows[i].HeadApprStatus = &three
	}
}

// erpFakeReconReader serves the read-back derived from the PG std rows.
type erpFakeReconReader struct {
	batch domain.OracleBatchReadBack
	adj   []domain.AdjReadBackCombo
}

func (r erpFakeReconReader) ReadBackBatch(context.Context, int64) (domain.OracleBatchReadBack, error) {
	return r.batch, nil
}

func (r erpFakeReconReader) ReadBackAdj(context.Context, string, int64) ([]domain.AdjReadBackCombo, error) {
	return r.adj, nil
}

// erpLifecycleSummaries is the comparable output of one full flow.
type erpLifecycleSummaries struct {
	load     app.LoadDemandSummary
	coverage app.CoverageSummary
	derive   app.DeriveSummary
	validate app.ValidateSummary
	push     app.PushSummary
	recon    app.ReconSummary
	lock     app.LockBatchSummary
}

func erpSeedProduct(ctx context.Context, t *testing.T, raw *sql.DB, item, shade string) {
	t.Helper()
	var typeID int32
	require.NoError(t, raw.QueryRowContext(ctx, `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name) VALUES ('ITY', 'ITY')
		ON CONFLICT (cpt_type_code) DO UPDATE SET cpt_type_name = cost_product_type.cpt_type_name
		RETURNING cpt_type_id`).Scan(&typeID))
	var pid, hid int64
	require.NoError(t, raw.QueryRowContext(ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name,
			cpm_shade_code, cpm_grade_code, cpm_is_active, cpm_erp_item_code,
			cpm_erp_grade_code_1, cpm_erp_grade_code_2, cpm_created_by, cpm_updated_by)
		VALUES (generate_cost_product_code($1, NOW()), $1, 'E2E product', $2, 'AX', TRUE, $3,
			'KEEP-G1', 'KEEP-G2', 'e2e', 'e2e')
		RETURNING cpm_product_sys_id`, typeID, shade, item).Scan(&pid))
	require.NoError(t, raw.QueryRowContext(ctx, `INSERT INTO cost_route_head (crh_product_sys_id, crh_created_by)
		VALUES ($1, 'e2e') RETURNING crh_head_id`, pid).Scan(&hid))
	_, err := raw.ExecContext(ctx, `UPDATE cost_product_master SET cpm_erp_fg_type = 'Type 1',
		cpm_erp_item_type = 'YARN' WHERE cpm_product_sys_id = $1`, pid)
	require.NoError(t, err)
	_, err = raw.ExecContext(ctx, `INSERT INTO cst_product_cost
		(cpc_product_sys_id, cpc_period, cpc_calculation_type, cpc_route_head_id, cpc_cost_per_unit, cpc_total_rm_cost,
		 cpc_calculated_by, cpc_currency_code, cpc_status, cpc_verified_at, cpc_verified_by, cpc_approved_at, cpc_approved_by)
		VALUES ($1, $2, 'ACTUAL', $3, 2.345600, 2.345600, 'e2e', 'USD', 'APPROVED', NOW(), 'v', NOW(), 'a')`,
		pid, erpLifecyclePeriod, hid)
	require.NoError(t, err)
}

// erpSteps bundles the DRAFT -> VALIDATED step handlers; the fake lifecycle
// and the user-run RO DEV test share it and differ only in reader and prober.
type erpSteps struct {
	batches  *postgres.ErpIntBatchRepository
	create   *app.CreateBatchHandler
	load     *app.LoadDemandStep
	cov      *app.CoverageStep
	derive   *app.DeriveStep
	validate *app.ValidateStep
}

func newErpSteps(db *postgres.DB, reader domain.ErpDemandReader, prober domain.ErpAdjHeadProber) erpSteps {
	runner := postgres.NewErpBatchTxRunner(db)
	maxAge := time.Hour
	batches := postgres.NewErpIntBatchRepository(db)
	return erpSteps{
		batches:  batches,
		create:   app.NewCreateBatchHandler(batches, postgres.NewPeriodLockRepository(db), prober),
		load:     app.NewLoadDemandStep(runner, reader, prober),
		cov:      app.NewCoverageStep(runner, postgres.NewErpCoverageSourceRepository(db), maxAge),
		derive:   app.NewDeriveStep(runner, postgres.NewErpRuleSetLoader(db), maxAge),
		validate: app.NewValidateStep(runner, prober, maxAge),
	}
}

// runErpLifecycle seeds a fresh migrated database and runs the whole
// DRAFT -> VALIDATED flow on it. A second LIVE batch per period is refused by
// the service, so the re-run uses a second database on the same container.
func runErpLifecycle(ctx context.Context, t *testing.T, srv *pgcontainer.Server, dbName string) erpLifecycleSummaries {
	t.Helper()
	raw := srv.CreateDatabase(t, dbName, "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))
	db := postgres.NewDBFromSQL(raw)

	_, err = raw.ExecContext(ctx, `INSERT INTO cst_period_lock (cpl_period, cpl_calc_type, cpl_locked_at, cpl_locked_by, cpl_reason)
		VALUES ($1, 'ACTUAL', NOW(), 'e2e', 'e2e lifecycle')`, erpLifecyclePeriod)
	require.NoError(t, err)
	// Grade -> group assignments come from the 000548 seed table; the master
	// sync (not run here) would copy them onto cost_erp_grade.
	_, err = raw.ExecContext(ctx, `INSERT INTO cost_erp_grade (ceg_grade_code, ceg_grade_name, ceg_grade_group)
		SELECT cggs_grade_code, cggs_grade_code, cggs_grade_group FROM cst_erp_grade_group_seed
		WHERE cggs_grade_code IN ('APQ', 'B1')
		ON CONFLICT (ceg_grade_code) DO UPDATE SET ceg_grade_group = EXCLUDED.ceg_grade_group`)
	require.NoError(t, err)
	// ERP replica masters (V-06): the items and shades of the demand plus AX.
	for _, q := range []string{
		`INSERT INTO cost_erp_item (cei_item_code, cei_item_name) VALUES ('POY0000275', 'POY 275'), ('POY0000300', 'POY 300')`,
		`INSERT INTO cost_erp_shade (ces_shade_code, ces_shade_name) VALUES ('X419T', 'X419T'), ('D1', 'D1')`,
		`INSERT INTO cost_erp_grade (ceg_grade_code, ceg_grade_name) VALUES ('AX', 'AX') ON CONFLICT (ceg_grade_code) DO NOTHING`,
	} {
		_, err = raw.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	erpSeedProduct(ctx, t, raw, "POY0000275", "X419T")
	erpSeedProduct(ctx, t, raw, "POY0000300", "D1")

	row := func(item, grade, shade, qty string) domain.ErpDemandRow {
		return domain.ErpDemandRow{Period: erpLifecyclePeriod, TxnCode: domain.TxnInvAdj, ItemCode: item, GradeCode: grade,
			ShadeCode: shade, HeadCount: 1, ItemCount: 1, RateVariants: 1, QtyKg: decimal.RequireFromString(qty)}
	}
	reader := erpFakeDemandReader{rows: []domain.ErpDemandRow{
		row("POY0000275", "APQ", "X419T", "10"), row("POY0000275", "B1", "X419T", "5.5"), row("POY0000300", "APQ", "D1", "3"),
	}}
	var posted int64
	prober := erpFakeProber{posted: &posted}
	runner := postgres.NewErpBatchTxRunner(db)
	steps := newErpSteps(db, reader, prober)
	batches := steps.batches
	create, load, cov, derive, validate := steps.create, steps.load, steps.cov, steps.derive, steps.validate

	status := func(id int64) domain.BatchStatus {
		b, gerr := batches.GetByID(ctx, id)
		require.NoError(t, gerr)
		return b.Status()
	}

	var s erpLifecycleSummaries
	b, cerr := create.Handle(ctx, app.CreateBatchCommand{Period: erpLifecyclePeriod, Mode: domain.ModeLive, Actor: "e2e"})
	require.NoError(t, cerr)
	id := b.ID()
	assert.Equal(t, domain.StatusDraft, status(id))

	s.load, err = load.Run(ctx, id, "e2e", nil)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDemandLoaded, status(id))

	s.coverage, err = cov.Run(ctx, id, "e2e", nil)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCovered, status(id))

	s.derive, err = derive.Run(ctx, id, "e2e", nil)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDerived, status(id))

	s.validate, err = validate.Run(ctx, id, "e2e", nil)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusValidated, status(id))

	// --- T7b: PUSHED -> VALUATED -> RECONCILED -> (approve) -> LOCKED ---
	fw := oracle.NewFakeWriter()
	gate := app.WriterGate{Writer: fw, Mode: domain.WriterModeFake}
	callRepo := postgres.NewErpOracleCallRepository(db)
	audit := auditapp.NewEmitter(postgres.NewCostAuditLogRepository(db))
	previews := postgres.NewErpValuationPreviewRepository(db)
	stdRepo := postgres.NewErpStdCostRepository(db)
	stdRows, err := stdRepo.List(ctx, id)
	require.NoError(t, err)
	digest, err := domain.ComputeStdDigest(stdRows)
	require.NoError(t, err)

	snap := &erpFakeSnapReader{}
	for i, sr := range stdRows {
		snap.rows = append(snap.rows, domain.AdjSnapshotRow{
			HeadSysID: int64(100 + i), ItemSysID: int64(1000 + i), TxnCode: domain.TxnInvAdj,
			ItemCode: sr.Key.ItemCode, GradeCode: sr.Key.GradeCode, ShadeCode: sr.Key.ShadeCode,
			QtyBu: decimal.NewNullDecimal(decimal.NewFromInt(1000)),
			Rate:  decimal.NewNullDecimal(decimal.NewFromInt(1)), Val: decimal.NewNullDecimal(decimal.NewFromInt(1)),
		})
	}

	push := app.NewPushStep(runner, gate, callRepo, audit, true, 30*time.Second)
	rowCount, sumStd := s.derive.RowCount, s.derive.SumStd
	s.push, err = push.Run(ctx, app.PushCommand{BatchID: id, Actor: "e2e", JobID: uuid.NewString(), HasPermission: true,
		ConfirmRowCount: &rowCount, ConfirmSumStd: sumStd}, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPushed, status(id))

	pvh := app.NewValuationPreviewHandler(batches, postgres.NewPeriodLockRepository(db), snap, stdRepo, previews,
		app.ValuationPreviewConfig{ValuationEnabled: true, WriterMode: domain.WriterModeFake, TTL: 30 * time.Minute})
	adj := app.NewAdjExecuteStep(runner, gate, callRepo, audit, previews, previews, snap,
		app.AdjExecuteConfig{ValuationEnabled: true, AdjApproveEnabled: true, CallTimeout: 30 * time.Second, BusyRetries: 3})
	pv, err := pvh.Handle(ctx, app.ValuationPreviewCommand{BatchID: id, Actor: "e2e", HasPermission: true})
	require.NoError(t, err)
	require.NotZero(t, pv.Totals.ProjectedItems, "preview must project items")
	_, err = adj.Run(ctx, app.AdjExecuteCommand{BatchID: id, PreviewID: pv.Preview.ID, Operation: domain.AdjOpValuate,
		ConfirmSetHash: pv.Preview.SetHash, ConfirmText: pv.ConfirmText, Actor: "e2e", JobID: uuid.NewString(), HasPermission: true}, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusValuated, status(id))
	snap.valuate(id, stdRows)

	rr := erpFakeReconReader{batch: domain.OracleBatchReadBack{
		Found: true, BatchID: id, Period: erpLifecyclePeriod, Seq: s.push.Seq, Status: domain.GsbValuated,
		RuleHash: s.derive.RuleHash, Header: digest.Totals, CostRows: stdRows,
	}}
	for _, sr := range stdRows {
		rr.adj = append(rr.adj, domain.AdjReadBackCombo{Key: sr.Key, Items: 1, RateVariants: 1, MaxRate: sr.StdCost,
			QtyKg: decimal.NewNullDecimal(decimal.NewFromInt(1)), Value: sr.StdCost,
			Flex13: strconv.FormatInt(id, 10), Stamped: 1})
	}
	s.recon, err = app.NewReconStep(runner, rr, callRepo).Run(ctx, id, "e2e", nil)
	require.NoError(t, err)
	require.True(t, s.recon.TotalsEqual && s.recon.MD5Equal && s.recon.RuleHashOK, "recon must match: %+v", s.recon)
	assert.Equal(t, domain.StatusReconciled, status(id))

	// APPROVE preview over the batch-stamped set (RECONCILED), built by the handler.
	apvRes, err := pvh.Handle(ctx, app.ValuationPreviewCommand{BatchID: id, Actor: "e2e", Operation: domain.AdjOpApprove, HasPermission: true})
	require.NoError(t, err)
	apv := apvRes.Preview
	_, err = adj.Run(ctx, app.AdjExecuteCommand{BatchID: id, PreviewID: apv.ID, Operation: domain.AdjOpApprove,
		ConfirmSetHash: apv.SetHash, ConfirmText: apv.ConfirmText, Actor: "e2e", JobID: uuid.NewString(), HasPermission: true}, nil)
	require.NoError(t, err)
	snap.approve()
	posted = int64(len(snap.rows))

	lock := app.NewLockBatchStep(runner, gate, callRepo, audit, prober, true, 30*time.Second, 3)
	s.lock, err = lock.Run(ctx, app.LockBatchCommand{BatchID: id, Actor: "e2e", JobID: uuid.NewString(), HasPermission: true}, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusLocked, status(id))
	fs, ok := fw.Status(id)
	require.True(t, ok)
	assert.Equal(t, "LOCKED", string(fs))
	return s
}

func TestErpIntegrationLifecycle(t *testing.T) {
	if os.Getenv("E2E_TEST") != "true" {
		t.Skip("Skipping E2E test. Set E2E_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)

	first := runErpLifecycle(ctx, t, srv, "erp_lifecycle_e2e_1")
	assert.Equal(t, "18.5", first.load.QtyKg)
	assert.Zero(t, first.validate.Errors)
	assert.True(t, first.validate.Validated)

	second := runErpLifecycle(ctx, t, srv, "erp_lifecycle_e2e_2")
	assert.Equal(t, first.load.Digest, second.load.Digest, "load digest")
	assert.Equal(t, first.load.QtyKg, second.load.QtyKg)
	assert.Equal(t, first.coverage.Digest, second.coverage.Digest, "coverage digest")
	assert.Equal(t, first.coverage.Counts, second.coverage.Counts)
	assert.Equal(t, first.derive.RuleHash, second.derive.RuleHash, "rule hash")
	assert.Equal(t, first.derive.RowsMD5, second.derive.RowsMD5, "rows md5")
	assert.Equal(t, first.derive.SumStd, second.derive.SumStd)
	assert.Equal(t, first.derive.SumConv, second.derive.SumConv)
	assert.Equal(t, first.derive.SumPvl, second.derive.SumPvl)
	assert.Equal(t, first.derive.RowCount, second.derive.RowCount)
	assert.Equal(t, first.validate.WarningSetHash, second.validate.WarningSetHash)
	assert.Equal(t, first.validate.ByCode, second.validate.ByCode)
	assert.Equal(t, first.push.RowsMD5, second.push.RowsMD5, "push rows md5")
	assert.Equal(t, first.push.SumStd, second.push.SumStd)
	assert.Equal(t, first.push.RowCount, second.push.RowCount)
	assert.Equal(t, first.recon.PgRowsMD5, second.recon.PgRowsMD5, "recon md5")
	assert.Equal(t, first.recon.PgRowCount, second.recon.PgRowCount)
	assert.Equal(t, first.recon.OraRowCount, second.recon.OraRowCount)
	assert.Equal(t, first.recon.Counts, second.recon.Counts)
}

// erpRecordingQuerier forwards to the guarded querier and records each
// statement so the RO test can assert every one passed oracle.CheckReadOnly.
type erpRecordingQuerier struct {
	inner oracle.ReadOnlyQuerier
	stmts []string
}

func (r *erpRecordingQuerier) QueryRO(ctx context.Context, query string, args ...any) (oracle.Rows, error) {
	r.stmts = append(r.stmts, query)
	return r.inner.QueryRO(ctx, query, args...)
}

// TestErpIntegrationLifecycle_RODev is the USER-RUN read-only DEV variant (P5-T7c).
//
// RUNBOOK (user only; sub-agents and CI must NEVER run this against Oracle):
//
//  1. Use ONLY the existing READ-ONLY ALTHARADEV (DEV) user. Never a
//     write-capable, production or staging account.
//
//  2. From goapps-backend/services/finance run (PG comes from a local
//     pgcontainer, Oracle is read through oracle.NewReadOnlyDB only):
//
//     DOCKER_HOST=unix://$HOME/.colima/default/docker.sock \
//     TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
//     TESTCONTAINERS_RYUK_DISABLED=true \
//     E2E_TEST=true ORACLE_RO_TEST=true \
//     ORACLE_RO_DSN='oracle://<ro_user>:<password>@<dev_host>:<port>/<service>' \
//     go test ./tests/e2e/ -run TestErpIntegrationLifecycle_RODev -count=1 -v -timeout 240s
//
//     Optional: ORACLE_RO_PERIOD=YYYYMM (default 202608).
//
//  3. Paste the full -v output (the "RO ..." log lines) into the ledger.
//
// What it does: real Oracle demand reader (demand_source=view, appEnv
// "development") and ADJ head prober, empty migrated PG, flow DRAFT ->
// VALIDATED. It never builds a writer. Coverage will show unmatched rows
// (PG master data is empty); a refusing step stops the test at the last
// reachable status. Without both env flags it skips and opens no connection.
func TestErpIntegrationLifecycle_RODev(t *testing.T) {
	if os.Getenv("E2E_TEST") != "true" {
		t.Skip("Skipping E2E test. Set E2E_TEST=true to run.")
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("ORACLE_RO_TEST")), "true") {
		t.Skip("ORACLE_RO_TEST!=true: user-run read-only DEV test skipped (P5-T7c)")
	}
	dsn := strings.TrimSpace(os.Getenv("ORACLE_RO_DSN"))
	require.NotEmpty(t, dsn, "ORACLE_RO_TEST=true but ORACLE_RO_DSN is empty; set the READ-ONLY DEV DSN")
	period := erpLifecyclePeriod
	if p := strings.TrimSpace(os.Getenv("ORACLE_RO_PERIOD")); p != "" {
		period = p
	}

	odb, err := sql.Open("oracle", dsn)
	require.NoError(t, err)
	odb.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = odb.Close() })
	pctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, odb.PingContext(pctx))
	rq := &erpRecordingQuerier{inner: oracle.NewReadOnlyDB(odb)}
	defer func() {
		require.NotEmpty(t, rq.stmts, "no Oracle statements recorded")
		for _, s := range rq.stmts {
			assert.NoError(t, oracle.CheckReadOnly(s), "statement not read-only")
		}
		t.Logf("RO oracle statements: %d (all passed CheckReadOnly)", len(rq.stmts))
	}()

	reader, err := oracle.NewErpDemandReader(rq, "view", "development")
	require.NoError(t, err)
	prober := oracle.NewErpAdjReader(rq)

	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "erp_lifecycle_rodev", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))
	db := postgres.NewDBFromSQL(raw)
	_, err = raw.ExecContext(ctx, `INSERT INTO cst_period_lock (cpl_period, cpl_calc_type, cpl_locked_at, cpl_locked_by, cpl_reason)
		VALUES ($1, 'ACTUAL', NOW(), 'e2e', 'e2e RO DEV')`, period)
	require.NoError(t, err)

	st := newErpSteps(db, reader, prober)
	status := func(id int64) domain.BatchStatus {
		b, gerr := st.batches.GetByID(ctx, id)
		require.NoError(t, gerr)
		return b.Status()
	}
	b, err := st.create.Handle(ctx, app.CreateBatchCommand{Period: period, Mode: domain.ModeLive, Actor: "e2e"})
	require.NoError(t, err)
	id := b.ID()

	ld, err := st.load.Run(ctx, id, "e2e", nil)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDemandLoaded, status(id))
	t.Logf("RO load: period=%s rows=%d items=%d qtyKg=%s digest=%s", period, ld.Rows, ld.Items, ld.QtyKg, ld.Digest)

	cv, err := st.cov.Run(ctx, id, "e2e", nil)
	if err != nil {
		t.Logf("RO stopped at %s: coverage refused: %v", status(id), err)
		return
	}
	assert.Equal(t, domain.StatusCovered, status(id))
	t.Logf("RO coverage: counts=%+v digest=%s", cv.Counts, cv.Digest)

	dv, err := st.derive.Run(ctx, id, "e2e", nil)
	if err != nil {
		t.Logf("RO stopped at %s: derive refused: %v", status(id), err)
		return
	}
	assert.Equal(t, domain.StatusDerived, status(id))
	t.Logf("RO derive: rows=%d sumStd=%s", dv.RowCount, dv.SumStd)

	vs, err := st.validate.Run(ctx, id, "e2e", nil)
	if err != nil {
		t.Logf("RO stopped at %s: validate refused: %v", status(id), err)
		return
	}
	t.Logf("RO validate: status=%s errors=%d validated=%v byCode=%+v", status(id), vs.Errors, vs.Validated, vs.ByCode)
}
