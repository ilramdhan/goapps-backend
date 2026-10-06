package validation

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func nd(s string) decimal.NullDecimal { return decimal.NullDecimal{Decimal: dec(s), Valid: true} }

var now = time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

func key(item, grade, shade string) erpintegration.ErpKey {
	return erpintegration.ErpKey{ItemCode: item, GradeCode: grade, ShadeCode: shade}
}

// okRow builds a valued row with sane components.
func okRow(k erpintegration.ErpKey, src erpintegration.StdSource, std string, costID int64) erpintegration.StdRow {
	kind := erpintegration.ItemKindForCode(k.ItemCode)
	id, ver := costID, int32(1)
	r := erpintegration.StdRow{
		Key: k, Kind: kind, Source: src, Status: erpintegration.DeriveOK,
		Basis: erprule.BasisCost, AxCostSysID: &id, AxCostVersion: &ver,
		ChpConKg: nd("1"), ChpCost: nd("1.10000"), AxConvCost: nd("0.50000"),
		ConvCost: nd("0.50000"), StdCost: nd(std), AxCost: nd("1.60000"),
	}
	return r
}

func axRow(item, shade, std string, costID int64) erpintegration.StdRow {
	return okRow(key(item, "AX", shade), erpintegration.SourceAX, std, costID)
}

func derivedRow(item, grade, shade, std string, costID int64) erpintegration.StdRow {
	return okRow(key(item, grade, shade), erpintegration.SourceDerived, std, costID)
}

func mbRow(item, shade, std string, costID int64) erpintegration.StdRow {
	return okRow(key(item, "A", shade), erpintegration.SourceMB, std, costID)
}

func replicaFor(rows ...erpintegration.StdRow) *ReplicaSets {
	rs := &ReplicaSets{Items: map[string]struct{}{}, Shades: map[string]struct{}{}, Grades: map[string]struct{}{}}
	for _, r := range rows {
		rs.Items[r.Key.ItemCode] = struct{}{}
		rs.Shades[r.Key.ShadeCode] = struct{}{}
		rs.Grades[r.Key.GradeCode] = struct{}{}
	}
	return rs
}

func usdCosts(rows ...erpintegration.StdRow) map[int64]SourceCost {
	m := map[int64]SourceCost{}
	for _, r := range rows {
		if r.AxCostSysID != nil {
			m[*r.AxCostSysID] = SourceCost{Currency: "USD", CostPerUnit: dec("1.6")}
		}
	}
	return m
}

// cleanInput is a fully valid batch: 1 yarn combo (AX + derived B) and 1 MB.
func cleanInput(t *testing.T) Input {
	t.Helper()
	rows := []erpintegration.StdRow{
		axRow("POY100", "NL", "1.60000", 11),
		derivedRow("POY100", "B", "NL", "1.50000", 11),
		mbRow("CMB200", "RD", "3.00000", 22),
	}
	loaded := now.Add(-time.Hour)
	var posted int64
	pid1, pid2, c1, c2 := int64(1), int64(2), int64(11), int64(22)
	return Input{
		Period: "202608",
		Now:    now,
		Rows:   rows,
		Coverage: []erpintegration.CoverageLine{
			{Kind: erpintegration.ItemKindYarn, ItemCode: "POY100", ShadeCode: "NL", Status: erpintegration.CoverageOK, ProductSysID: &pid1, CostID: &c1},
			{Kind: erpintegration.ItemKindMB, ItemCode: "CMB200", ShadeCode: "RD", Status: erpintegration.CoverageOK, ProductSysID: &pid2, CostID: &c2},
		},
		Demand: []erpintegration.DemandLine{
			{TxnCode: "INVADJ", Kind: erpintegration.ItemKindYarn, ItemCode: "POY100", GradeCode: "AX", ShadeCode: "NL"},
			{TxnCode: "INVADJ", Kind: erpintegration.ItemKindYarn, ItemCode: "POY100", GradeCode: "B", ShadeCode: "NL"},
			{TxnCode: "MBINVADJ", Kind: erpintegration.ItemKindMB, ItemCode: "CMB200", GradeCode: "A", ShadeCode: "RD"},
		},
		PrevStd: map[erpintegration.ErpKey]decimal.Decimal{
			rows[0].Key: dec("1.55"), rows[1].Key: dec("1.45"), rows[2].Key: dec("3.1"),
		},
		Replica:        replicaFor(rows...),
		SourceCosts:    usdCosts(rows...),
		DemandLoadedAt: &loaded,
		PostedHeads:    &posted,
	}
}

func codes(fs []Finding) []erpintegration.IssueCode {
	out := make([]erpintegration.IssueCode, len(fs))
	for i, f := range fs {
		out[i] = f.Code
	}
	return out
}
