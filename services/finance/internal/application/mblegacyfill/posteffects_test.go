package mblegacyfill

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
)

func res(e Entity, id, col, result, before, fx string) RowResult {
	return RowResult{Row: Row{Entity: e, ID: id, Spec: FieldSpec{Entity: e, Column: col}}, Result: result, BeforeJSON: before, SideEffects: fx}
}

func TestSelectRecalcTargets(t *testing.T) {
	fx := `{"cascaded_spin_ids":["c1","c2"],"spin_changes":{"c1":{"before":{"mbs_dozing":"1.5","mbs_denier":"2"},"after":{"mbs_denier":"3"}},"c2":{"before":{"mbs_shade_code":"a"},"after":{"mbs_shade_code":"b"}}}}`
	rs := []RowResult{
		res(EntitySpin, "s1", "mbs_denier", ResultApplied, `{"mbs_denier":"2"}`, ""),
		res(EntitySpin, "s2", "mbs_dozing", ResultApplied, `{"mbs_dozing":"0.5"}`, ""),
		res(EntitySpin, "s3", "mbs_dozing", ResultSkipped, `{}`, ""),
		res(EntitySpin, "s4", "mbs_status", ResultApplied, `{}`, ""),
		res(EntityHead, "h1", "mbh_denier", ResultApplied, `{}`, fx),
		res(EntityHead, "h2", "mbh_status", ResultFailed, `{}`, fx),
	}
	got := SelectRecalcTargets(rs)
	ids := []string{}
	for _, g := range got {
		ids = append(ids, g.SpinID)
	}
	if strings.Join(ids, ",") != "c1,s1,s2" {
		t.Fatalf("targets=%v", ids)
	}
	if got[0].OldDozing == nil || *got[0].OldDozing != 1.5 {
		t.Errorf("c1 old dozing: %v", got[0].OldDozing)
	}
	if got[1].OldDozing != nil || got[2].OldDozing == nil || *got[2].OldDozing != 0.5 {
		t.Errorf("old dozing mapping wrong")
	}
}

type fakeRC struct{}

func (fakeRC) RecalcSpin(_ context.Context, t RecalcTarget, _ string) (RecalcOutcome, error) {
	if t.SpinID == "bad" {
		return RecalcOutcome{}, errors.New("boom")
	}
	return RecalcOutcome{Recalculated: 2}, nil
}

func TestRunPostEffectsNotFatal(t *testing.T) {
	outs := RunPostEffects(context.Background(), fakeRC{}, "a", []RecalcTarget{{SpinID: "bad"}, {SpinID: "ok"}})
	if len(outs) != 2 || outs[0].Result != RecalcFailed || outs[0].Reason != "boom" || outs[1].Result != RecalcOK || outs[1].Recalculated != 2 {
		t.Fatalf("%+v", outs)
	}
	var b strings.Builder
	if err := WriteRecalcCSV(&b, outs); err != nil || !strings.Contains(b.String(), "recalc_failed") {
		t.Fatal(err, b.String())
	}
	b.Reset()
	if err := WriteAffectedProductsCSV(&b, []AffectedProduct{{1, "X", "r"}}); err != nil || !strings.Contains(b.String(), "1,X,r") {
		t.Fatal(err, b.String())
	}
}

type fakeImpact struct{ total map[string]int }

func (f fakeImpact) ImpactBySpin(_ context.Context, code string, _ int) ([]mbdozing.ImpactRow, mbdozing.Totals, error) {
	rows := []mbdozing.ImpactRow{{ProductSysID: int64(len(code)), ProductCode: "P-" + code}}
	return rows, mbdozing.Totals{TotalAffected: f.total[code] + 1}, nil
}

type fakeOrion map[string]string

func (f fakeOrion) OrionCodeOf(_ context.Context, id string) (string, error) { return f[id], nil }

func TestParentAndChildBothChangedFlagsOverwrite(t *testing.T) {
	rs := []RowResult{
		res(EntitySpin, "p", "mbs_dozing", ResultApplied, `{"mbs_dozing":"1"}`, ""),
		res(EntitySpin, "c", "mbs_dozing", ResultApplied, `{"mbs_dozing":"2"}`, ""),
	}
	targets := SelectRecalcTargets(rs)
	if len(targets) != 2 || !targets[0].LegacyDozing["c"] || !targets[0].LegacyDozing["p"] {
		t.Fatalf("%+v", targets)
	}
	got := OverwrittenLegacy([]string{"c", "other"}, targets[0].LegacyDozing)
	if len(got) != 1 || got[0] != "c" {
		t.Fatalf("overwrite=%v", got)
	}
	var b strings.Builder
	_ = WriteRecalcCSV(&b, []RecalcOutcome{{SpinID: "p", Result: RecalcOK, OverwroteLegacy: got}})
	if !strings.Contains(b.String(), "recalc_overwrote_legacy") || !strings.Contains(b.String(), ",c\n") {
		t.Fatal(b.String())
	}
}

func TestAffectedCodesCoverChildrenOldOrionAndNonDenSpins(t *testing.T) {
	fx := `{"spin_changes":{"k":{"before":{"mbs_orion_item_code":"OLDK"},"after":{"mbs_orion_item_code":"NEWK"}}}}`
	rs := []RowResult{
		res(EntitySpin, "s1", "mbs_orion_item_code", ResultApplied, `{"mbs_orion_item_code":"OLD1"}`, ""),
		res(EntitySpin, "s2", "mbs_shade_code", ResultApplied, `{}`, ""),
		res(EntitySpin, "s3", "mbs_status", ResultApplied, `{}`, ""),
		res(EntityHead, "h", "mbh_orion_item_code", ResultApplied, `{}`, fx),
	}
	outs := []RecalcOutcome{{SpinID: "p", OrionCode: "PAR", ChildIDs: []string{"c1"}}}
	ro := fakeOrion{"s1": "NEW1", "s2": "SH2", "s3": "NOPE", "k": "NEWK", "c1": "CH1"}
	codes, err := AffectedOrionCodes(context.Background(), ro, rs, outs)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range codes {
		got = append(got, c.code)
	}
	if strings.Join(got, ",") != "CH1,NEW1,NEWK,OLD1,OLDK,PAR,SH2" {
		t.Fatalf("codes=%v", got)
	}
}

func TestCollectAffectedFlagsTruncation(t *testing.T) {
	outs := []RecalcOutcome{{SpinID: "p", OrionCode: "BIG"}}
	rows, err := CollectAffectedProducts(context.Background(), fakeImpact{total: map[string]int{"BIG": 500}}, fakeOrion{}, nil, outs)
	if err != nil || len(rows) != 2 || !strings.HasPrefix(rows[1].Reason, "TRUNCATED") {
		t.Fatal(err, rows)
	}
}
