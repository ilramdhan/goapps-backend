package changeover

import "strings"

// ComponentDefault holds the default duration (minutes) and waste (kg) for one
// changeover component. Source: PRD page 6 table.
type ComponentDefault struct {
	DurationMin int32
	WasteKg     float64
}

// DefaultComponentConfig is the seed C1..C7 + BASE default duration/waste table
// from PRD page 6. A future master table can override these per machine group.
var DefaultComponentConfig = map[string]ComponentDefault{
	CompBase: {DurationMin: 30, WasteKg: 8},
	CompC1:   {DurationMin: 60, WasteKg: 20},
	CompC2:   {DurationMin: 90, WasteKg: 30},
	CompC3:   {DurationMin: 60, WasteKg: 25},
	CompC4:   {DurationMin: 45, WasteKg: 15},
	CompC5:   {DurationMin: 30, WasteKg: 10},
	CompC6:   {DurationMin: 15, WasteKg: 5},
	CompC7:   {DurationMin: 120, WasteKg: 40},
}

// Spec is the subset of a product/WO specification relevant to changeover
// detection. Values are compared between the outgoing (from) and incoming (to)
// WO. Empty/zero fields are treated as "unknown" and never trigger a component.
type Spec struct {
	Denier        float64
	ColorFamily   string
	ShadeDarkness int // ordinal 1..N, higher = darker; 0 = unknown
	FilamentCount int
	TwistDir      string // "S" / "Z"
	LotNo         string
	ProductSysID  int64
}

// DetectInput carries the from/to specs plus manual flags for detection.
type DetectInput struct {
	From          Spec
	To            Spec
	DeepCleanFlag bool // C7 is manual-only
}

// Detect returns the active changeover components (BASE always included) for a
// transition, using the supplied default config. Detection rules (PRD page 6):
//
//	C1 denier change       — different denier
//	C2 color family change — different color family
//	C3 shade direction     — dark -> light (to lighter than from)
//	C4 filament count      — different filament count
//	C5 twist direction     — S <-> Z change
//	C6 lot change          — same product, different lot
//	C7 deep clean          — manual flag only
//
// C6 is suppressed when a product-level change (C1..C4) is already present — a
// product change implies a new lot, so it is not counted twice.
func Detect(in DetectInput, cfg map[string]ComponentDefault) []Component {
	if cfg == nil {
		cfg = DefaultComponentConfig
	}
	active := []string{CompBase}

	productChanged := false
	if denierChanged(in.From.Denier, in.To.Denier) {
		active = append(active, CompC1)
		productChanged = true
	}
	if colorFamilyChanged(in.From.ColorFamily, in.To.ColorFamily) {
		active = append(active, CompC2)
		productChanged = true
	}
	if shadeGoesDarkToLight(in.From.ShadeDarkness, in.To.ShadeDarkness) {
		active = append(active, CompC3)
	}
	if filamentChanged(in.From.FilamentCount, in.To.FilamentCount) {
		active = append(active, CompC4)
		productChanged = true
	}
	if twistChanged(in.From.TwistDir, in.To.TwistDir) {
		active = append(active, CompC5)
		productChanged = true
	}
	if !productChanged && lotChanged(in.From.LotNo, in.To.LotNo, in.From.ProductSysID, in.To.ProductSysID) {
		active = append(active, CompC6)
	}
	if in.DeepCleanFlag {
		active = append(active, CompC7)
	}

	return buildComponents(active, cfg)
}

// buildComponents maps active codes to Component lines using the config
// defaults, skipping codes absent from the config.
func buildComponents(codes []string, cfg map[string]ComponentDefault) []Component {
	out := make([]Component, 0, len(codes))
	for _, code := range codes {
		def, ok := cfg[code]
		if !ok {
			continue
		}
		out = append(out, NewComponent(code, def.DurationMin, def.WasteKg))
	}
	return out
}

func denierChanged(from, to float64) bool {
	if from == 0 || to == 0 {
		return false
	}
	return from != to
}

func colorFamilyChanged(from, to string) bool {
	from, to = normalize(from), normalize(to)
	if from == "" || to == "" {
		return false
	}
	return from != to
}

// shadeGoesDarkToLight reports a dark -> light transition (to is lighter than
// from, i.e. lower darkness ordinal). Unknown (0) darkness never triggers.
func shadeGoesDarkToLight(from, to int) bool {
	if from == 0 || to == 0 {
		return false
	}
	return to < from
}

func filamentChanged(from, to int) bool {
	if from == 0 || to == 0 {
		return false
	}
	return from != to
}

func twistChanged(from, to string) bool {
	from, to = normalize(from), normalize(to)
	if from == "" || to == "" {
		return false
	}
	return from != to
}

// lotChanged reports a same-product lot change. When product ids are known they
// must match (a product change is C1..C4, not C6); otherwise fall back to a
// bare lot difference.
func lotChanged(fromLot, toLot string, fromProd, toProd int64) bool {
	fromLot, toLot = normalize(fromLot), normalize(toLot)
	if fromLot == "" || toLot == "" || fromLot == toLot {
		return false
	}
	if fromProd != 0 && toProd != 0 && fromProd != toProd {
		return false
	}
	return true
}

// normalize trims and upper-cases a spec string for case-insensitive comparison.
func normalize(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
