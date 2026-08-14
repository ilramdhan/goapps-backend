// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// noOfProcessValidator checks a user-supplied mbh_no_of_process code against the live
// NO_OF_PROCESS options in mst_mb_param_option (spec section 2.3).
//
// The permitted set is deliberately not hardcoded: the domain value object enforces presence and
// length only, and this check reads the master so adding a fourth option needs no code change.
// It reuses mbparam.Repository.GetByCode, which already eager-loads the parameter's options — the
// same access path the dropdown and the Validate-time snapshot use.
type noOfProcessValidator struct {
	paramRepo mbparam.Repository
}

// validate rejects a code that is not an ACTIVE option of the NO_OF_PROCESS parameter.
//
// Active-only is deliberate. mbh_no_of_process is a brand-new column, so no historical row can
// carry a since-deactivated code, and is_active is precisely the master's "stop offering this"
// signal — accepting an inactive code would let a write store a value the dropdown can no longer
// produce or re-display. The frozen mbh_param_no_of_process snapshot is untouched by this check,
// so already-frozen historical values keep resolving during auto-gen regardless of is_active.
//
// An empty code short-circuits to nil: presence is the domain's job (mbhead.New / Entity.Update
// already reject it), and the Validate-time fall-through to the master default depends on empty
// staying a legitimate "not supplied" signal rather than an invalid-membership error.
func (v noOfProcessValidator) validate(ctx context.Context, code string) error {
	if code == "" {
		return nil
	}
	param, err := v.paramRepo.GetByCode(ctx, paramCodeNoOfProcess)
	if err != nil {
		return err
	}
	for _, opt := range param.Options() {
		if opt.Code() == code && opt.IsActive() {
			return nil
		}
	}
	return mbhead.ErrInvalidNoOfProcess
}
