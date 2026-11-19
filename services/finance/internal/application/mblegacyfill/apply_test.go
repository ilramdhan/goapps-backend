package mblegacyfill

import (
	"bytes"
	"strings"
	"testing"
)

func TestShadeNK(t *testing.T) {
	for in, want := range map[string]string{"AX-7": "AX7", " ax7 ": "AX7", "a x-7": "AX7", "": ""} {
		if got := ShadeNK(in); got != want {
			t.Errorf("ShadeNK(%q)=%q want %q", in, got, want)
		}
	}
}

func TestWriteResultCSV(t *testing.T) {
	var b bytes.Buffer
	err := WriteResultCSV(&b, []RowResult{{Row: Row{Entity: EntityHead, ID: "x", Field: "notes"}, Decision: "APPLY", Result: ResultApplied, BeforeJSON: `{"a":"1"}`}})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "decision,result,reason,before_json,after_json,side_effects_json") {
		t.Fatalf("unexpected csv: %q", b.String())
	}
}
