package machinesync_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/ppc/internal/application/machinesync"
	machinedomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
)

// fakeRepo records UpsertSourced calls and replays scripted outcomes.
type fakeRepo struct {
	machinedomain.Repository
	calls    []machinedomain.SourcedMachine
	outcomes map[string]machinedomain.UpsertOutcome
	err      error
	// groups records each EnsureGroup call as "AREA|NAME" and assigns ids in
	// call order, so a test can assert both the set created and its de-duping.
	groups    []string
	groupIDs  map[string]int64
	groupErr  error
	groupSeed int64
}

func (r *fakeRepo) EnsureGroup(_ context.Context, name, area string) (int64, error) {
	key := area + "|" + name
	r.groups = append(r.groups, key)
	if r.groupErr != nil {
		return 0, r.groupErr
	}
	if r.groupIDs == nil {
		r.groupIDs = make(map[string]int64)
	}
	if id, ok := r.groupIDs[key]; ok {
		return id, nil
	}
	r.groupSeed++
	r.groupIDs[key] = r.groupSeed
	return r.groupSeed, nil
}

func (r *fakeRepo) UpsertSourced(_ context.Context, src machinedomain.SourcedMachine) (machinedomain.UpsertOutcome, error) {
	r.calls = append(r.calls, src)
	if r.err != nil {
		return machinedomain.OutcomeSkipped, r.err
	}
	if o, ok := r.outcomes[src.MachineNo]; ok {
		return o, nil
	}
	return machinedomain.OutcomeInserted, nil
}

type fakeFinance struct {
	machines []*financev1.Machine
	err      error
	degraded bool
}

func (f *fakeFinance) ListAllMachines(_ context.Context) ([]*financev1.Machine, error) {
	return f.machines, f.err
}
func (f *fakeFinance) IsDegraded() bool { return f.degraded }

type fakeOracle struct {
	rows []oracle.TxtMachine
	err  error
}

func (o *fakeOracle) ListTxtMachines(_ context.Context) ([]oracle.TxtMachine, error) {
	return o.rows, o.err
}

func financeMachine(id, code string, active bool) *financev1.Machine {
	return &financev1.Machine{MachineId: id, MachineCode: code, IsActive: active}
}

func findCall(calls []machinedomain.SourcedMachine, no string) (machinedomain.SourcedMachine, bool) {
	for _, c := range calls {
		if c.MachineNo == no {
			return c, true
		}
	}
	return machinedomain.SourcedMachine{}, false
}

func TestSync_MergesFinanceAndOracleArea(t *testing.T) {
	repo := &fakeRepo{}
	finance := &fakeFinance{machines: []*financev1.Machine{financeMachine("uuid-1", "M01", true)}}
	oracleSrc := &fakeOracle{rows: []oracle.TxtMachine{{MachNo: "M01", Dept: "TXT"}}}

	uc := machinesync.NewUsecase(repo, finance, oracleSrc)
	res, err := uc.Sync(context.Background())
	require.NoError(t, err)

	call, ok := findCall(repo.calls, "M01")
	require.True(t, ok)
	assert.Equal(t, "TXT", call.Area, "MACH_DEPT=TXT maps to area TXT")
	require.NotNil(t, call.SourceMcID)
	assert.Equal(t, "uuid-1", *call.SourceMcID)
	assert.True(t, call.IsActive)
	assert.False(t, call.SyncedAt.IsZero())
	assert.True(t, res.FinanceUsed)
	assert.True(t, res.OracleUsed)
	assert.Equal(t, 1, res.Inserted)
}

func TestSync_CarriesOrionLineAndGroup(t *testing.T) {
	repo := &fakeRepo{}
	finance := &fakeFinance{machines: []*financev1.Machine{financeMachine("uuid-1", "M01", true)}}
	oracleSrc := &fakeOracle{rows: []oracle.TxtMachine{{
		MachNo: "M01", Dept: "TXT", OrionCode: "TXT 01",
		Line: "TXLINE1", Group: "Texturising Machine",
	}}}

	uc := machinesync.NewUsecase(repo, finance, oracleSrc)
	_, err := uc.Sync(context.Background())
	require.NoError(t, err)

	call, ok := findCall(repo.calls, "M01")
	require.True(t, ok)
	assert.Equal(t, "TXT 01", call.OrionCode)
	assert.Equal(t, "TXLINE1", call.Line)
	require.NotNil(t, call.GroupID, "MACH_GROUP must resolve to a machine_group id")
	assert.Equal(t, repo.groupIDs["TXT|Texturising Machine"], *call.GroupID)
	assert.Equal(t, []string{"TXT|Texturising Machine"}, repo.groups)
}

// TXTMACH is the source of truth for machine→group assignment (G2): each
// distinct (group, area) pair is ensured exactly once per run, not once per
// machine, and a group whose department maps to no PPC area is skipped because
// machine_group.group_area is NOT NULL and CHECK-constrained to TXT/SPG/TWT.
func TestSync_EnsuresEachGroupOncePerRunAndSkipsUnmappedDept(t *testing.T) {
	repo := &fakeRepo{}
	finance := &fakeFinance{degraded: true}
	oracleSrc := &fakeOracle{rows: []oracle.TxtMachine{
		{MachNo: "T1", Dept: "TWT", Group: "TFO Machine"},
		{MachNo: "T2", Dept: "TWT", Group: "TFO Machine"},
		{MachNo: "X1", Dept: "TXT", Group: "TFO Machine"}, // same name, other area
		{MachNo: "J1", Dept: "", Group: "JLT"},            // NULL dept → skipped
	}}

	uc := machinesync.NewUsecase(repo, finance, oracleSrc)
	_, err := uc.Sync(context.Background())
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"TWT|TFO Machine", "TXT|TFO Machine"}, repo.groups,
		"one EnsureGroup per distinct (area, group); unmapped dept never reaches the repo")

	j1, ok := findCall(repo.calls, "J1")
	require.True(t, ok)
	assert.Nil(t, j1.GroupID, "unresolvable area leaves the group PPC-local (preserved)")
}

// A machine_group that cannot be created must not fail the whole sync: the
// affected machines are still upserted with a nil GroupID, which the
// repository's COALESCE-preserve turns into "keep whatever is stored".
func TestSync_GroupCreateFailureDegradesToPreserve(t *testing.T) {
	repo := &fakeRepo{groupErr: errors.New("group table down")}
	finance := &fakeFinance{degraded: true}
	oracleSrc := &fakeOracle{rows: []oracle.TxtMachine{
		{MachNo: "T1", Dept: "TWT", Group: "TFO Machine", Line: "TFOLINE"},
	}}

	uc := machinesync.NewUsecase(repo, finance, oracleSrc)
	_, err := uc.Sync(context.Background())
	require.NoError(t, err)

	call, ok := findCall(repo.calls, "T1")
	require.True(t, ok)
	assert.Nil(t, call.GroupID)
	assert.Equal(t, "TFOLINE", call.Line, "other Oracle fields still sync")
}

// 24 of 179 live TXTMACH rows carry no MACH_LINE. An empty source field must
// stay empty here so the repository maps it to NULL and COALESCE preserves the
// stored value instead of blanking a planner's edit.
func TestSync_EmptyOracleFieldsStayEmpty(t *testing.T) {
	repo := &fakeRepo{}
	finance := &fakeFinance{degraded: true}
	oracleSrc := &fakeOracle{rows: []oracle.TxtMachine{
		{MachNo: "T1", Dept: "TWT", OrionCode: "ACY 01", Line: "", Group: ""},
	}}

	uc := machinesync.NewUsecase(repo, finance, oracleSrc)
	_, err := uc.Sync(context.Background())
	require.NoError(t, err)

	call, ok := findCall(repo.calls, "T1")
	require.True(t, ok)
	assert.Equal(t, "ACY 01", call.OrionCode)
	assert.Empty(t, call.Line)
	assert.Nil(t, call.GroupID)
	assert.Empty(t, repo.groups, "an empty MACH_GROUP creates nothing")
}

func TestSync_DeptMappingVariants(t *testing.T) {
	repo := &fakeRepo{}
	finance := &fakeFinance{machines: []*financev1.Machine{
		financeMachine("u1", "A1", true),
		financeMachine("u2", "A2", true),
		financeMachine("u3", "A3", true),
		financeMachine("u4", "A4", true),
	}}
	oracleSrc := &fakeOracle{rows: []oracle.TxtMachine{
		{MachNo: "A1", Dept: "TXT"},
		{MachNo: "A2", Dept: "twt"}, // lowercase tolerated
		{MachNo: "A3", Dept: "SPG"},
		{MachNo: "A4", Dept: "XXX"}, // unknown → empty area
	}}

	uc := machinesync.NewUsecase(repo, finance, oracleSrc)
	_, err := uc.Sync(context.Background())
	require.NoError(t, err)

	want := map[string]string{"A1": "TXT", "A2": "TWT", "A3": "SPG", "A4": ""}
	for no, area := range want {
		call, ok := findCall(repo.calls, no)
		require.True(t, ok, no)
		assert.Equal(t, area, call.Area, no)
	}
}

func TestSync_FinanceOnlyLeavesAreaEmpty(t *testing.T) {
	repo := &fakeRepo{}
	finance := &fakeFinance{machines: []*financev1.Machine{financeMachine("u1", "M9", true)}}
	oracleSrc := &fakeOracle{rows: nil}

	uc := machinesync.NewUsecase(repo, finance, oracleSrc)
	res, err := uc.Sync(context.Background())
	require.NoError(t, err)

	call, ok := findCall(repo.calls, "M9")
	require.True(t, ok)
	assert.Empty(t, call.Area, "no Oracle match → area stays PPC-local (empty from source)")
	assert.False(t, res.OracleUsed)
	assert.True(t, res.FinanceUsed)
}

func TestSync_OracleUnreachableDegradesToFinanceOnly(t *testing.T) {
	repo := &fakeRepo{}
	finance := &fakeFinance{machines: []*financev1.Machine{financeMachine("u1", "M1", true)}}
	oracleSrc := &fakeOracle{err: errors.New("oracle down")}

	uc := machinesync.NewUsecase(repo, finance, oracleSrc)
	res, err := uc.Sync(context.Background())
	require.NoError(t, err)

	assert.Len(t, repo.calls, 1)
	assert.True(t, res.FinanceUsed)
	assert.False(t, res.OracleUsed)
}

func TestSync_NilOracleSource(t *testing.T) {
	repo := &fakeRepo{}
	finance := &fakeFinance{machines: []*financev1.Machine{financeMachine("u1", "M1", true)}}

	uc := machinesync.NewUsecase(repo, finance, nil)
	_, err := uc.Sync(context.Background())
	require.NoError(t, err)
	assert.Len(t, repo.calls, 1)
}

func TestSync_FinanceDegradedOracleOnly(t *testing.T) {
	repo := &fakeRepo{}
	finance := &fakeFinance{degraded: true}
	oracleSrc := &fakeOracle{rows: []oracle.TxtMachine{{MachNo: "T1", Dept: "TXT"}}}

	uc := machinesync.NewUsecase(repo, finance, oracleSrc)
	res, err := uc.Sync(context.Background())
	require.NoError(t, err)

	call, ok := findCall(repo.calls, "T1")
	require.True(t, ok, "Oracle-only machine should still be upserted")
	assert.Equal(t, "TXT", call.Area)
	assert.False(t, res.FinanceUsed)
	assert.True(t, res.OracleUsed)
}

func TestSync_BothUnavailableNoOp(t *testing.T) {
	repo := &fakeRepo{}
	finance := &fakeFinance{degraded: true}

	uc := machinesync.NewUsecase(repo, finance, nil)
	res, err := uc.Sync(context.Background())
	require.NoError(t, err)

	assert.Empty(t, repo.calls)
	assert.False(t, res.FinanceUsed)
	assert.False(t, res.OracleUsed)
}

func TestSync_CountsOutcomes(t *testing.T) {
	repo := &fakeRepo{outcomes: map[string]machinedomain.UpsertOutcome{
		"INS": machinedomain.OutcomeInserted,
		"UPD": machinedomain.OutcomeUpdated,
		"SKP": machinedomain.OutcomeSkipped,
	}}
	finance := &fakeFinance{machines: []*financev1.Machine{
		financeMachine("u1", "INS", true),
		financeMachine("u2", "UPD", true),
		financeMachine("u3", "SKP", true),
	}}

	uc := machinesync.NewUsecase(repo, finance, nil)
	res, err := uc.Sync(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, res.Inserted)
	assert.Equal(t, 1, res.Updated)
	assert.Equal(t, 1, res.Skipped)
}

func TestSync_RepoErrorPropagates(t *testing.T) {
	repo := &fakeRepo{err: errors.New("db down")}
	finance := &fakeFinance{machines: []*financev1.Machine{financeMachine("u1", "M1", true)}}

	uc := machinesync.NewUsecase(repo, finance, nil)
	_, err := uc.Sync(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}
