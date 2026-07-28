package changeover

import (
	"testing"
	"time"
)

// codes extracts the component codes from a detection result for assertions.
func codes(components []Component) []string {
	out := make([]string, len(components))
	for i, c := range components {
		out[i] = c.Code()
	}
	return out
}

// hasCode reports whether the code set contains want.
func hasCode(components []Component, want string) bool {
	for _, c := range components {
		if c.Code() == want {
			return true
		}
	}
	return false
}

func TestDetect_Components(t *testing.T) {
	base := Spec{Denier: 150, ColorFamily: "RED", ShadeDarkness: 5, FilamentCount: 48, TwistDir: "S", LotNo: "L1", ProductSysID: 100}

	tests := []struct {
		name    string
		in      DetectInput
		wantYes []string
		wantNo  []string
	}{
		{
			name:    "identical spec -> BASE only",
			in:      DetectInput{From: base, To: base},
			wantYes: []string{CompBase},
			wantNo:  []string{CompC1, CompC2, CompC3, CompC4, CompC5, CompC6, CompC7},
		},
		{
			name: "denier change -> C1",
			in: DetectInput{From: base, To: func() Spec {
				s := base
				s.Denier = 300
				s.ProductSysID = 200
				return s
			}()},
			wantYes: []string{CompBase, CompC1},
		},
		{
			name: "color family change -> C2",
			in: DetectInput{From: base, To: func() Spec {
				s := base
				s.ColorFamily = "blue"
				s.ProductSysID = 200
				return s
			}()},
			wantYes: []string{CompBase, CompC2},
		},
		{
			name: "shade dark->light -> C3",
			in: DetectInput{From: base, To: func() Spec {
				s := base
				s.ShadeDarkness = 2
				return s
			}()},
			wantYes: []string{CompBase, CompC3},
		},
		{
			name:    "shade light->dark -> no C3",
			in:      DetectInput{From: func() Spec { s := base; s.ShadeDarkness = 2; return s }(), To: base},
			wantYes: []string{CompBase},
			wantNo:  []string{CompC3},
		},
		{
			name: "filament change -> C4",
			in: DetectInput{From: base, To: func() Spec {
				s := base
				s.FilamentCount = 72
				s.ProductSysID = 200
				return s
			}()},
			wantYes: []string{CompBase, CompC4},
		},
		{
			name: "twist S<->Z -> C5",
			in: DetectInput{From: base, To: func() Spec {
				s := base
				s.TwistDir = "Z"
				s.ProductSysID = 200
				return s
			}()},
			wantYes: []string{CompBase, CompC5},
		},
		{
			name: "same product, different lot -> C6",
			in: DetectInput{From: base, To: func() Spec {
				s := base
				s.LotNo = "L2"
				return s
			}()},
			wantYes: []string{CompBase, CompC6},
		},
		{
			name: "product change suppresses C6",
			in: DetectInput{From: base, To: func() Spec {
				s := base
				s.Denier = 300
				s.LotNo = "L2"
				s.ProductSysID = 200
				return s
			}()},
			wantYes: []string{CompBase, CompC1},
			wantNo:  []string{CompC6},
		},
		{
			name:    "deep clean flag -> C7",
			in:      DetectInput{From: base, To: base, DeepCleanFlag: true},
			wantYes: []string{CompBase, CompC7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Detect(tt.in, nil)
			for _, code := range tt.wantYes {
				if !hasCode(got, code) {
					t.Errorf("Detect() missing %s; got %v", code, codes(got))
				}
			}
			for _, code := range tt.wantNo {
				if hasCode(got, code) {
					t.Errorf("Detect() unexpectedly has %s; got %v", code, codes(got))
				}
			}
		})
	}
}

func TestDetect_MultipleComponents(t *testing.T) {
	from := Spec{Denier: 150, ColorFamily: "RED", ShadeDarkness: 5, FilamentCount: 48, TwistDir: "S", ProductSysID: 1}
	to := Spec{Denier: 300, ColorFamily: "WHITE", ShadeDarkness: 1, FilamentCount: 72, TwistDir: "Z", ProductSysID: 2}
	got := Detect(DetectInput{From: from, To: to}, nil)

	// BASE + C1(denier) + C2(color) + C3(dark->light) + C4(filament) + C5(twist).
	for _, want := range []string{CompBase, CompC1, CompC2, CompC3, CompC4, CompC5} {
		if !hasCode(got, want) {
			t.Errorf("missing %s; got %v", want, codes(got))
		}
	}
}

func TestClassifyGroup(t *testing.T) {
	tests := []struct {
		dur  int32
		want string
	}{
		{30, GroupMinor},
		{59, GroupMinor},
		{60, GroupMedium},
		{120, GroupMedium},
		{121, GroupMajor},
		{240, GroupMajor},
		{241, GroupDeep},
		{500, GroupDeep},
	}
	for _, tt := range tests {
		if got := ClassifyGroup(tt.dur); got != tt.want {
			t.Errorf("ClassifyGroup(%d) = %s, want %s", tt.dur, got, tt.want)
		}
	}
}

func TestNewEvent_SumsAndClassifies(t *testing.T) {
	// BASE(30/8) + C1(60/20) + C2(90/30) = 180 min -> MAJOR; waste 58.
	components := Detect(DetectInput{
		From: Spec{Denier: 150, ColorFamily: "RED", ProductSysID: 1},
		To:   Spec{Denier: 300, ColorFamily: "BLUE", ProductSysID: 2},
	}, nil)
	ev, err := NewEvent(1, 2, 10, components, "test")
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	if ev.DurationEstimated() != 180 {
		t.Errorf("DurationEstimated = %d, want 180", ev.DurationEstimated())
	}
	if ev.WasteEstimated() != 58 {
		t.Errorf("WasteEstimated = %g, want 58", ev.WasteEstimated())
	}
	if ev.Group() != GroupMajor {
		t.Errorf("Group = %s, want MAJOR", ev.Group())
	}
	if ev.Status() != StatusPlanned {
		t.Errorf("Status = %s, want PLANNED", ev.Status())
	}
}

func TestNewEvent_Validation(t *testing.T) {
	comps := []Component{NewComponent(CompBase, 30, 8)}
	if _, err := NewEvent(0, 2, 10, comps, ""); err != ErrMissingWO {
		t.Errorf("missing from WO: got %v, want ErrMissingWO", err)
	}
	if _, err := NewEvent(1, 2, 0, comps, ""); err != ErrMissingMachine {
		t.Errorf("missing machine: got %v, want ErrMissingMachine", err)
	}
	if _, err := NewEvent(1, 2, 10, nil, ""); err != ErrNoComponents {
		t.Errorf("no components: got %v, want ErrNoComponents", err)
	}
}

func TestEvent_CompleteAndTransitions(t *testing.T) {
	comps := []Component{NewComponent(CompBase, 30, 8)}
	ev, _ := NewEvent(1, 2, 10, comps, "")

	if err := ev.Complete(-1, 5, time.Now()); err != ErrNegativeActual {
		t.Errorf("negative duration: got %v, want ErrNegativeActual", err)
	}
	if err := ev.Complete(45, 12, time.Now()); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if ev.Status() != StatusDone {
		t.Errorf("Status = %s, want DONE", ev.Status())
	}
	if *ev.DurationActual() != 45 || *ev.WasteActual() != 12 {
		t.Errorf("actuals = %d/%g, want 45/12", *ev.DurationActual(), *ev.WasteActual())
	}
	if err := ev.Complete(50, 10, time.Now()); err != ErrInvalidTransition {
		t.Errorf("re-complete: got %v, want ErrInvalidTransition", err)
	}
}
