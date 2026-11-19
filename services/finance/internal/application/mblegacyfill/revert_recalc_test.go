package mblegacyfill

import "testing"

func TestSelectRevertRecalcTargets(t *testing.T) {
	fx := `{"spin_changes":{"s2":{"before":{"mbs_denier":"2"},"after":{"mbs_denier":"3","mbs_dozing":"1.5"}},` +
		`"s3":{"before":{"mbs_denier":"2"},"after":{"mbs_denier":"3"}},"s4":{"before":{"mbs_orion_item_code":"a"},"after":{"mbs_orion_item_code":"b"}}}}`
	res := []RevertResult{
		{Item: ResultRow{Entity: EntitySpin, ID: "s1", AfterJSON: `{"mbs_dozing":"2.5"}`}, Result: RevertReverted, Columns: []string{"mbs_dozing:restored"}},
		{Item: ResultRow{Entity: EntitySpin, ID: "s5"}, Result: RevertAlreadyReverted, Columns: []string{"mbs_denier:already_reverted"}},
		{Item: ResultRow{Entity: EntitySpin, ID: "s6"}, Result: RevertReverted, Columns: []string{"mbs_notes:restored"}},
		{Item: ResultRow{Entity: EntityHead, ID: "h1", SideEffects: fx}, Result: RevertChangedSince, Reason: ReasonCascadePartial,
			Cascade: `{"spin:s2":"restored","spin:s3":"changed_since_apply","spin:s4":"restored"}`},
		{Item: ResultRow{Entity: EntitySpin, ID: "s7"}, Result: RevertFailed},
	}
	got := SelectRevertRecalcTargets(res)
	if len(got) != 2 || got[0].SpinID != "s1" || got[1].SpinID != "s2" {
		t.Fatalf("%+v", got)
	}
	if got[0].OldDozing == nil || *got[0].OldDozing != 2.5 || got[1].OldDozing == nil || *got[1].OldDozing != 1.5 {
		t.Fatalf("old dozing %+v", got)
	}
	rr := RevertedRowResults(res)
	if len(rr) != 3 {
		t.Fatalf("want 3 reverted rows, got %d", len(rr))
	}
}
