package erpintegration

// recon_resolve.go settles STARTED / UNKNOWN Oracle call-log rows from the
// recon read-back (plan-06 P5-T6; design Part 3 §14 "Unknown outcome":
// "Read-only probe resolves it: W1 -> SELECT COUNT(*) FROM CST_GOAPPS_STD_COST
// WHERE GSC_BATCH_ID=:1 vs expected; W2 -> SELECT of the period-set items'
// FLEX_13 + GSB_STATUS. Never blind re-execute").
//
// The evidence is SELECT-only (already read by the recon step) and the only
// write is the PG call-log row. A row is settled only on unambiguous proof;
// anything else stays unresolved, is counted, alerts, and blocks RECONCILED.
// The batch status is never moved here: a W1 UNKNOWN already FAILED the
// batch (correction is a new batch) and a W2 UNKNOWN keeps it held until the
// operator re-runs the step (idempotent, ALREADY_DONE finalizes it).

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// CallResolution reports what recon did with one unresolved call-log row.
type CallResolution struct {
	CallID       string `json:"call_id"`
	StatementKey string `json:"statement_key"`
	From         string `json:"from"`
	To           string `json:"to"` // SUCCESS | FAILED | "" (unresolved)
	Evidence     string `json:"evidence"`
}

// callEvidence is the read-only evidence for settling a call.
type callEvidence struct {
	batchID     int64
	found       bool   // CST_GOAPPS_STD_BATCH row exists
	gsbStatus   string // GSB_STATUS
	costRows    int64  // CST_GOAPPS_STD_COST rows of the batch
	costMD5     string
	expectRows  int64 // PG OK std rows
	expectMD5   string
	adjItems    int64 // period ADJ items (guarded txns)
	adjStamped  int64 // of which FLEX_13 = batch
	digestError error
}

func newCallEvidence(b *domain.Batch, rb domain.OracleBatchReadBack, pg domain.StdDigest, adj []domain.AdjReadBackCombo) callEvidence {
	ev := callEvidence{
		batchID: b.ID(), found: rb.Found, gsbStatus: rb.Status,
		expectRows: pg.Totals.RowCount(), expectMD5: pg.RowsMD5,
	}
	if rb.Found {
		d, err := domain.ComputeStdDigest(rb.CostRows)
		ev.costRows, ev.costMD5, ev.digestError = int64(len(rb.CostRows)), d.RowsMD5, err
	}
	for _, c := range adj {
		ev.adjItems += c.Items
		ev.adjStamped += c.Stamped
	}
	return ev
}

// settle returns the target status ("" = unresolved) and the evidence text.
func (ev callEvidence) settle(key string) (domain.OracleCallStatus, string) {
	if key == domain.CallKeyW1InsertBatch || key == "W1_INSERT_COST" {
		return ev.settleW1()
	}
	if !ev.found {
		return "", "batch header not in Oracle; W2 outcome cannot be proven"
	}
	done, notDone := domain.ExpectedGsbStatusFor(key, ev.gsbStatus)
	if key == domain.CallKeyW2ValuateAdj || key == domain.CallKeyW2RestoreAdj {
		done, notDone = ev.crossCheckStamps(key, done, notDone)
	}
	switch {
	case done:
		return domain.OracleCallSuccess, fmt.Sprintf("GSB_STATUS=%s stamped=%d/%d", ev.gsbStatus, ev.adjStamped, ev.adjItems)
	case notDone:
		return domain.OracleCallFailed, fmt.Sprintf("GSB_STATUS=%s stamped=%d/%d", ev.gsbStatus, ev.adjStamped, ev.adjItems)
	default:
		return "", fmt.Sprintf("ambiguous: GSB_STATUS=%s stamped=%d/%d", ev.gsbStatus, ev.adjStamped, ev.adjItems)
	}
}

// settleW1: the header and the cost rows are one Oracle transaction, so no
// header proves nothing committed; a header with exactly the expected rows
// and md5 proves the push committed. Anything else is ambiguous.
func (ev callEvidence) settleW1() (domain.OracleCallStatus, string) {
	switch {
	case !ev.found:
		return domain.OracleCallFailed, "no CST_GOAPPS_STD_BATCH row for the batch"
	case ev.digestError == nil && ev.costRows == ev.expectRows && ev.costMD5 == ev.expectMD5:
		return domain.OracleCallSuccess, fmt.Sprintf("header + %d cost rows, md5 equal", ev.costRows)
	default:
		return "", fmt.Sprintf("header present but %d/%d cost rows or md5 differs", ev.costRows, ev.expectRows)
	}
}

// crossCheckStamps demotes a GSB_STATUS verdict the FLEX_13 stamps
// contradict (VALUATE stamps the items, RESTORE removes the stamps).
func (ev callEvidence) crossCheckStamps(key string, done, notDone bool) (bool, bool) {
	stamped := ev.adjStamped > 0
	if key == domain.CallKeyW2RestoreAdj {
		stamped = !stamped
	}
	return done && stamped, notDone && !stamped
}

// resolveCalls settles every unresolved call of the batch and records the
// outcome on the summary. A call that stays unresolved alerts.
func (s *ReconStep) resolveCalls(ctx context.Context, batchID int64, ev callEvidence, sum *ReconSummary) error {
	recs, err := s.resolver.ListUnresolved(ctx, batchID)
	if err != nil {
		return fmt.Errorf("recon list unresolved calls: %w", err)
	}
	for _, rec := range recs {
		to, why := ev.settle(rec.StatementKey)
		res := CallResolution{CallID: rec.CallID, StatementKey: rec.StatementKey, From: string(rec.Status), Evidence: why}
		if to == "" {
			sum.Unresolved++
			log.Error().Int64("batch_id", batchID).Str("call_id", rec.CallID).Str("key", rec.StatementKey).
				Str("evidence", why).Msg("ALERT erp recon: Oracle call outcome still unresolved; manual review")
			sum.Calls = append(sum.Calls, res)
			continue
		}
		note, err := json.Marshal(map[string]string{"by": StepRecon, "evidence": why})
		if err != nil {
			return fmt.Errorf("recon encode resolution: %w", err)
		}
		if err := s.resolver.Resolve(ctx, rec.CallID, to, note); err != nil {
			return fmt.Errorf("recon resolve call %s: %w", rec.CallID, err)
		}
		res.To = string(to)
		sum.Calls = append(sum.Calls, res)
	}
	return nil
}
