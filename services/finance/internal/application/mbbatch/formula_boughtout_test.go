package mbbatch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
)

// boughtoutLoader reuses noSharingLoader but feeds the migration 000613 expressions
// (IS_BOUGHTOUT guard + IS_BOUGHTOUT input edge) and a configurable IS_BOUGHTOUT flag.
type boughtoutLoader struct {
	*noSharingLoader
	boughtout float64
}

func (l *boughtoutLoader) LoadCAPP(ctx context.Context, ids []int64) (map[int64]map[string]float64, error) {
	out, err := l.noSharingLoader.LoadCAPP(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, m := range out {
		m["IS_BOUGHTOUT"] = l.boughtout
	}
	return out, nil
}

func (l *boughtoutLoader) LoadFormulas(ctx context.Context, ids []int64) (map[int64][]costcalc.Formula, error) {
	out, err := l.noSharingLoader.LoadFormulas(ctx, ids)
	if err != nil {
		return nil, err
	}
	// 000613 expressions, keyed by formula code.
	exprs := map[string]string{
		FormulaCodeWasteVal:  "IS_BOUGHTOUT == 1 ? 0 : ((MB_RM_COST / (1 - MB_WASTE/100)) * (MB_WASTE/100))",
		FormulaCodeFixedCost: "IS_BOUGHTOUT == 1 ? 0 : (MB_NET_PROD > 0 ? MACHINE_MB_FIXED_TOTAL / MB_NET_PROD : 0)",
		FormulaCodeOthers:    "IS_BOUGHTOUT == 1 ? 0 : (((MB_RM_COST + MB_WASTE_VAL + MB_FIXED_TOTAL) * ((MB_QUALITY_LOSS + MB_DEV_EXPENSE)/100)) + MB_PACKING)",
		FormulaCodeConvCost:  "IS_BOUGHTOUT == 1 ? 0 : ((MB_NO_PROCESS * MB_FIXED_TOTAL) + MB_COST_OTHERS + MB_WASTE_VAL)",
	}
	for id, fs := range out {
		nf := make([]costcalc.Formula, len(fs))
		copy(nf, fs)
		for i := range nf {
			if e, ok := exprs[nf[i].FormulaCode]; ok {
				nf[i].Expression = e
				nf[i].InputParamCodes = append(append([]string{}, nf[i].InputParamCodes...), "IS_BOUGHTOUT")
			}
		}
		out[id] = nf
	}
	return out, nil
}

func runBoughtout(t *testing.T, boughtout float64) map[string]float64 {
	t.Helper()
	const productID = int64(70003)
	loader := &boughtoutLoader{
		noSharingLoader: &noSharingLoader{productID: productID, rmCostFor: map[string]float64{"ACTUAL": 100, "FORECAST": 100, "SELLING": 100}},
		boughtout:       boughtout,
	}
	writer := &fakeResultWriter{}
	svc := &Service{loader: loader, resultWriter: writer, evalCache: evaluator.NewCache()}
	cand := MBHeadCandidate{MBHID: "mbh-bo", Code: "MB-BO", CostProductID: productID}
	_, _, err := svc.computeAndPersist(context.Background(), nil, cand, "202609", 1, newBatchCosts([]MBHeadCandidate{cand}))
	require.NoError(t, err)
	require.NotEmpty(t, writer.got)
	var snap map[string]float64
	require.NoError(t, json.Unmarshal(writer.got[0].ParamSnapshot(), &snap))
	return snap
}

func TestBoughtout_ConversionCostsZeroAndFinalIsRMCost(t *testing.T) {
	s := runBoughtout(t, 1)
	for _, code := range []string{ResultParamWasteVal, ResultParamFixedCost, ResultParamOthers, ResultParamConvCost} {
		require.Zero(t, s[code], code)
	}
	require.InDelta(t, 100, s[ResultParamFinalCost], 1e-9)
	require.InDelta(t, s[ResultParamRMCost], s[ResultParamFinalCost], 1e-9)
}

func TestNotBoughtout_MatchesOld000452Values(t *testing.T) {
	s := runBoughtout(t, 0)
	// RM=100, waste=10%, net=90, machine fixed=900, QL+DEV=3%, packing=5, no_process=1.
	waste := (100.0 / 0.9) * 0.1
	fixed := 900.0 / 90.0
	others := (100+waste+fixed)*0.03 + 5
	conv := fixed + others + waste
	require.InDelta(t, waste, s[ResultParamWasteVal], 1e-6)
	require.InDelta(t, fixed, s[ResultParamFixedCost], 1e-6)
	require.InDelta(t, others, s[ResultParamOthers], 1e-6)
	require.InDelta(t, conv, s[ResultParamConvCost], 1e-6)
	require.InDelta(t, 100+conv, s[ResultParamFinalCost], 1e-6)
}
