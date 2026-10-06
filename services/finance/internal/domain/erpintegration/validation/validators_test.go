package validation

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

func runOne(v Validator, in Input) []Finding { return v.Validate(&in) }

func TestCleanInput_NoFindingsExceptNone(t *testing.T) {
	res := Validate(cleanInput(t))
	assert.Empty(t, res.Findings, "%+v", res.Findings)
	assert.False(t, res.HasErrors())
	assert.False(t, res.NeedsAck())
	assert.Equal(t, "", res.WarningSetHash())
}

func TestV01(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V01(), in))

	in.Rows[0].StdCost = nd("0")
	in.Rows[2].StdCost = decimal.NullDecimal{}
	in.Rows[1].StdCost = nd("-1") // derived: V-02, not V-01
	fs := runOne(V01(), in)
	require.Len(t, fs, 2)
	assert.Equal(t, SeverityError, fs[0].Severity)
	assert.Equal(t, ScopeRow, fs[0].Scope)
	assert.Contains(t, fs[0].Message, "GOAPPS_AX")
	assert.Contains(t, fs[1].Message, "NULL")

	in.Rows[0].Status = erpintegration.DeriveNoRule // non-OK rows skipped
	assert.Len(t, runOne(V01(), in), 1)
}

func TestV02(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V02(), in))
	in.Rows[1].StdCost = nd("-0.2")
	in.Rows[0].StdCost = nd("0") // AX: not V-02
	fs := runOne(V02(), in)
	require.Len(t, fs, 1)
	assert.Equal(t, in.Rows[1].Key, fs[0].Key)
	in.Rows[1].Status = erpintegration.DeriveNoSellPrice
	assert.Empty(t, runOne(V02(), in))
}

func TestV03(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V03(), in))

	in.Rows[0].ChpCost = nd("999.99999") // boundary passes
	in.Rows[0].ValueLoss = nd("-999.99999")
	assert.Empty(t, runOne(V03(), in))

	in.Rows[0].ChpCost = nd("1000.00000")
	in.Rows[1].ValueLoss = nd("-1000")
	in.Rows[2].ChpConKg = nd("0")
	fs := runOne(V03(), in)
	require.Len(t, fs, 3)
	assert.Contains(t, fs[0].Message, "chp_cost")
	assert.Contains(t, fs[1].Message, "value_loss")
	assert.Contains(t, fs[2].Message, "chp_con_kg")

	// OK row with NULL kg fails; non-OK row with NULL kg is skipped.
	in2 := cleanInput(t)
	in2.Rows[0].ChpConKg = decimal.NullDecimal{}
	in2.Rows[1].ChpConKg = decimal.NullDecimal{}
	in2.Rows[1].Status = erpintegration.DeriveNoAX
	require.Len(t, runOne(V03(), in2), 1)

	// INVALID row from Derive: its V-03 issue is carried.
	in3 := cleanInput(t)
	r := in3.Rows[0]
	r.Status = erpintegration.DeriveInvalid
	r.Issues = []erpintegration.Issue{{Key: r.Key, Code: erpintegration.IssueV03, Severity: erpintegration.SeverityError, Message: "INVALID: std_cost 1000 is outside"}}
	r.StdCost = decimal.NullDecimal{}
	in3.Rows[0] = r
	fs = runOne(V03(), in3)
	require.Len(t, fs, 1)
	assert.Contains(t, fs[0].Message, "INVALID: std_cost")
}

func TestV04(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V04(), in))
	in.Coverage[0].Status = erpintegration.CoverageDupMapping
	in.Coverage[0].Reason = "V-04 ambiguous: 2 active AX products"
	in.Coverage = append(in.Coverage, erpintegration.CoverageLine{Kind: erpintegration.ItemKindYarn, ItemCode: "PTY9", ShadeCode: "X", Status: erpintegration.CoverageDupMapping})
	fs := runOne(V04(), in)
	require.Len(t, fs, 2)
	assert.Equal(t, ScopeCoverage, fs[0].Scope)
	assert.Equal(t, "V-04 ambiguous: 2 active AX products", fs[0].Message)
	assert.Contains(t, fs[1].Message, "PTY9/X")
	assert.Equal(t, "", fs[1].Key.GradeCode)
}

func TestV05(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V05(), in))

	k0, k1, k2 := in.Rows[0].Key, in.Rows[1].Key, in.Rows[2].Key
	in.PrevStd[k0] = dec("1.33334") // 1.6/1.33334-1 = 19.99%  → pass
	in.PrevStd[k1] = dec("1.0")     // +50% → warning
	delete(in.PrevStd, k2)          // first-seen → info
	fs := runOne(V05(), in)
	require.Len(t, fs, 2)
	assert.Equal(t, SeverityWarning, fs[0].Severity)
	assert.Contains(t, fs[0].Message, "50.00%")
	assert.True(t, fs[0].RequiresAck())
	assert.Equal(t, SeverityInfo, fs[1].Severity)
	assert.False(t, fs[1].RequiresAck())

	// exactly 20% passes; -25% warns.
	in2 := cleanInput(t)
	in2.PrevStd[k0] = dec("2.0")   // -20% exactly
	in2.PrevStd[k1] = dec("2.0")   // -25%
	in2.PrevStd[k2] = decimal.Zero // prev 0, cur 3 → unbounded warning
	fs = runOne(V05(), in2)
	require.Len(t, fs, 2)
	assert.Equal(t, k1, fs[0].Key)
	assert.Contains(t, fs[1].Message, "unbounded")

	// prev 0 and cur 0: nothing. non-OK row skipped.
	in3 := cleanInput(t)
	in3.Rows[2].StdCost = nd("0")
	in3.PrevStd[k2] = decimal.Zero
	in3.Rows[0].Status = erpintegration.DeriveNoAX
	delete(in3.PrevStd, k0)
	assert.Empty(t, runOne(V05(), in3))
}

func TestV06(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V06(), in))

	delete(in.Replica.Items, "CMB200")
	delete(in.Replica.Grades, "B")
	delete(in.Replica.Shades, "NL")
	fs := runOne(V06(), in)
	// row0: shade; row1: shade + grade; row2: item
	require.Len(t, fs, 4)
	all := ""
	for _, f := range fs {
		all += f.Message + "|"
	}
	assert.Contains(t, all, "cost_erp_item")
	assert.Contains(t, all, "cost_erp_grade")
	assert.Contains(t, all, "cost_erp_shade")

	in2 := cleanInput(t)
	in2.Replica = nil
	fs = runOne(V06(), in2)
	require.Len(t, fs, 1)
	assert.Equal(t, ScopeBatch, fs[0].Scope)

	in3 := cleanInput(t)
	long := "POY1234567890"
	in3.Rows[0].Key.ItemCode = long
	in3.Replica.Items[long] = struct{}{}
	fs = runOne(V06(), in3)
	require.Len(t, fs, 1)
	assert.Contains(t, fs[0].Message, "push-safe")

	// Derive-attached V-06 carried when nothing recomputed.
	in4 := cleanInput(t)
	in4.Rows[0].Issues = []erpintegration.Issue{{Code: erpintegration.IssueV06, Severity: erpintegration.SeverityError, Message: "INVALID: invalid item kind"}}
	fs = runOne(V06(), in4)
	require.Len(t, fs, 1)
	assert.Contains(t, fs[0].Message, "invalid item kind")
}

func TestV07Row(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V07(), in))
	in.Rows[0].StdCost = nd("20") // boundary passes
	assert.Empty(t, runOne(V07(), in))
	in.Rows[0].StdCost = nd("20.00001")
	in.Rows[1].StdCost = nd("0")
	in.Rows[2].StdCost = nd("25") // MB exempt
	fs := runOne(V07(), in)
	require.Len(t, fs, 2)
	assert.Contains(t, fs[0].Message, "241441")

	in2 := cleanInput(t)
	in2.Rows[0].Key.ItemCode = "XYZ100" // not guarded
	in2.Rows[0].StdCost = nd("50")
	in2.Rows[1].Status = erpintegration.DeriveNoRule
	in2.Rows[1].StdCost = decimal.NullDecimal{}
	assert.Empty(t, runOne(V07(), in2))
}

func TestV07Guarded(t *testing.T) {
	for _, c := range []string{"POY1", "pty1", " ACY1", "ITY", "MMK9", "TTY9", "HOY9"} {
		assert.True(t, IsV07Guarded(c), c)
	}
	for _, c := range []string{"CMB1", "XYZ", "", "PO"} {
		assert.False(t, IsV07Guarded(c), c)
	}
	assert.True(t, V07RateOK("CMB1", decimal.NullDecimal{}))
	assert.False(t, V07RateOK("POY1", decimal.NullDecimal{}))
	assert.False(t, V07RateOK("POY1", nd("-1")))
	assert.True(t, V07RateOK("POY1", nd("0.00001")))
}

func TestV08(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V08(), in))

	in.Rows[0].Status = erpintegration.DeriveNoAX
	in.Rows[0].Issues = []erpintegration.Issue{{Code: erpintegration.IssueV08, Severity: erpintegration.SeverityError, Message: "NO_AX: no active approved AX cost"}}
	in.Rows[1].Status = erpintegration.DeriveNoGradeGroup // no derive issue: default message
	in.Rows[2].Status = erpintegration.DeriveInvalid      // handled by V-03/V-06
	fs := runOne(V08(), in)
	require.Len(t, fs, 2)
	assert.Equal(t, "NO_AX: no active approved AX cost", fs[0].Message)
	assert.Contains(t, fs[1].Message, "NO_GRADE_GROUP")

	// V-08w: derived row with non POY/PTY/ITY prefix.
	in2 := cleanInput(t)
	in2.Rows[1].Key.ItemCode = "ACY100"
	fs = runOne(V08(), in2)
	require.Len(t, fs, 1)
	assert.Equal(t, CodeV08w, fs[0].Code)
	assert.Equal(t, SeverityWarning, fs[0].Severity)
	// AX row with ACY prefix: no warning (no rule lookup).
	in2.Rows[0].Key.ItemCode = "ACY100"
	assert.Len(t, runOne(V08(), in2), 1)
}

func TestV09(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V09(), in))

	// No policy: strict USD.
	in.SourceCosts[11] = SourceCost{Currency: "IDR", CostPerUnit: dec("1.6")}
	fs := runOne(V09(), in)
	require.Len(t, fs, 2) // rows 0 and 1 share cost 11
	assert.Contains(t, fs[0].Message, `"IDR"`)

	// Interim policy accepts IDR non-outlier.
	policy := func(_, _ string, c SourceCost) bool {
		return c.Currency == "USD" || (c.Currency == "IDR" && c.CostPerUnit.LessThan(dec("20")))
	}
	in.CurrencyPolicy = policy
	assert.Empty(t, runOne(V09(), in))
	in.SourceCosts[11] = SourceCost{Currency: "IDR", CostPerUnit: dec("15000")}
	assert.Len(t, runOne(V09(), in), 2)

	// Relabel applied: strict even with a policy.
	in.SourceCosts[11] = SourceCost{Currency: "IDR", CostPerUnit: dec("1.6")}
	in.CurrencyRelabelApplied = true
	assert.Len(t, runOne(V09(), in), 2)

	// Missing source cost fails closed; row without cost id skipped.
	in2 := cleanInput(t)
	delete(in2.SourceCosts, 22)
	in2.Rows[0].AxCostSysID = nil
	fs = runOne(V09(), in2)
	require.Len(t, fs, 1)
	assert.Contains(t, fs[0].Message, "not provided")
}

func TestV10(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V10(), in))

	edge := now.Add(-DefaultDemandMaxAge)
	in.DemandLoadedAt = &edge
	assert.Empty(t, runOne(V10(), in), "exactly 24h passes")

	stale := now.Add(-DefaultDemandMaxAge - time.Second)
	posted := int64(3)
	in.DemandLoadedAt = &stale
	in.PostedHeads = &posted
	fs := runOne(V10(), in)
	require.Len(t, fs, 2)
	assert.Equal(t, ScopeBatch, fs[0].Scope)
	assert.Contains(t, fs[0].Message, "older than")
	assert.Contains(t, fs[1].Message, "3 ADJ head(s)")

	in.DemandMaxAge = 48 * time.Hour
	assert.Len(t, runOne(V10(), in), 1)

	in2 := cleanInput(t)
	in2.DemandLoadedAt, in2.PostedHeads = nil, nil
	fs = runOne(V10(), in2)
	require.Len(t, fs, 2)
	assert.Contains(t, fs[0].Message, "never been loaded")
	assert.Contains(t, fs[1].Message, "probe was not run")
}

func TestV11(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V11(), in))

	in.Coverage[1].Status = erpintegration.CoverageNotApproved
	in.Coverage[1].Reason = "ACTUAL cost 22 is VERIFIED"
	in.Demand = append(in.Demand, erpintegration.DemandLine{TxnCode: "INVADJ", ItemCode: "PTY5", ShadeCode: "Z"})
	fs := runOne(V11(), in)
	require.Len(t, fs, 2)
	assert.Contains(t, fs[0].Message, "NOT_APPROVED (ACTUAL cost 22 is VERIFIED)")
	assert.Contains(t, fs[1].Message, "no coverage line")

	// DUP_MAPPING (V-04) and V-12 INVALID are not repeated; other INVALID is.
	in2 := cleanInput(t)
	in2.Coverage[0].Status = erpintegration.CoverageDupMapping
	in2.Coverage[1].Status = erpintegration.CoverageInvalid
	in2.Coverage[1].Reason = "V-12: CMB item linked to non-MB"
	assert.Empty(t, runOne(V11(), in2))
	in2.Coverage[1].Reason = "other"
	fs = runOne(V11(), in2)
	require.Len(t, fs, 1)
	assert.Contains(t, fs[0].Message, "INVALID (other)")

	in3 := cleanInput(t)
	in3.Demand = nil
	fs = runOne(V11(), in3)
	require.Len(t, fs, 1)
	assert.Contains(t, fs[0].Message, "no ADJ demand")
}

func TestV12(t *testing.T) {
	in := cleanInput(t)
	assert.Empty(t, runOne(V12(), in))

	in.Coverage[0].Status = erpintegration.CoverageInvalid
	in.Coverage[0].Reason = "V-12: yarn item POY100 linked to MB product 1"
	in.Coverage[1].Kind = erpintegration.ItemKindYarn // CMB coverage as yarn
	fs := runOne(V12(), in)
	require.Len(t, fs, 2)
	assert.Equal(t, in.Coverage[0].Reason, fs[0].Message)
	assert.Contains(t, fs[1].Message, "does not match item code")

	in2 := cleanInput(t)
	in2.Rows[0].Kind = erpintegration.ItemKindMB // yarn item flagged MB
	in2.Rows[1].Source = erpintegration.SourceMB // yarn row with MB source
	in2.Rows[2].Source = erpintegration.SourceAX // MB row with AX source
	fs = runOne(V12(), in2)
	require.Len(t, fs, 3)
	assert.Contains(t, fs[0].Message, "row kind MB")
	assert.Contains(t, fs[1].Message, "source GOAPPS_MB")
	assert.Contains(t, fs[2].Message, "source GOAPPS_AX")
}

func TestQuoteAndStdText(t *testing.T) {
	assert.Equal(t, `"x"`, quote("x"))
	assert.True(t, strings.HasPrefix(stdText(erpintegration.StdRow{}), "NULL"))
}
