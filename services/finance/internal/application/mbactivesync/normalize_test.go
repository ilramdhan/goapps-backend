package mbactivesync

import "testing"

func TestNormShade(t *testing.T) {
	tests := []struct{ in, loose, exact string }{
		{"Y145T", "Y145T", "Y145T"},
		{" y-145-T ", "Y145T", "Y-145-T"},
		{"X 438 T", "X438T", "X 438 T"},
		{"", "", ""},
		{"   ", "", ""},
	}
	for _, tc := range tests {
		if got := NormShadeLoose(tc.in); got != tc.loose {
			t.Errorf("Loose(%q)=%q want %q", tc.in, got, tc.loose)
		}
		if got := NormShadeExact(tc.in); got != tc.exact {
			t.Errorf("Exact(%q)=%q want %q", tc.in, got, tc.exact)
		}
	}
	if NormShadeLoose("Y145T") != NormShadeLoose("Y-145-T") {
		t.Error("Y145T must equal Y-145-T loosely")
	}
}

func TestFamilyKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"MGT THRUST YELLOW TR-1359-B", "MGTTHRUSTYELLOWTR1359"},
		{"MGT FIRDAUS GREEN TR-1433-DL-D-01369-C", "MGTFIRDAUSGREENTR1433DL"},
		{"MGT HOMING BROWN TR-1428", "MGTHOMINGBROWNTR1428"},
		{"MGT HOMING BROWN TR-1428 CS", "MGTHOMINGBROWNTR1428"},
		{"mgt x tr-5 REV2", "MGTXTR5"},
		{"MGT X TR-5 N-D-00955-C", "MGTXTR5"},
		{"MGT X TR-5 ND-0012", "MGTXTR5"},
		{"MGT X TR-5 NEW", "MGTXTR5"},
		{"  ", ""},
		{"", ""},
	}
	for _, tc := range tests {
		if got := FamilyKey(tc.in); got != tc.want {
			t.Errorf("FamilyKey(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	if FamilyKey("MGT X TR-5") != FamilyKey("MGT X TR-5 CS") {
		t.Error("CS variant must share family")
	}
}
