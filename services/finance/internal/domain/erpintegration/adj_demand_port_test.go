package erpintegration

import (
	"errors"
	"testing"
)

func TestParseDemandSource(t *testing.T) {
	cases := map[string]DemandSource{"": DemandSourceView, " view ": DemandSourceView, "VIEW": DemandSourceView, "base_tables": DemandSourceBaseTables, "Base_Tables": DemandSourceBaseTables}
	for in, want := range cases {
		got, err := ParseDemandSource(in)
		if err != nil || got != want {
			t.Errorf("ParseDemandSource(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"tables", "base-tables", "oracle"} {
		if _, err := ParseDemandSource(in); !errors.Is(err, ErrInvalidDemandSource) {
			t.Errorf("ParseDemandSource(%q): expected ErrInvalidDemandSource, got %v", in, err)
		}
	}
}
