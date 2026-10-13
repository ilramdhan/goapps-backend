package rmgroup

import "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"

// applyDenierPatch applies the request-level denier semantics to a head:
// nil = unchanged, 0 = clear to NULL, >0 = set, <0 = rmgroup.ErrInvalidDenier.
func applyDenierPatch(head *rmgroup.Head, denier *float64) error {
	if denier == nil {
		return nil
	}
	if *denier == 0 {
		return head.SetDenier(nil)
	}
	return head.SetDenier(denier)
}

// denierPatchChanges reports whether applying the denier patch would change the head.
func denierPatchChanges(head *rmgroup.Head, denier *float64) bool {
	if denier == nil {
		return false
	}
	cur := head.Denier()
	if *denier == 0 {
		return cur != nil
	}
	return cur == nil || *cur != *denier
}
