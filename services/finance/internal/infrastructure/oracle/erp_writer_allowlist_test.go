package oracle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// goldenAllowlist pins the exact allowlist (§S-T1). Changing, adding or
// removing any statement must fail this test and needs CODEOWNERS review.
var goldenAllowlist = map[StatementKey]string{
	KeyW1InsertBatch: "b9ccaa2b3d92a561fef841b710382a28204024a8c1dbe3eb10cfc2106bb0caaf",
	KeyW1InsertCost:  "4bc1d4c1b813d3df44c328ebad7d115a532435b8faee255ae24638f5b70627fb",
	KeyW2ValuateAdj:  "3b0d443a11a13da5908bfb44f099dca4646338a5c68d7d819c03900fef3c9e68",
	KeyW2ApproveAdj:  "711b467a7946f03aee241e91bb6064f06b0fb50403672ea2e8c89696abcde69e",
	KeyW2RestoreAdj:  "60bccdd456da0f75eee62de79db0cd1891415f445d2fa59873d1b97cc24e9450",
	KeyW2LockBatch:   "9b55df0b9424e103cd743bd8a8e726ed84963ad841733a40c66a427cddbbc9d6",
}

func TestAllowlist_GoldenKeysAndHashes(t *testing.T) {
	if len(allowlist) != 6 || len(goldenAllowlist) != 6 {
		t.Fatalf("allowlist must hold exactly 6 keys, has %d", len(allowlist))
	}
	for key, text := range allowlist {
		want, ok := goldenAllowlist[key]
		if !ok {
			t.Errorf("unexpected allowlist key %q", key)
			continue
		}
		sum := sha256.Sum256([]byte(text))
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("allowlist text for %q changed: sha256 %s, golden %s", key, got, want)
		}
	}
	for key := range goldenAllowlist {
		if _, ok := allowlist[key]; !ok {
			t.Errorf("allowlist key %q removed", key)
		}
	}
	if len(AllowlistKeys()) != 6 {
		t.Errorf("AllowlistKeys must return 6 keys")
	}
}

func TestAllowlist_TextShape(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)\b(DELETE|TRUNCATE|DROP|ALTER|CREATE|GRANT|REVOKE|MERGE|RENAME|UPDATE|COMMIT|ROLLBACK|EXECUTE)\b`)
	for key, text := range allowlist {
		if forbidden.MatchString(text) {
			t.Errorf("%s contains a forbidden verb: %s", key, text)
		}
		switch {
		case strings.HasPrefix(string(key), "W1_"):
			if !strings.HasPrefix(text, "INSERT INTO MGTDAT.CST_GOAPPS_STD_") {
				t.Errorf("%s must be an INSERT into CST_GOAPPS_STD_*: %s", key, text)
			}
		case strings.HasPrefix(string(key), "W2_"):
			if !strings.HasPrefix(text, "BEGIN MGTDAT.PKG_GOAPPS_ADJ.") || !strings.HasSuffix(text, "; END;") {
				t.Errorf("%s must be a single PKG_GOAPPS_ADJ call: %s", key, text)
			}
		default:
			t.Errorf("unexpected key family %s", key)
		}
	}
}

func TestAllowlist_InsertCostBindCountMatchesColumns(t *testing.T) {
	text, ok := Statement(KeyW1InsertCost)
	if !ok {
		t.Fatal("W1_INSERT_COST missing")
	}
	binds := regexp.MustCompile(`:\d+`).FindAllString(text, -1)
	if len(binds) != len(W1InsertCostColumns) {
		t.Fatalf("bind count %d != column count %d", len(binds), len(W1InsertCostColumns))
	}
	for _, col := range W1InsertCostColumns {
		if !strings.Contains(text, col) {
			t.Errorf("column %s missing from W1_INSERT_COST", col)
		}
	}
	if strings.Contains(text, "GSC_PUSHED_DT") {
		t.Error("GSC_PUSHED_DT must not be bound")
	}
}

func TestDisabledWriter_AllMethodsRefuse(t *testing.T) {
	w := NewDisabledWriter()
	ctx := context.Background()
	if _, err := w.InsertBatch(ctx, erpintegration.PushBatch{}, nil); !errors.Is(err, erpintegration.ErrWriterDisabled) {
		t.Errorf("InsertBatch: %v", err)
	}
	if _, err := w.ValuateAdj(ctx, 1); !errors.Is(err, erpintegration.ErrWriterDisabled) {
		t.Errorf("ValuateAdj: %v", err)
	}
	if _, err := w.ApproveAdj(ctx, 1, "u"); !errors.Is(err, erpintegration.ErrWriterDisabled) {
		t.Errorf("ApproveAdj: %v", err)
	}
	if _, err := w.RestoreAdj(ctx, 1); !errors.Is(err, erpintegration.ErrWriterDisabled) {
		t.Errorf("RestoreAdj: %v", err)
	}
	if _, err := w.LockBatch(ctx, 1); !errors.Is(err, erpintegration.ErrWriterDisabled) {
		t.Errorf("LockBatch: %v", err)
	}
}

func TestFakeWriter_HappyPathAndRerun(t *testing.T) {
	f := NewFakeWriter()
	ctx := context.Background()
	res, err := f.InsertBatch(ctx, erpintegration.PushBatch{BatchID: 7, Period: "202609"}, make([]erpintegration.PushRow, 3))
	if err != nil || res.CostRows != 3 || res.BatchRows != 1 {
		t.Fatalf("insert: %+v %v", res, err)
	}
	if _, err := f.ValuateAdj(ctx, 7); err != nil {
		t.Fatalf("valuate: %v", err)
	}
	s, err := f.ValuateAdj(ctx, 7)
	if err != nil || !strings.HasPrefix(s.Text, "ALREADY_DONE") {
		t.Fatalf("rerun valuate should be ALREADY_DONE: %+v %v", s, err)
	}
	if _, err := f.ApproveAdj(ctx, 7, "approver"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := f.LockBatch(ctx, 7); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if st, _ := f.Status(7); st != FakeStatusLocked {
		t.Fatalf("status %s", st)
	}
	if len(f.Calls()) != 5 || f.RowCount(7) != 3 {
		t.Fatalf("calls %d rows %d", len(f.Calls()), f.RowCount(7))
	}
	if _, err := f.InsertBatch(ctx, erpintegration.PushBatch{BatchID: 7}, nil); err == nil {
		t.Fatal("duplicate batch insert must fail (ORA-00001)")
	}
}

func TestFakeWriter_Injections(t *testing.T) {
	f := NewFakeWriter()
	ctx := context.Background()
	if _, err := f.InsertBatch(ctx, erpintegration.PushBatch{BatchID: 1, Period: "202609"}, nil); err != nil {
		t.Fatal(err)
	}
	f.InjectBusy(KeyW2ValuateAdj, 2)
	for i := 0; i < 2; i++ {
		if _, err := f.ValuateAdj(ctx, 1); !errors.Is(err, erpintegration.ErrOracleBusy) {
			t.Fatalf("attempt %d: expected busy, got %v", i, err)
		}
	}
	if _, err := f.ValuateAdj(ctx, 1); err != nil {
		t.Fatalf("third attempt should succeed: %v", err)
	}
	f.Inject(KeyW2ApproveAdj, erpintegration.ErrOutcomeUnknown)
	if _, err := f.ApproveAdj(ctx, 1, "u"); !errors.Is(err, erpintegration.ErrOutcomeUnknown) {
		t.Fatalf("expected unknown, got %v", err)
	}
	for _, code := range []int{20901, 20902, 20903, 20904, 20905, 20910, 20911, 20912} {
		f.InjectOraCode(KeyW2LockBatch, code)
		_, err := f.LockBatch(ctx, 1)
		var ae *erpintegration.OracleAppError
		if !errors.As(err, &ae) || ae.Code != code {
			t.Fatalf("expected ORA-%d, got %v", code, err)
		}
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.RestoreAdj(cctx, 1); !errors.Is(err, erpintegration.ErrOracleTimeout) {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestFakeWriter_FrozenPeriodAndWrongStatus(t *testing.T) {
	f := NewFakeWriter()
	ctx := context.Background()
	if _, err := f.InsertBatch(ctx, erpintegration.PushBatch{BatchID: 2, Period: "202608"}, nil); err != nil {
		t.Fatal(err)
	}
	var ae *erpintegration.OracleAppError
	if _, err := f.LockBatch(ctx, 2); !errors.As(err, &ae) || ae.Code != erpintegration.OraCode20903 {
		t.Fatalf("lock of PUSHED batch must fail, got %v", err)
	}
	f.FreezePeriod("202608")
	if _, err := f.ValuateAdj(ctx, 2); !errors.As(err, &ae) || ae.Code != erpintegration.OraPeriodFrozen {
		t.Fatalf("expected ORA-20901, got %v", err)
	}
	if _, err := f.ValuateAdj(ctx, 99); !errors.As(err, &ae) || ae.Code != erpintegration.OraCode20902 {
		t.Fatalf("unknown batch expected 20902, got %v", err)
	}
}
