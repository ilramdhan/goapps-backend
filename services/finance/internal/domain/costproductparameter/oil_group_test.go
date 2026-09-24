package costproductparameter_test

import (
	"errors"
	"strings"
	"testing"

	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

func TestOilGroupRule_Validate(t *testing.T) {
	pty := &cpp.OilGroupRule{TypeCode: "PTY", OilClass: "PTY", Allowed: []string{"202006101"}, Default: "202006101"}
	cases := []struct {
		name    string
		rule    *cpp.OilGroupRule
		code    string
		wantErr bool
	}{
		{"nil rule allows anything", nil, "X", false},
		{"allowed code", pty, "202006101", false},
		{"allowed code trimmed", pty, "  202006101 ", false},
		{"empty code uses default", pty, "", false},
		{"disallowed code", pty, "202006077", true},
		{"empty allowed set rejects", &cpp.OilGroupRule{TypeCode: "POY", OilClass: "POY"}, "202006077", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate(tc.code)
			if tc.wantErr {
				if !errors.Is(err, cpp.ErrOilGroupNotAllowed) {
					t.Fatalf("want ErrOilGroupNotAllowed, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestOilGroupRule_ValidateMessage(t *testing.T) {
	r := &cpp.OilGroupRule{TypeCode: "PTY", Allowed: []string{"202006101"}}
	err := r.Validate("202006077")
	want := `OIL_NAME "202006077" is not allowed for product type PTY; allowed: 202006101`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("message = %v, want contains %q", err, want)
	}
}
