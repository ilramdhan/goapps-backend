package workorder_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// PLAN-06 T4. These tests exist because PLAN-06 shipped with five critical
// defects that `go build`, `go vet` and golangci-lint all passed: none of them
// can read SQL inside a string literal, and none of them can tell that
// validateLot ran unconditionally on a path whose caller always sends a blank
// lot. Every case below is a defect that reached the working tree.

const (
	carrySourceMonth = "2026-08"
	carryTargetMonth = "2026-09"
)

// carrySourceDeadline anchors the source WO in carrySourceMonth. A WO has no
// first-class month — the repository derives it as
// TO_CHAR(wo_deadline,'YYYY-MM') — so the deadline IS the month.
var carrySourceDeadline = time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// stubWOCarryRepo answers the three carry read-model queries from fixed values.
// It stands in for wo_carry_repository.go, whose SQL these tests cannot reach —
// what they can pin is that the application layer applies every figure the SQL
// returns.
type stubWOCarryRepo struct {
	coverage    workorderdomain.CoverCoverage
	carriedInto bool
	candidates  []*workorderdomain.CarryCandidate
	coverCalls  int
}

func (r *stubWOCarryRepo) ListCandidates(
	context.Context, string, string,
) ([]*workorderdomain.CarryCandidate, error) {
	return r.candidates, nil
}

func (r *stubWOCarryRepo) IsAlreadyCarriedInto(context.Context, int64, string) (bool, error) {
	return r.carriedInto, nil
}

func (r *stubWOCarryRepo) CarryCoverage(
	context.Context, int64,
) (workorderdomain.CoverCoverage, error) {
	r.coverCalls++
	return r.coverage, nil
}

// txLotProv models the Postgres provisioner's transaction boundary: the
// sequence is minted inside the transaction and only kept when the whole unit
// commits. stubLotProv in lot_test.go bumps its counter before calling build,
// so it cannot express the rollback guarantee this file asserts.
type txLotProv struct {
	repo  *memRepo
	lots  *stubLots
	seq   int
	calls int
}

func (p *txLotProv) CreateWithGeneratedLot(
	ctx context.Context,
	req workorderdomain.LotProvisionRequest,
	build func(lotNo string) (*workorderdomain.WorkOrder, error),
) (*workorderdomain.WorkOrder, error) {
	p.calls++
	seq := p.seq + 1 // minted inside the transaction, not yet committed
	lotNo := workorderdomain.FormatLotNo(req.AreaCode, req.Year, seq)
	entity, err := build(lotNo)
	if err != nil {
		return nil, err // rollback: no lot_master row, sequence not advanced
	}
	if err := p.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	p.seq = seq
	p.lots.known[lotNo] = true
	return entity, nil
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// putCarrySource seeds a source WO at an arbitrary status. Reconstruct is the
// production rehydration path (woDTO.toEntity uses it), so seeding a RUNNING or
// CLOSED WO this way is what the repository itself does — no transition walk is
// needed to reach a status the state machine only allows from a specific
// predecessor.
func putCarrySource(
	r *memRepo, status string, qtyTarget float64, mutate func(*workorderdomain.ReconstructParams),
) *workorderdomain.WorkOrder {
	r.seq++
	p := workorderdomain.ReconstructParams{
		ID:           r.seq,
		WoNo:         "WO-CARRY-SRC",
		LotNo:        "TXT0009-26",
		AreaCode:     "TXT",
		MachineID:    2,
		CrhHeadID:    10,
		CrhVersion:   1,
		PlanItemID:   7,
		DemandID:     ptrI64(99),
		QtyTarget:    qtyTarget,
		Deadline:     carrySourceDeadline,
		ProdCategory: workorderdomain.ProdCategoryNormal,
		Status:       status,
		CreatedBy:    1,
	}
	if mutate != nil {
		mutate(&p)
	}
	wo := workorderdomain.Reconstruct(p)
	r.orders[wo.ID()] = wo
	return wo
}

// carrySvc wires the full carry chain: the WO repository, the carry read model,
// and the lot-provisioning collaborators a blank-lot carry needs.
func carrySvc(
	repo *memRepo, lots *stubLots, prov workorderapp.LotProvisioner, carry *stubWOCarryRepo,
) *workorderapp.Service {
	return workorderapp.NewService(repo, workorderapp.Deps{
		Lots:        lots,
		PlanItems:   &stubPlanItems{productSysID: 900},
		Resolver:    workorderdomain.NewResolver(&stubParamDefs{stdWeight: "5"}, nil, nil, nil),
		LotSpecs:    &stubLotSpecs{item: testItemCode, shade: testShadeCode},
		LotProv:     prov,
		WOCarryRepo: carry,
	})
}

// carryCmd is the command the frontend actually sends: no lot, no explicit qty.
func carryCmd(sourceWOID int64) workorderapp.ProcessWorkOrderCarryForwardCommand {
	return workorderapp.ProcessWorkOrderCarryForwardCommand{
		SourceWOID:  sourceWOID,
		TargetMonth: carryTargetMonth,
		LotNo:       "",
		ActedBy:     42,
	}
}

// ---------------------------------------------------------------------------
// T4.1 — every eligible status carries
// ---------------------------------------------------------------------------

// A WO mid-approval or mid-production still has remaining quantity that has to
// reach the new month. Only the five terminal/pre-confirmation statuses are
// excluded; everything else must succeed.
func TestProcessWorkOrderCarryForward_EligibleStatuses(t *testing.T) {
	eligible := []string{
		workorderdomain.StatusSubmitted,
		workorderdomain.StatusPCApproved,
		workorderdomain.StatusApproved,
		workorderdomain.StatusScheduled,
		workorderdomain.StatusChangeover,
		workorderdomain.StatusRunning,
	}
	for _, status := range eligible {
		t.Run(status, func(t *testing.T) {
			repo := newMemRepo()
			lots := &stubLots{known: map[string]bool{"TXT0009-26": true}}
			prov := &txLotProv{repo: repo, lots: lots}
			carry := &stubWOCarryRepo{coverage: workorderdomain.CoverCoverage{QtyProduced: 200}}
			svc := carrySvc(repo, lots, prov, carry)
			src := putCarrySource(repo, status, 500, nil)

			created, err := svc.ProcessWorkOrderCarryForward(context.Background(), carryCmd(src.ID()))
			require.NoError(t, err)

			// A carry is a CONTINUATION, hard-linked to its source: that link is
			// what carriedAwayExpr sums, so without it the same qty could be
			// carried into every month forever.
			require.NotNil(t, created.RefWoID())
			assert.Equal(t, src.ID(), *created.RefWoID())
			assert.Equal(t, workorderdomain.RefTypeContinuation, created.RefType())
			require.NotNil(t, created.DemandID(), "a continuation inherits the source demand")
			assert.Equal(t, int64(99), *created.DemandID())
			assert.Equal(t, workorderdomain.StatusDraft, created.Status(),
				"the carried WO starts as a draft the planner still confirms")
			// H3: the continuation is created by whoever clicked carry-forward.
			// Before the fix this was 0 — no row in the audit trail.
			assert.Equal(t, int64(42), created.CreatedBy())
			assert.InDelta(t, 300.0, created.QtyTarget(), 1e-9)
			assert.Equal(t, 1, carry.coverCalls,
				"the server re-reads coverage rather than trusting the client's figure")
		})
	}
}

// ---------------------------------------------------------------------------
// T4.2 — every excluded status is refused, with its own reason
// ---------------------------------------------------------------------------

// M3: every ineligibility used to collapse into one sentinel, so a planner was
// told "cannot be carried" with no way to know whether to confirm the draft or
// stop trying. Each status must carry prose that names the next action.
func TestProcessWorkOrderCarryForward_IneligibleStatuses(t *testing.T) {
	ineligible := []string{
		workorderdomain.StatusDraft,
		workorderdomain.StatusCompleted,
		workorderdomain.StatusClosed,
		workorderdomain.StatusRejected,
		workorderdomain.StatusCancelled,
	}
	reasons := make(map[string]string, len(ineligible))
	for _, status := range ineligible {
		t.Run(status, func(t *testing.T) {
			repo := newMemRepo()
			lots := &stubLots{known: map[string]bool{}}
			prov := &txLotProv{repo: repo, lots: lots}
			carry := &stubWOCarryRepo{}
			svc := carrySvc(repo, lots, prov, carry)
			src := putCarrySource(repo, status, 500, nil)

			_, err := svc.ProcessWorkOrderCarryForward(context.Background(), carryCmd(src.ID()))
			require.Error(t, err)

			var ineligibleErr workorderdomain.CarryIneligibleError
			require.ErrorAs(t, err, &ineligibleErr,
				"the handler renders the reason, so it must survive as a typed error")
			assert.NotEmpty(t, ineligibleErr.Reason)
			reasons[status] = ineligibleErr.Reason

			// Refusal happens before any work: no coverage read, no lot minted,
			// no WO written.
			assert.Zero(t, carry.coverCalls)
			assert.Zero(t, prov.calls)
			assert.Len(t, repo.orders, 1, "only the source WO exists")
		})
	}

	seen := make(map[string]string, len(reasons))
	for status, reason := range reasons {
		if other, dup := seen[reason]; dup {
			t.Errorf("%s and %s share the reason %q — the planner cannot tell them apart",
				status, other, reason)
		}
		seen[reason] = status
	}
}

// ---------------------------------------------------------------------------
// T4.3 — remaining-qty arithmetic
// ---------------------------------------------------------------------------

// remaining = qty_target − produced − already-carried, clamped at zero. Both
// debits matter: dropping the carried-away term (the H2 shape) lets the same
// quantity be carried into an unlimited number of months.
func TestProcessWorkOrderCarryForward_RemainingQty(t *testing.T) {
	tests := []struct {
		name      string
		target    float64
		produced  float64
		carried   float64
		carryQty  *float64
		wantQty   float64
		wantErrIs error
	}{
		{name: "nothing produced carries the whole target", target: 500, wantQty: 500},
		{name: "production debits the carry", target: 500, produced: 200, wantQty: 300},
		{
			name: "an earlier carry debits it too",
			// Source-scoped, not target-scoped: a qty handed to August's
			// continuation is not available again for September.
			target: 500, produced: 200, carried: 100, wantQty: 200,
		},
		{
			name:   "overproduction clamps at zero rather than going negative",
			target: 500, produced: 600, wantErrIs: workorderdomain.ErrNothingToCarry,
		},
		{
			name:   "production plus earlier carries can exhaust it",
			target: 500, produced: 300, carried: 200,
			wantErrIs: workorderdomain.ErrNothingToCarry,
		},
		{
			name:   "a partial carry takes only what was asked",
			target: 500, produced: 200, carryQty: fptr(100), wantQty: 100,
		},
		{
			name:   "a partial carry equal to the remainder is allowed",
			target: 500, produced: 200, carryQty: fptr(300), wantQty: 300,
		},
		{
			name: "a partial carry beyond the remainder is refused, not clamped",
			// Clamping would silently ship a different number than the planner
			// typed; refusing tells them the figure is stale.
			target: 500, produced: 200, carryQty: fptr(400),
			wantErrIs: workorderdomain.ErrCarryQtyExceedsRemaining,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemRepo()
			lots := &stubLots{known: map[string]bool{}}
			prov := &txLotProv{repo: repo, lots: lots}
			carry := &stubWOCarryRepo{coverage: workorderdomain.CoverCoverage{
				QtyProduced:       tt.produced,
				QtyAlreadyCarried: tt.carried,
			}}
			svc := carrySvc(repo, lots, prov, carry)
			src := putCarrySource(repo, workorderdomain.StatusRunning, tt.target, nil)

			cmd := carryCmd(src.ID())
			cmd.CarryQty = tt.carryQty
			created, err := svc.ProcessWorkOrderCarryForward(context.Background(), cmd)

			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
				assert.Zero(t, prov.calls, "a refused carry mints no lot")
				assert.Len(t, repo.orders, 1)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tt.wantQty, created.QtyTarget(), 1e-9)
		})
	}
}

// A carry qty of zero means "not specified" on the wire (proto scalars have no
// presence here), so it must fall back to the full remainder rather than
// creating a zero-qty WO the domain would reject.
func TestProcessWorkOrderCarryForward_ZeroCarryQtyMeansFullRemainder(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	prov := &txLotProv{repo: repo, lots: lots}
	carry := &stubWOCarryRepo{coverage: workorderdomain.CoverCoverage{QtyProduced: 150}}
	svc := carrySvc(repo, lots, prov, carry)
	src := putCarrySource(repo, workorderdomain.StatusRunning, 500, nil)

	cmd := carryCmd(src.ID())
	cmd.CarryQty = fptr(0)
	created, err := svc.ProcessWorkOrderCarryForward(context.Background(), cmd)
	require.NoError(t, err)
	assert.InDelta(t, 350.0, created.QtyTarget(), 1e-9)
}

// ---------------------------------------------------------------------------
// T4.4 — lot minting
// ---------------------------------------------------------------------------

// C5, the defect that made carry-forward impossible: validateLot ran on every
// reference create, while the carry path always sends a blank lot. Every carry
// died with "invalid lot: this lot is not registered". A blank lot must be
// minted and registered instead.
func TestProcessWorkOrderCarryForward_BlankLotIsGenerated(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{"TXT0009-26": true}}
	prov := &txLotProv{repo: repo, lots: lots}
	carry := &stubWOCarryRepo{}
	svc := carrySvc(repo, lots, prov, carry)
	src := putCarrySource(repo, workorderdomain.StatusRunning, 500, nil)

	created, err := svc.ProcessWorkOrderCarryForward(context.Background(), carryCmd(src.ID()))
	require.NoError(t, err)

	wantLot := workorderdomain.FormatLotNo("TXT", time.Now().Year(), 1)
	assert.Equal(t, 1, prov.calls, "a blank lot goes through the provisioner")
	assert.Equal(t, wantLot, created.LotNo())
	assert.NotEqual(t, src.LotNo(), created.LotNo(),
		"a continuation is a fresh production run and needs its own lot")
	assert.True(t, lots.known[created.LotNo()],
		"the generated lot must be registered, or the ETL prices its bobbins at zero")
}

// A lot the planner typed still has to exist. The blank-lot path must not have
// reopened the hole where a WO points at a lot with no standard weights.
func TestProcessWorkOrderCarryForward_SuppliedLotStillValidated(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	prov := &txLotProv{repo: repo, lots: lots}
	carry := &stubWOCarryRepo{}
	svc := carrySvc(repo, lots, prov, carry)
	src := putCarrySource(repo, workorderdomain.StatusRunning, 500, nil)

	cmd := carryCmd(src.ID())
	cmd.LotNo = "TYPO-1"
	_, err := svc.ProcessWorkOrderCarryForward(context.Background(), cmd)
	require.ErrorIs(t, err, workorderdomain.ErrLotNotFound)
	assert.Zero(t, prov.calls, "a supplied lot never reaches the generator")
	assert.Len(t, repo.orders, 1)
}

// The carried WO has to land in the target month, since the month is derived
// from the deadline. A source deadline earlier than the target month would
// otherwise put the continuation back in the month it was carried out of.
func TestProcessWorkOrderCarryForward_DeadlineMovesIntoTargetMonth(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	prov := &txLotProv{repo: repo, lots: lots}
	carry := &stubWOCarryRepo{}
	svc := carrySvc(repo, lots, prov, carry)
	src := putCarrySource(repo, workorderdomain.StatusRunning, 500, nil)

	created, err := svc.ProcessWorkOrderCarryForward(context.Background(), carryCmd(src.ID()))
	require.NoError(t, err)
	assert.Equal(t, carryTargetMonth, created.Deadline().Format("2006-01"))
	assert.True(t, created.Deadline().After(src.Deadline()))
}

// ---------------------------------------------------------------------------
// T4.5 — double-carry prevention
// ---------------------------------------------------------------------------

// S-2.4: carrying the same source into the same month twice must not create two
// continuations. This is the per-target guard; the qty-level source-scoped
// guard is covered by the remaining-qty table above.
func TestProcessWorkOrderCarryForward_AlreadyCarriedIntoTargetMonth(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	prov := &txLotProv{repo: repo, lots: lots}
	carry := &stubWOCarryRepo{
		carriedInto: true,
		coverage:    workorderdomain.CoverCoverage{QtyProduced: 100},
	}
	svc := carrySvc(repo, lots, prov, carry)
	src := putCarrySource(repo, workorderdomain.StatusRunning, 500, nil)

	_, err := svc.ProcessWorkOrderCarryForward(context.Background(), carryCmd(src.ID()))
	require.ErrorIs(t, err, workorderdomain.ErrAlreadyCarriedIntoMonth)
	assert.Zero(t, prov.calls)
	assert.Zero(t, carry.coverCalls,
		"the duplicate check short-circuits before the coverage read")
	assert.Len(t, repo.orders, 1)
}

// ---------------------------------------------------------------------------
// T4.6 — a rolled-back carry burns no lot sequence
// ---------------------------------------------------------------------------

// The provisioner mints the sequence inside the WO's transaction precisely so a
// WO that fails validation after the lot was minted rolls the counter back
// instead of burning a number. A gap in the lot sequence is not cosmetic: lots
// are transcribed onto paper doff tags and reconciled by hand.
func TestProcessWorkOrderCarryForward_FailedCarryBurnsNoLotSequence(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	prov := &txLotProv{repo: repo, lots: lots}
	carry := &stubWOCarryRepo{}
	svc := carrySvc(repo, lots, prov, carry)

	// A source whose route snapshot is missing: the continuation fails in
	// workorder.New, after the lot has been minted inside the transaction.
	broken := putCarrySource(repo, workorderdomain.StatusRunning, 500, func(p *workorderdomain.ReconstructParams) {
		p.CrhHeadID = 0
	})

	_, err := svc.ProcessWorkOrderCarryForward(context.Background(), carryCmd(broken.ID()))
	require.ErrorIs(t, err, workorderdomain.ErrInvalidRoute)
	assert.Equal(t, 1, prov.calls, "the provisioner was entered")
	assert.Zero(t, prov.seq, "the rolled-back transaction must not advance the sequence")
	assert.Empty(t, lots.known, "no lot_master row may survive the rollback")
	assert.Len(t, repo.orders, 1, "no work order may survive the rollback")

	// The next carry takes the number the failed one did not burn.
	healthy := putCarrySource(repo, workorderdomain.StatusRunning, 500, nil)
	created, err := svc.ProcessWorkOrderCarryForward(context.Background(), carryCmd(healthy.ID()))
	require.NoError(t, err)
	assert.Equal(t, workorderdomain.FormatLotNo("TXT", time.Now().Year(), 1), created.LotNo())
}

// ---------------------------------------------------------------------------
// M1 — same-month / backwards-carry guard
// ---------------------------------------------------------------------------

// A WO's month is TO_CHAR(wo_deadline,'YYYY-MM'). Carrying into the same month
// therefore produces a WO whose deadline lands back in the source month, which
// the candidate list then offers as a fresh candidate for the same move.
// Carrying backwards pushes work into a month whose production is already
// reported. Both were accepted before this guard.
func TestProcessWorkOrderCarryForward_TargetMonthMustBeLater(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr error
	}{
		{name: "the next month is the normal case", target: "2026-09"},
		{name: "skipping a month is allowed", target: "2026-11"},
		{name: "crossing the year boundary is allowed", target: "2027-01"},
		{
			name:   "the source's own month is refused",
			target: carrySourceMonth, wantErr: workorderdomain.ErrCarryTargetNotLater,
		},
		{
			name:   "the previous month is refused",
			target: "2026-07", wantErr: workorderdomain.ErrCarryTargetNotLater,
		},
		{
			name: "the previous December is refused across the year boundary",
			// A plain string comparison that ignored the year would read "12" as
			// later than "08" and let this through.
			target: "2025-12", wantErr: workorderdomain.ErrCarryTargetNotLater,
		},
		{
			name:   "a malformed month is refused rather than silently ignored",
			target: "Sept 2026", wantErr: workorderdomain.ErrInvalidTargetMonth,
		},
		{
			name:   "an empty month is refused",
			target: "", wantErr: workorderdomain.ErrInvalidTargetMonth,
		},
		{
			name: "a month-shaped string with a bad month number is refused",
			// carryDeadlineWO silently returns the source deadline when the parse
			// fails, so without this guard the carry would land in the source
			// month with no complaint at all.
			target: "2026-13", wantErr: workorderdomain.ErrInvalidTargetMonth,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemRepo()
			lots := &stubLots{known: map[string]bool{}}
			prov := &txLotProv{repo: repo, lots: lots}
			carry := &stubWOCarryRepo{}
			svc := carrySvc(repo, lots, prov, carry)
			src := putCarrySource(repo, workorderdomain.StatusRunning, 500, nil)

			cmd := carryCmd(src.ID())
			cmd.TargetMonth = tt.target
			created, err := svc.ProcessWorkOrderCarryForward(context.Background(), cmd)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				// The guard runs before any side effect: no duplicate check, no
				// coverage read, no lot minted, no WO written.
				assert.Zero(t, carry.coverCalls)
				assert.Zero(t, prov.calls)
				assert.Zero(t, prov.seq)
				assert.Len(t, repo.orders, 1)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.target, created.Deadline().Format("2006-01"),
				"the carried WO must land in the month it was carried into")
		})
	}
}

// The comparison has to be on the formatted month, not on the instant. A
// deadline of 2026-09-01 00:00 WIB is 2026-08-31 17:00 UTC, and time.Parse reads
// "2026-09" as UTC midnight — so an instant comparison sees the target as
// strictly later than a WO that already lives in that month, and lets a
// same-month carry through. Plant deadlines are entered in local time, so this
// is the ordinary case at a month boundary, not a contrived one.
func TestProcessWorkOrderCarryForward_SameMonthRefusedAcrossTimeZones(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	prov := &txLotProv{repo: repo, lots: lots}
	carry := &stubWOCarryRepo{}
	svc := carrySvc(repo, lots, prov, carry)

	wib := time.FixedZone("WIB", 7*60*60)
	src := putCarrySource(repo, workorderdomain.StatusRunning, 500,
		func(p *workorderdomain.ReconstructParams) {
			p.Deadline = time.Date(2026, 9, 1, 0, 0, 0, 0, wib)
		})

	cmd := carryCmd(src.ID())
	cmd.TargetMonth = "2026-09" // the WO's own month, in its own zone
	_, err := svc.ProcessWorkOrderCarryForward(context.Background(), cmd)
	require.ErrorIs(t, err, workorderdomain.ErrCarryTargetNotLater)
	assert.Zero(t, prov.calls)
	assert.Len(t, repo.orders, 1)
}

// The guard compares months, not instants: a source deadline late in its month
// must still carry into the next month, whose first day is an earlier instant.
func TestProcessWorkOrderCarryForward_LateMonthDeadlineStillCarries(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	prov := &txLotProv{repo: repo, lots: lots}
	svc := carrySvc(repo, lots, prov, &stubWOCarryRepo{})
	src := putCarrySource(repo, workorderdomain.StatusRunning, 500,
		func(p *workorderdomain.ReconstructParams) {
			p.Deadline = time.Date(2026, 8, 31, 23, 30, 0, 0, time.UTC)
		})

	created, err := svc.ProcessWorkOrderCarryForward(context.Background(), carryCmd(src.ID()))
	require.NoError(t, err)
	assert.Equal(t, carryTargetMonth, created.Deadline().Format("2006-01"))
}

// ---------------------------------------------------------------------------
// Candidate listing
// ---------------------------------------------------------------------------

// H4: ineligible rows used to be filtered out of the candidate list, so a
// planner opening a month with nothing carryable saw an empty table and no
// reason. Every WO in the source month is returned, decorated.
func TestListWorkOrderCarryCandidates_ReturnsIneligibleWithReason(t *testing.T) {
	repo := newMemRepo()
	eligible := putCarrySource(repo, workorderdomain.StatusRunning, 500, nil)
	closed := putCarrySource(repo, workorderdomain.StatusClosed, 500, nil)
	carry := &stubWOCarryRepo{candidates: []*workorderdomain.CarryCandidate{
		{WO: eligible, MachineLabel: "AC3", ProductLabel: testItemCode},
		{
			WO: closed, MachineLabel: "AC4", ProductLabel: testItemCode,
			IneligibilityReason: "production is closed and its final quantity is locked",
		},
	}}
	svc := carrySvc(repo, &stubLots{known: map[string]bool{}}, nil, carry)

	got, err := svc.ListWorkOrderCarryCandidates(
		context.Background(), carrySourceMonth, carryTargetMonth,
	)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Empty(t, got[0].IneligibilityReason)
	assert.NotEmpty(t, got[1].IneligibilityReason)
	// Labels, never ids — a planner reads a machine number and a product code.
	for _, c := range got {
		assert.NotEmpty(t, c.MachineLabel)
		assert.NotEmpty(t, c.ProductLabel)
	}
}

// The application layer's ineligibility reasons and the repository's copy are
// two hand-maintained maps over the same five statuses. They are rendered in
// the same table, so a drift between them shows up as one row explaining itself
// differently depending on whether the planner listed or clicked. This asserts
// the application side stays complete; the repository copy is asserted against
// the same statuses by its own list query.
func TestIneligibleWOStatuses_CoverEveryTerminalStatus(t *testing.T) {
	repo := newMemRepo()
	svc := carrySvc(repo, &stubLots{known: map[string]bool{}}, nil, &stubWOCarryRepo{})

	// Every status the state machine can reach is either carryable or refused
	// with a reason — never silently accepted into a broken carry.
	all := []string{
		workorderdomain.StatusDraft, workorderdomain.StatusSubmitted,
		workorderdomain.StatusPCApproved, workorderdomain.StatusApproved,
		workorderdomain.StatusScheduled, workorderdomain.StatusChangeover,
		workorderdomain.StatusRunning, workorderdomain.StatusCompleted,
		workorderdomain.StatusClosed, workorderdomain.StatusRejected,
		workorderdomain.StatusCancelled,
	}
	for _, status := range all {
		src := putCarrySource(repo, status, 500, nil)
		_, err := svc.ProcessWorkOrderCarryForward(context.Background(), carryCmd(src.ID()))
		var ineligibleErr workorderdomain.CarryIneligibleError
		switch status {
		case workorderdomain.StatusDraft, workorderdomain.StatusCompleted,
			workorderdomain.StatusClosed, workorderdomain.StatusRejected,
			workorderdomain.StatusCancelled:
			require.ErrorAs(t, err, &ineligibleErr, "%s must be refused with a reason", status)
		default:
			// Eligible: it got past the status gate. With no provisioner wired
			// here it then fails on lot generation, which is the proof it was
			// never blocked on status.
			assert.False(t, errors.As(err, &ineligibleErr),
				"%s must not be refused on status", status)
			require.ErrorIs(t, err, workorderdomain.ErrLotGenerationUnavailable)
		}
	}
}
