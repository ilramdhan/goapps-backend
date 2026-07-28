// Package machinesync merges the sync-sourced machine master from finance
// (mst_machine via gRPC) and Oracle (TXTMACH), preserving PPC-local fields.
// See design §4.2 (anti-drift): machine rows are never hand-authored.
package machinesync

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	machinedomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
)

// FinanceMachineSource lists machines from the finance master (via gRPC).
type FinanceMachineSource interface {
	ListAllMachines(ctx context.Context) ([]*financev1.Machine, error)
	IsDegraded() bool
}

// OracleMachineSource lists machines from Oracle TXTMACH. A nil implementation
// (Oracle unavailable) is tolerated by the usecase.
type OracleMachineSource interface {
	ListTxtMachines(ctx context.Context) ([]oracle.TxtMachine, error)
}

// Result summarizes a sync run.
type Result struct {
	Inserted int
	Updated  int
	Skipped  int
	// FinanceUsed / OracleUsed report which sources contributed (false when a
	// source was unreachable and the run degraded).
	FinanceUsed bool
	OracleUsed  bool
}

// Usecase merges machine sources and upserts into ppc.machine.
type Usecase struct {
	repo    machinedomain.Repository
	finance FinanceMachineSource
	oracle  OracleMachineSource
	now     func() time.Time
}

// NewUsecase builds the machine-sync usecase. finance may be degraded and oracle
// may be nil; the usecase degrades gracefully in both cases.
func NewUsecase(repo machinedomain.Repository, finance FinanceMachineSource, oracleSrc OracleMachineSource) *Usecase {
	return &Usecase{
		repo:    repo,
		finance: finance,
		oracle:  oracleSrc,
		now:     time.Now,
	}
}

// Sync pulls both sources, merges them by machine number, and upserts each row.
// Resilience: if Oracle is unreachable it syncs finance-only; if finance is also
// unavailable it no-ops. It never returns an error for an unreachable source —
// only for a repository failure while writing resolved rows.
func (u *Usecase) Sync(ctx context.Context) (Result, error) {
	res := Result{}
	syncedAt := u.now().UTC()

	oracleByNo := u.loadOracle(ctx, &res)
	groupIDs := u.resolveGroups(ctx, oracleByNo)
	merged := u.mergeFinance(ctx, oracleByNo, groupIDs, syncedAt, &res)
	u.addOracleOnly(oracleByNo, groupIDs, merged, syncedAt)

	for _, src := range merged {
		outcome, err := u.repo.UpsertSourced(ctx, src)
		if err != nil {
			return res, err
		}
		switch outcome {
		case machinedomain.OutcomeInserted:
			res.Inserted++
		case machinedomain.OutcomeUpdated:
			res.Updated++
		case machinedomain.OutcomeSkipped:
			res.Skipped++
		}
	}
	return res, nil
}

// loadOracle reads TXTMACH keyed by uppercased machine number. Errors and nil
// sources degrade to an empty map (finance-only sync).
func (u *Usecase) loadOracle(ctx context.Context, res *Result) map[string]oracle.TxtMachine {
	byNo := make(map[string]oracle.TxtMachine)
	if u.oracle == nil {
		return byNo
	}
	rows, err := u.oracle.ListTxtMachines(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("machine sync: Oracle TXTMACH read failed, running finance-only")
		return byNo
	}
	for _, row := range rows {
		no := normalizeNo(row.MachNo)
		if no == "" {
			continue
		}
		byNo[no] = row
	}
	if len(rows) > 0 {
		res.OracleUsed = true
	}
	return byNo
}

// resolveGroups materializes every (MACH_GROUP, area) pair TXTMACH carries into
// machine_group and returns the resulting ids keyed by "AREA|GROUP". TXTMACH is
// the source of truth for the machine→group assignment (gap G2): the static seed
// list drifted from it, so the sync creates what is missing instead.
//
// A group whose department does not map to a PPC area is skipped — group_area is
// NOT NULL and CHECK-constrained to TXT/SPG/TWT. A failure to create one group
// is logged and skipped rather than failing the whole sync; the affected
// machines then keep whatever group they already had (COALESCE-preserve).
func (u *Usecase) resolveGroups(ctx context.Context, oracleByNo map[string]oracle.TxtMachine) map[string]int64 {
	ids := make(map[string]int64)
	for _, row := range oracleByNo {
		areaVal := deptToArea(row.Dept)
		name := strings.TrimSpace(row.Group)
		if areaVal == "" || name == "" {
			continue
		}
		key := groupKey(areaVal, name)
		if _, ok := ids[key]; ok {
			continue
		}
		id, err := u.repo.EnsureGroup(ctx, name, areaVal)
		if err != nil {
			log.Warn().Err(err).Str("group", name).Str("area", areaVal).
				Msg("machine sync: failed to ensure machine group, leaving affected machines unchanged")
			continue
		}
		ids[key] = id
	}
	return ids
}

// groupKey keys a machine group by its area and name, matching the
// (group_name, group_area) uniqueness of the table.
func groupKey(areaVal, name string) string {
	return areaVal + "|" + name
}

// sourcedFromOracle projects a TXTMACH row onto the source-owned fields of a
// SourcedMachine. Unresolvable fields are left zero so the repository's
// COALESCE-preserve keeps the stored value.
func sourcedFromOracle(row oracle.TxtMachine, groupIDs map[string]int64) (areaVal, orion, line string, groupID *int64) {
	areaVal = deptToArea(row.Dept)
	orion = strings.TrimSpace(row.OrionCode)
	line = strings.TrimSpace(row.Line)
	if id, ok := groupIDs[groupKey(areaVal, strings.TrimSpace(row.Group))]; ok {
		groupID = &id
	}
	return areaVal, orion, line, groupID
}

// mergeFinance builds sourced rows from finance machines, attaching the Oracle
// fields when the machine number matches. Degraded/failed finance yields nothing.
func (u *Usecase) mergeFinance(ctx context.Context, oracleByNo map[string]oracle.TxtMachine, groupIDs map[string]int64, syncedAt time.Time, res *Result) map[string]machinedomain.SourcedMachine {
	merged := make(map[string]machinedomain.SourcedMachine)
	if u.finance == nil || u.finance.IsDegraded() {
		return merged
	}
	machines, err := u.finance.ListAllMachines(ctx)
	if err != nil {
		return merged
	}
	res.FinanceUsed = true

	for _, m := range machines {
		no := normalizeNo(m.GetMachineCode())
		if no == "" {
			continue
		}
		// Zero values when Oracle has no match: those fields stay PPC-local.
		areaVal, orion, line, groupID := sourcedFromOracle(oracleByNo[no], groupIDs)
		src := machinedomain.SourcedMachine{
			MachineNo: no,
			Area:      areaVal,
			OrionCode: orion,
			Line:      line,
			GroupID:   groupID,
			IsActive:  m.GetIsActive(),
			SyncedAt:  syncedAt,
		}
		if mcID := m.GetMachineId(); mcID != "" {
			src.SourceMcID = &mcID
		}
		merged[no] = src
	}
	return merged
}

// addOracleOnly appends machines present only in Oracle (not in finance) so
// TXT-area machines are captured even when finance lacks them.
func (u *Usecase) addOracleOnly(oracleByNo map[string]oracle.TxtMachine, groupIDs map[string]int64, merged map[string]machinedomain.SourcedMachine, syncedAt time.Time) {
	for no, row := range oracleByNo {
		if _, ok := merged[no]; ok {
			continue
		}
		areaVal, orion, line, groupID := sourcedFromOracle(row, groupIDs)
		merged[no] = machinedomain.SourcedMachine{
			MachineNo: no,
			Area:      areaVal,
			OrionCode: orion,
			Line:      line,
			GroupID:   groupID,
			IsActive:  true,
			SyncedAt:  syncedAt,
		}
	}
}

// normalizeNo trims and uppercases a machine number for stable matching.
func normalizeNo(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// deptToArea maps an Oracle MACH_DEPT to a PPC area (TXT/SPG/TWT). Unknown
// departments yield an empty string (area then stays PPC-local).
func deptToArea(dept string) string {
	switch strings.ToUpper(strings.TrimSpace(dept)) {
	case "TXT":
		return "TXT"
	case "TWT":
		return "TWT"
	case "SPG":
		return "SPG"
	default:
		return ""
	}
}
