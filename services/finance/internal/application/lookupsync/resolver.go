package lookupsync

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// Lookup master codes the Resolver routes on.
const (
	MasterMachine        = "MACHINE"
	MasterIntermingling  = "INTERMINGLING"
	MasterProductGrade   = "PRODUCT_GRADE"
	MasterMBHead         = "MB_HEAD"
	MasterMBSpin         = "MB_SPIN"
	MasterBoxBobbinCost  = "BOX_BOBBIN_COST"
	MasterRMGroupOil     = "RM_GROUP_OIL"
	successFillRetrieved = "Fill values retrieved"
)

// ErrUnknownMaster is returned by Resolve when the lookup master code is not
// one the Resolver can route. The wrapped message carries the quoted code.
var ErrUnknownMaster = errors.New("unknown lookup_master_code")

// FillGroupError wraps a failure to load the fill-group children of the
// trigger parameter (paramRepo.GetByFillGroup). Error() returns the underlying
// message verbatim so callers that surface it keep the original text; callers
// use errors.As to tell it apart from a master-record lookup failure.
type FillGroupError struct {
	Err error
}

// Error returns the underlying error message unchanged.
func (e *FillGroupError) Error() string { return e.Err.Error() }

// Unwrap exposes the underlying error to errors.Is / errors.As.
func (e *FillGroupError) Unwrap() error { return e.Err }

// IsNotFound reports whether err is the not-found sentinel of any master
// repository the Resolver reads from.
func IsNotFound(err error) bool {
	return errors.Is(err, machine.ErrNotFound) ||
		errors.Is(err, intermingling.ErrNotFound) ||
		errors.Is(err, productgrade.ErrNotFound) ||
		errors.Is(err, mbhead.ErrNotFound) ||
		errors.Is(err, mbspin.ErrNotFound) ||
		errors.Is(err, boxbobbincost.ErrNotFound)
}

// FillResult is the outcome of resolving one master record for one trigger
// parameter: child param_code → value, plus the label and message the
// interactive fill shows.
type FillResult struct {
	Numeric      map[string]float64
	Text         map[string]string
	DisplayLabel string
	// Message is the human-readable success message for the resolved master.
	Message string
}

// Resolver resolves lookup master records into fill-group child values. It
// depends on domain repository interfaces only.
type Resolver struct {
	machineRepo       machine.Repository
	interminglingRepo intermingling.Repository
	productGradeRepo  productgrade.Repository
	mbHeadRepo        mbhead.Repository
	mbSpinRepo        mbspin.Repository
	boxBobbinRepo     boxbobbincost.Repository
	paramRepo         parameter.Repository
	// superbaFallback resolves a SUPERBA shade stored in MB_SP_CODE (optional; see
	// WithSuperbaFallback).
	superbaFallback mbsource.Provider
}

// NewResolver creates a Resolver over the given repositories.
func NewResolver(
	machineRepo machine.Repository,
	interminglingRepo intermingling.Repository,
	productGradeRepo productgrade.Repository,
	mbHeadRepo mbhead.Repository,
	mbSpinRepo mbspin.Repository,
	boxBobbinRepo boxbobbincost.Repository,
	paramRepo parameter.Repository,
) *Resolver {
	return &Resolver{
		machineRepo:       machineRepo,
		interminglingRepo: interminglingRepo,
		productGradeRepo:  productGradeRepo,
		mbHeadRepo:        mbHeadRepo,
		mbSpinRepo:        mbSpinRepo,
		boxBobbinRepo:     boxBobbinRepo,
		paramRepo:         paramRepo,
	}
}

// WithSuperbaFallback lets the MB_SPIN fill resolve a Superba shade (stored in MB_SP_CODE by the
// shade-driven auto-fill) instead of returning NotFound: it fills MB_SP_DYE with the color name.
func (r *Resolver) WithSuperbaFallback(p mbsource.Provider) *Resolver {
	r.superbaFallback = p
	return r
}

// Resolve routes to master-specific fill logic by lookupMasterCode.
//
// Errors: ErrUnknownMaster (wrapped) for an unroutable code; *FillGroupError
// when the trigger parameter's fill-group children cannot be loaded; otherwise
// the master repository error unchanged (see IsNotFound). An MB_SPIN key that is
// not a spin but a known Superba shade (when the fallback is wired) resolves
// successfully.
func (r *Resolver) Resolve(ctx context.Context, lookupMasterCode, selectedKey, sourceParamCode string) (FillResult, error) {
	switch lookupMasterCode {
	case MasterMachine:
		return r.fillFromMachine(ctx, selectedKey, sourceParamCode)
	case MasterIntermingling:
		return r.fillFromIntermingling(ctx, selectedKey, sourceParamCode)
	case MasterProductGrade:
		return r.fillFromProductGrade(ctx, selectedKey, sourceParamCode)
	case MasterMBHead:
		return r.fillFromMBHead(ctx, selectedKey, sourceParamCode)
	case MasterMBSpin:
		return r.fillFromMBSpin(ctx, selectedKey, sourceParamCode)
	case MasterBoxBobbinCost:
		return r.fillFromBoxBobbinCost(ctx, selectedKey, sourceParamCode)
	case MasterRMGroupOil:
		// oil-cost-rm-group D14: OIL_NAME's fill-group children are never filled
		// from the lookup — OIL_RATE is resolved per calc period by the engine
		// and OIL_GAIN_POY_DEFAULT is formula-produced. Return success with no
		// fills so selecting an oil group does not 404.
		return FillResult{
			Numeric:      map[string]float64{},
			Text:         map[string]string{},
			DisplayLabel: selectedKey,
			Message:      "RM group (oil) selected — children resolved by the calc engine",
		}, nil
	default:
		return FillResult{}, fmt.Errorf("%w: %q", ErrUnknownMaster, lookupMasterCode)
	}
}

// warnNoReader reports a lookup_source_column that is registered in
// mst_lookup_master_column but has no reader in the Go-side reader maps (or no
// switch case). Such a column is silently skipped and yields an empty fill, so
// the only trace of the failure is this log line. It stays a warning rather than
// an error on purpose: registered-but-unreadable columns still exist in the wild
// and hard-failing would break otherwise working lookups.
func warnNoReader(ctx context.Context, source, col, paramCode, sourceParamCode string) {
	log.Ctx(ctx).Warn().
		Str("lookup_source_column", col).
		Str("param_code", paramCode).
		Str("source_param_code", sourceParamCode).
		Msg(source + " fill: no reader registered for lookup_source_column — value skipped")
}

// fillChildren loads the fill-group children of triggerParamCode, wrapping a
// failure in *FillGroupError.
func (r *Resolver) fillChildren(ctx context.Context, triggerParamCode string) ([]*parameter.Parameter, error) {
	children, err := r.paramRepo.GetByFillGroup(ctx, triggerParamCode)
	if err != nil {
		return nil, &FillGroupError{Err: err}
	}
	return children, nil
}
