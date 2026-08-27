// Package mbhead provides domain logic for Melange Batch Head (MEL product type) management.
package mbhead

// MaxAdditionalShades is the number of extra shade rows a head may carry in
// mst_mb_head_shade. One shade lives on the header itself (mbh_shade_code /
// mbh_shade_name), so the effective total is 3 shades per head.
//
// The same ceiling is enforced twice in the database by migration 000483:
// CHECK (mbhs_seq_no IN (1,2)) makes a third sequence impossible, and the
// partial unique index uix_mbhs_mbh_seq keeps one live row per (head, seq).
// The domain rejects first so the user gets a readable error.
const MaxAdditionalShades = 2

// Shade is one additional shade row belonging to an MB head.
//
// Code maps to mbhs_shade_code, which is NOT NULL in 000483 — it must never be
// empty. Name maps to mbhs_shade_name, which is NULLABLE — an empty name is a
// legitimate value and is deliberately accepted.
type Shade struct {
	SeqNo int32
	Code  string
	Name  string
}

// AdditionalShades returns the extra shade rows attached to this head. Nil when
// the head has none — absence is preserved, no empty-slice substitution (D13).
func (e *Entity) AdditionalShades() []Shade { return e.additionalShades }

// SetAdditionalShades replaces the head's additional shade rows after validating
// them against the shape migration 000483 permits.
//
// Rejects: more than MaxAdditionalShades rows (ErrTooManyShades), a sequence
// number outside 1..MaxAdditionalShades, a duplicate sequence number, and an
// empty shade code. An empty shade NAME is accepted — mbhs_shade_name is
// nullable.
//
// A nil or empty slice is valid and clears the additional shades: a payload that
// omits additional_shades must keep working (D13).
func (e *Entity) SetAdditionalShades(shades []Shade) error {
	if len(shades) > MaxAdditionalShades {
		return ErrTooManyShades
	}
	seen := make(map[int32]struct{}, len(shades))
	for _, s := range shades {
		if s.SeqNo < 1 || s.SeqNo > MaxAdditionalShades {
			return ErrTooManyShades
		}
		if _, dup := seen[s.SeqNo]; dup {
			return ErrTooManyShades
		}
		seen[s.SeqNo] = struct{}{}
		if s.Code == "" {
			return ErrRecipeFieldRequired
		}
	}
	if len(shades) == 0 {
		e.additionalShades = nil
		return nil
	}
	e.additionalShades = append([]Shade(nil), shades...)
	return nil
}
