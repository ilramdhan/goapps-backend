package shared

import "testing"

func TestQtyTXT(t *testing.T) {
	// Fixture lot qU04qB006, machine SM2: TOTAL=20, NORMAL=18, DG=2.
	// Std weights chosen: full=4.0 kg, unfull=2.0 kg.
	const (
		stdFull   = 4.0
		stdUnfull = 2.0
	)
	tests := []struct {
		name          string
		fullBobbins   int
		unfullBobbins int
		want          float64
	}{
		{name: "full only, all 20 full", fullBobbins: 20, unfullBobbins: 0, want: 80.0},
		{name: "unfull only, all 20 unfull", fullBobbins: 0, unfullBobbins: 20, want: 40.0},
		{name: "mixed 18 full 2 unfull", fullBobbins: 18, unfullBobbins: 2, want: 76.0},
		{name: "mixed 2 full 18 unfull", fullBobbins: 2, unfullBobbins: 18, want: 44.0},
		{name: "zero", fullBobbins: 0, unfullBobbins: 0, want: 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QtyTXT(tt.fullBobbins, tt.unfullBobbins, stdFull, stdUnfull)
			if got != tt.want {
				t.Errorf("QtyTXT(%d, %d, %g, %g) = %g, want %g",
					tt.fullBobbins, tt.unfullBobbins, stdFull, stdUnfull, got, tt.want)
			}
		})
	}
}

func TestIsSpgFullDoff(t *testing.T) {
	// SPG DOFF_OPTION is the inverse of TXT TRN_STS: 1=Full, 2=Unfull.
	if !IsSpgFullDoff(SpgDoffOptionFull) {
		t.Errorf("IsSpgFullDoff(1) = false, want true (DOFF_OPTION 1 = Full)")
	}
	if IsSpgFullDoff(SpgDoffOptionUnfull) {
		t.Errorf("IsSpgFullDoff(2) = true, want false (DOFF_OPTION 2 = Unfull)")
	}
	if IsSpgFullDoff(0) {
		t.Errorf("IsSpgFullDoff(0) = true, want false")
	}
}

func TestQtySPG(t *testing.T) {
	const (
		stdFull   = 5.0
		stdUnfull = 2.5
	)
	tests := []struct {
		name         string
		bobbins      int
		doffOption   int
		weightPerBob float64
		want         float64
	}{
		{name: "measured weight wins over std", bobbins: 10, doffOption: SpgDoffOptionFull, weightPerBob: 4.8, want: 48.0},
		{name: "measured weight, unfull option ignored", bobbins: 4, doffOption: SpgDoffOptionUnfull, weightPerBob: 3.0, want: 12.0},
		{name: "no measured weight, full doff uses std full", bobbins: 10, doffOption: SpgDoffOptionFull, weightPerBob: 0, want: 50.0},
		{name: "no measured weight, unfull doff uses std unfull", bobbins: 10, doffOption: SpgDoffOptionUnfull, weightPerBob: 0, want: 25.0},
		{name: "zero bobbins", bobbins: 0, doffOption: SpgDoffOptionFull, weightPerBob: 4.0, want: 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QtySPG(tt.bobbins, tt.doffOption, tt.weightPerBob, stdFull, stdUnfull)
			if got != tt.want {
				t.Errorf("QtySPG(%d, %d, %g, %g, %g) = %g, want %g",
					tt.bobbins, tt.doffOption, tt.weightPerBob, stdFull, stdUnfull, got, tt.want)
			}
		})
	}
}

func TestQtyTXTNormal(t *testing.T) {
	const (
		stdFull   = 4.0
		stdUnfull = 2.0
	)
	tests := []struct {
		name          string
		normalBobs    int
		relUnfullBobs int
		want          float64
	}{
		{name: "normal only 18", normalBobs: 18, relUnfullBobs: 0, want: 72.0},
		{name: "normal 18 plus rel-unfull 2", normalBobs: 18, relUnfullBobs: 2, want: 76.0},
		{name: "rel-unfull only", normalBobs: 0, relUnfullBobs: 5, want: 10.0},
		{name: "zero", normalBobs: 0, relUnfullBobs: 0, want: 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QtyTXTNormal(tt.normalBobs, tt.relUnfullBobs, stdFull, stdUnfull)
			if got != tt.want {
				t.Errorf("QtyTXTNormal(%d, %d, %g, %g) = %g, want %g",
					tt.normalBobs, tt.relUnfullBobs, stdFull, stdUnfull, got, tt.want)
			}
		})
	}
}
