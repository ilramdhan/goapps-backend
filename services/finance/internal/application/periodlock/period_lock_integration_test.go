// AC-09 period-lock integration suite (plan-02 P1-T6). LOCAL PostgreSQL
// only, via internal/testutil/pgcontainer (a throwaway postgres:16-alpine
// container, or a LOCAL TEST_DATABASE_URL). Migrations 000001..latest are
// applied to a fresh database.
//
// One scenario over the real handlers + repositories, wired as in
// cmd/server/main.go:
//
//  1. lock 202608 ACTUAL;
//  2. trigger, verify, approve, MB recompute (MB_BATCH), MB push and supersede
//     are each refused with ErrPeriodLocked and write nothing;
//  3. FORECAST 202608 still triggers (and supersedes);
//  4. unlock with a reason (unlock without one, or of a posted period, is refused);
//  5. trigger, verify, approve, MB batch, MB push and supersede succeed again.
//
// Plus the concurrent supersede-vs-lock race, in both orders, for a period
// that has never had a lock row.
//
// Skipped unless INTEGRATION_TEST=true.
package periodlock_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbbatch"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbpush"
	plapp "github.com/mutugading/goapps-backend/services/finance/internal/application/periodlock"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

const ac09Period = "202608"

// recordingPublisher stands in for RMQ: it records the job ids it was asked to
// publish, so "refused" can also mean "nothing published".
type recordingPublisher struct {
	mu  sync.Mutex
	ids []int64
}

func (p *recordingPublisher) PublishJobTriggered(_ context.Context, id int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ids = append(p.ids, id)
	return nil
}

func (p *recordingPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.ids)
}

type ac09Env struct {
	raw       *sql.DB
	results   *postgres.CostResultRepository
	locks     *postgres.PeriodLockRepository
	pub       *recordingPublisher
	trigger   *costcalc.TriggerJobHandler
	verify    *costcalc.VerifyCostHandler
	approve   *costcalc.ApproveCostHandler
	mbTrigger *mbbatch.TriggerHandler
	mbPush    *mbpush.ExecuteHandler
	lockH     *plapp.LockHandler
	emitter   *auditapp.Emitter
}

func newAC09Env(ctx context.Context, t *testing.T, dbName string) *ac09Env {
	t.Helper()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, dbName, "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	db := postgres.NewDBFromSQL(raw)
	jobRepo := postgres.NewCostCalcJobRepository(db)
	results := postgres.NewCostResultRepository(db)
	auditHist := postgres.NewCostAuditHistoryRepository(db)
	locks := postgres.NewPeriodLockRepository(db)
	emitter := auditapp.NewEmitter(postgres.NewCostAuditLogRepository(db))
	loader := costcalc.NewProductLoader(raw)
	cache := evaluator.NewCache()
	pub := &recordingPublisher{}

	svc := costcalc.NewService(
		jobRepo, postgres.NewCostCalcChunkRepository(db), postgres.NewCostCalcJobProductRepository(db),
		results, auditHist, loader, cache, nil, pub,
	)
	mbComp := postgres.NewMBCompositionRepository(db)
	mbHead := postgres.NewMBHeadRepository(db, mbComp)
	mbSvc := mbbatch.NewService(db,
		mbbatch.NewMBHeadReaderAdapter(mbHead), mbbatch.NewMBEdgeReaderAdapter(mbComp),
		mbbatch.NewResultWriterAdapter(results), loader, cache, auditHist, jobRepo)

	return &ac09Env{
		raw: raw, results: results, locks: locks, pub: pub, emitter: emitter,
		trigger:   costcalc.NewTriggerJobHandler(svc, costcalc.WithTriggerPeriodLock(locks)),
		verify:    costcalc.NewVerifyCostHandler(svc, costcalc.WithVerifyPeriodLock(locks)),
		approve:   costcalc.NewApproveCostHandler(svc, costcalc.WithApprovePeriodLock(locks)),
		mbTrigger: mbbatch.NewTriggerHandler(mbSvc, jobRepo, mbbatch.WithPeriodLock(locks)),
		mbPush: mbpush.NewExecuteHandler(db, mbpush.NewMBHeadReaderAdapter(mbHead),
			mbpush.NewCostReaderAdapter(results), postgres.NewCstMBCostRepository(db),
			postgres.NewMBPushLogRepository(db), mbpush.WithPeriodLock(locks)),
		lockH: plapp.NewLockHandler(locks, emitter),
	}
}

func (e *ac09Env) count(ctx context.Context, t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, e.raw.QueryRowContext(ctx, q, args...).Scan(&n))
	return n
}

func (e *ac09Env) jobs(ctx context.Context, t *testing.T, calcType, scope string) int {
	return e.count(ctx, t,
		`SELECT count(*) FROM cal_job WHERE cj_period=$1 AND cj_calculation_type=$2 AND cj_scope=$3`,
		ac09Period, calcType, scope)
}

func (e *ac09Env) status(ctx context.Context, t *testing.T, costID int64) string {
	t.Helper()
	var s string
	require.NoError(t, e.raw.QueryRowContext(ctx,
		`SELECT cpc_status FROM cst_product_cost WHERE cpc_cost_id=$1`, costID).Scan(&s))
	return s
}

func (e *ac09Env) activeTotal(ctx context.Context, t *testing.T, productID int64, ct costcalcdom.CalculationType) float64 {
	t.Helper()
	var total float64
	require.NoError(t, e.raw.QueryRowContext(ctx, `
		SELECT cpc_total_cost FROM cst_product_cost
		 WHERE cpc_product_sys_id=$1 AND cpc_period=$2 AND cpc_calculation_type=$3
		   AND cpc_status <> 'SUPERSEDED'`, productID, ac09Period, string(ct)).Scan(&total))
	return total
}

func ac09Result(productID, headID int64, ct costcalcdom.CalculationType, total float64) *costcalcdom.Result {
	return costcalcdom.NewResult(
		productID, ac09Period, ct, headID, 1,
		total, 6, 4, total, 0, "USD",
		nil, nil, nil, nil, fmt.Sprintf("h-%d-%s-%v", productID, ct, total), 0, "integ-test",
		0, 0, 0, 0, 0, 0, 0,
	)
}

func seedAC09Product(ctx context.Context, t *testing.T, db *sql.DB, code string) (productID, headID int64) {
	t.Helper()
	var typeID int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT cpt_type_id FROM cost_product_type ORDER BY cpt_type_id LIMIT 1`).Scan(&typeID))
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO cost_product_master (
			cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by
		) VALUES ($1, $2, 'AC-09 period lock product', 'integ-test', 'integ-test')
		RETURNING cpm_product_sys_id`, code, typeID).Scan(&productID))
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO cost_route_head (
			crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by
		) VALUES ($1, 'DRAFT', 1, 'integ-test', 'integ-test')
		RETURNING crh_head_id`, productID).Scan(&headID))
	return productID, headID
}

func triggerCmd(ct costcalcdom.CalculationType) costcalc.TriggerCommand {
	return costcalc.TriggerCommand{
		Period: ac09Period, CalcType: ct, Scope: costcalcdom.ScopeAll,
		TriggeredBy: "MANUAL", Actor: "integ-test",
	}
}

func TestPeriodLockAC09Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	e := newAC09Env(ctx, t, "period_lock_ac09_it")

	// Fixtures before the lock: p1 ACTUAL CALCULATED (verify target), p2 ACTUAL
	// VERIFIED (approve target), p1 FORECAST.
	p1, h1 := seedAC09Product(ctx, t, e.raw, "AC09-P1")
	p2, h2 := seedAC09Product(ctx, t, e.raw, "AC09-P2")
	c1, _, _, _, err := e.results.UpsertWithSupersede(ctx, ac09Result(p1, h1, costcalcdom.CalcTypeActual, 10))
	require.NoError(t, err)
	c2, _, _, _, err := e.results.UpsertWithSupersede(ctx, ac09Result(p2, h2, costcalcdom.CalcTypeActual, 20))
	require.NoError(t, err)
	_, _, _, _, err = e.results.UpsertWithSupersede(ctx, ac09Result(p1, h1, costcalcdom.CalcTypeForecast, 30))
	require.NoError(t, err)
	require.NoError(t, e.verify.Handle(ctx, costcalc.VerifyCostCommand{CostID: c2, Actor: "integ-test"}))
	require.Equal(t, "VERIFIED", e.status(ctx, t, c2))

	// 1. Lock 202608 ACTUAL.
	_, err = e.lockH.Handle(ctx, plapp.LockCommand{Period: ac09Period, User: "alice", Reason: "month close"})
	require.NoError(t, err)
	locked, err := e.locks.IsLocked(ctx, ac09Period, periodlock.CalcTypeActual)
	require.NoError(t, err)
	require.True(t, locked)
	_, err = e.lockH.Handle(ctx, plapp.LockCommand{Period: ac09Period, User: "alice", Reason: "again"})
	require.ErrorIs(t, err, periodlock.ErrAlreadyLocked)

	// 2. Every guarded ACTUAL path is refused and writes nothing.
	t.Run("refused/trigger", func(t *testing.T) {
		pubBefore := e.pub.count()
		for _, scope := range []costcalcdom.JobScope{costcalcdom.ScopeAll, costcalcdom.ScopeSingleProduct} {
			cmd := triggerCmd(costcalcdom.CalcTypeActual)
			cmd.Scope, cmd.ProductSysID = scope, p1
			_, err := e.trigger.Handle(ctx, cmd)
			require.ErrorIs(t, err, costcalcdom.ErrPeriodLocked, "scope %s", scope)
		}
		assert.Zero(t, e.count(ctx, t,
			`SELECT count(*) FROM cal_job WHERE cj_period=$1 AND cj_calculation_type='ACTUAL'`, ac09Period))
		assert.Equal(t, pubBefore, e.pub.count())
	})
	t.Run("refused/verify", func(t *testing.T) {
		err := e.verify.Handle(ctx, costcalc.VerifyCostCommand{CostID: c1, Actor: "integ-test"})
		require.ErrorIs(t, err, costcalcdom.ErrPeriodLocked)
		assert.Equal(t, "CALCULATED", e.status(ctx, t, c1))
	})
	t.Run("refused/approve", func(t *testing.T) {
		err := e.approve.Handle(ctx, costcalc.ApproveCostCommand{CostID: c2, Actor: "integ-test"})
		require.ErrorIs(t, err, costcalcdom.ErrPeriodLocked)
		assert.Equal(t, "VERIFIED", e.status(ctx, t, c2))
	})
	t.Run("refused/mb_recompute", func(t *testing.T) {
		_, err := e.mbTrigger.Handle(ctx, ac09Period, "integ-test")
		require.ErrorIs(t, err, costcalcdom.ErrPeriodLocked)
		assert.Zero(t, e.jobs(ctx, t, "ACTUAL", "MB_BATCH"))
	})
	t.Run("refused/mb_push", func(t *testing.T) {
		_, err := e.mbPush.Execute(ctx, ac09Period, []string{"any-mbh"}, "integ-test")
		require.ErrorIs(t, err, costcalcdom.ErrPeriodLocked)
		assert.Zero(t, e.count(ctx, t, `SELECT count(*) FROM mst_mb_push_log`))
	})
	t.Run("refused/supersede", func(t *testing.T) {
		_, _, _, _, err := e.results.UpsertWithSupersede(ctx, ac09Result(p1, h1, costcalcdom.CalcTypeActual, 99))
		require.ErrorIs(t, err, costcalcdom.ErrPeriodLocked)
		assert.InDelta(t, 10, e.activeTotal(ctx, t, p1, costcalcdom.CalcTypeActual), 1e-9)
		assert.Equal(t, "CALCULATED", e.status(ctx, t, c1))
	})

	// 3. FORECAST 202608 is not locked (Q7): it triggers and supersedes.
	t.Run("forecast_allowed", func(t *testing.T) {
		pubBefore := e.pub.count()
		job, err := e.trigger.Handle(ctx, triggerCmd(costcalcdom.CalcTypeForecast))
		require.NoError(t, err)
		assert.Equal(t, costcalcdom.JobStatusQueued, job.Status())
		assert.Equal(t, 1, e.jobs(ctx, t, "FORECAST", "ALL"))
		assert.Equal(t, pubBefore+1, e.pub.count())
		_, _, _, _, err = e.results.UpsertWithSupersede(ctx, ac09Result(p1, h1, costcalcdom.CalcTypeForecast, 31))
		require.NoError(t, err)
		assert.InDelta(t, 31, e.activeTotal(ctx, t, p1, costcalcdom.CalcTypeForecast), 1e-9)
	})

	// 4. Unlock: needs a reason, is refused for a posted period, then succeeds.
	_, err = plapp.NewUnlockHandler(e.locks, plapp.StaticAdjPostedProbe{}, e.emitter).Handle(ctx,
		plapp.UnlockCommand{Period: ac09Period, User: "bob", Reason: "  "})
	require.ErrorIs(t, err, periodlock.ErrReasonRequired)
	_, err = plapp.NewUnlockHandler(e.locks, plapp.StaticAdjPostedProbe{Posted: true}, e.emitter).Handle(ctx,
		plapp.UnlockCommand{Period: ac09Period, User: "bob", Reason: "reopen"})
	require.ErrorIs(t, err, periodlock.ErrUnlockPostedPeriod)
	res, err := plapp.NewUnlockHandler(e.locks, plapp.StaticAdjPostedProbe{}, e.emitter).Handle(ctx,
		plapp.UnlockCommand{Period: ac09Period, User: "bob", Reason: "reopen for correction"})
	require.NoError(t, err)
	assert.False(t, res.Lock.IsLocked())
	assert.Equal(t, "reopen for correction", res.Lock.UnlockReason())
	assert.Equal(t, 1, e.count(ctx, t,
		`SELECT count(*) FROM cost_audit_log WHERE cal_entity_type='cst_period_lock' AND cal_entity_id=$1 AND cal_operation='ERP_PERIOD_UNLOCK'`,
		202608))

	// 5. Unlocked: every path works again.
	t.Run("restored/trigger", func(t *testing.T) {
		pubBefore := e.pub.count()
		job, err := e.trigger.Handle(ctx, triggerCmd(costcalcdom.CalcTypeActual))
		require.NoError(t, err)
		assert.Positive(t, job.ID())
		assert.Equal(t, 1, e.jobs(ctx, t, "ACTUAL", "ALL"))
		assert.Equal(t, pubBefore+1, e.pub.count())
	})
	t.Run("restored/verify", func(t *testing.T) {
		require.NoError(t, e.verify.Handle(ctx, costcalc.VerifyCostCommand{CostID: c1, Actor: "integ-test"}))
		assert.Equal(t, "VERIFIED", e.status(ctx, t, c1))
	})
	t.Run("restored/approve", func(t *testing.T) {
		require.NoError(t, e.approve.Handle(ctx, costcalc.ApproveCostCommand{CostID: c2, Actor: "integ-test"}))
		assert.Equal(t, "APPROVED", e.status(ctx, t, c2))
	})
	t.Run("restored/mb_recompute", func(t *testing.T) {
		out, err := e.mbTrigger.Handle(ctx, ac09Period, "integ-test")
		require.NoError(t, err)
		assert.Positive(t, out.JobID)
		assert.Equal(t, 1, e.jobs(ctx, t, "ACTUAL", "MB_BATCH"))
	})
	t.Run("restored/mb_push", func(t *testing.T) {
		out, err := e.mbPush.Execute(ctx, ac09Period, nil, "integ-test")
		require.NoError(t, err)
		assert.Equal(t, ac09Period, out.Period)
	})
	t.Run("restored/supersede", func(t *testing.T) {
		_, _, _, _, err := e.results.UpsertWithSupersede(ctx, ac09Result(p1, h1, costcalcdom.CalcTypeActual, 11))
		require.NoError(t, err)
		assert.InDelta(t, 11, e.activeTotal(ctx, t, p1, costcalcdom.CalcTypeActual), 1e-9)
	})
}

// TestPeriodLockAC09RaceIntegration covers the concurrent supersede-vs-lock
// race for a period that has NO lock row yet (so FOR SHARE has no row to lock
// and the serialization comes from Lock's table lock):
//
//   - write first: an ACTUAL supersede tx that already passed the check keeps
//     Lock waiting until it commits, so the write is ordered before the lock;
//   - lock first: a supersede started while a first lock is uncommitted waits
//     for it and then fails with ErrPeriodLocked.
//
// The soft-unlocked re-lock race is covered by
// postgres.TestCostResultSupersedePeriodLockIntegration.
func TestPeriodLockAC09RaceIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	e := newAC09Env(ctx, t, "period_lock_ac09_race_it")
	p1, h1 := seedAC09Product(ctx, t, e.raw, "AC09-R1")
	_, _, _, _, err := e.results.UpsertWithSupersede(ctx, ac09Result(p1, h1, costcalcdom.CalcTypeActual, 10))
	require.NoError(t, err)
	require.Zero(t, e.count(ctx, t, `SELECT count(*) FROM cst_period_lock`))

	const blockWindow = 300 * time.Millisecond
	const resumeTimeout = 10 * time.Second

	t.Run("write_first_then_lock", func(t *testing.T) {
		txB, err := e.raw.BeginTx(ctx, nil)
		require.NoError(t, err)
		_, _, _, _, err = e.results.UpsertWithSupersedeTx(ctx, txB, ac09Result(p1, h1, costcalcdom.CalcTypeActual, 20))
		require.NoError(t, err)

		done := make(chan error, 1)
		go func() {
			_, e2 := e.lockH.Handle(ctx, plapp.LockCommand{Period: ac09Period, User: "alice", Reason: "close"})
			done <- e2
		}()
		select {
		case e2 := <-done:
			_ = txB.Rollback()
			t.Fatalf("lock must wait for the in-flight ACTUAL write, returned early: %v", e2)
		case <-time.After(blockWindow):
		}
		require.NoError(t, txB.Commit())
		select {
		case e2 := <-done:
			require.NoError(t, e2)
		case <-time.After(resumeTimeout):
			t.Fatal("lock did not resume after the write committed")
		}
		// The write landed before the lock; nothing after the lock does.
		assert.InDelta(t, 20, e.activeTotal(ctx, t, p1, costcalcdom.CalcTypeActual), 1e-9)
		_, _, _, _, err = e.results.UpsertWithSupersede(ctx, ac09Result(p1, h1, costcalcdom.CalcTypeActual, 21))
		require.ErrorIs(t, err, costcalcdom.ErrPeriodLocked)
		assert.InDelta(t, 20, e.activeTotal(ctx, t, p1, costcalcdom.CalcTypeActual), 1e-9)
	})

	t.Run("lock_first_then_write", func(t *testing.T) {
		// Reset to "never locked" so this is again a FIRST lock with no row.
		_, err := e.raw.ExecContext(ctx, `DELETE FROM cst_period_lock WHERE cpl_period=$1`, ac09Period)
		require.NoError(t, err)

		// Tx L mirrors PeriodLockRepository.Lock but holds the commit open.
		txL, err := e.raw.BeginTx(ctx, nil)
		require.NoError(t, err)
		_, err = txL.ExecContext(ctx, `LOCK TABLE cst_period_lock IN EXCLUSIVE MODE`)
		require.NoError(t, err)
		_, err = txL.ExecContext(ctx, `
			INSERT INTO cst_period_lock (cpl_period, cpl_calc_type, cpl_locked_at, cpl_locked_by, cpl_reason)
			VALUES ($1, 'ACTUAL', NOW(), 'alice', 'close')`, ac09Period)
		require.NoError(t, err)

		done := make(chan error, 1)
		go func() {
			_, _, _, _, e2 := e.results.UpsertWithSupersede(ctx, ac09Result(p1, h1, costcalcdom.CalcTypeActual, 30))
			done <- e2
		}()
		select {
		case e2 := <-done:
			_ = txL.Rollback()
			t.Fatalf("supersede must wait for the uncommitted first lock, returned early: %v", e2)
		case <-time.After(blockWindow):
		}
		require.NoError(t, txL.Commit())
		select {
		case e2 := <-done:
			require.ErrorIs(t, e2, costcalcdom.ErrPeriodLocked)
		case <-time.After(resumeTimeout):
			t.Fatal("supersede did not resume after the lock committed")
		}
		assert.InDelta(t, 20, e.activeTotal(ctx, t, p1, costcalcdom.CalcTypeActual), 1e-9)
	})
}
