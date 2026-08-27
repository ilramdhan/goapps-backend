package mbdozing

import (
	"errors"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// Domain errors for MB dozing calculations.
var (
	// ErrZeroFilament is returned when a filament count is zero or negative.
	// Guarding here keeps sqrt(denier/filament) from silently producing +Inf or NaN.
	ErrZeroFilament = errors.New("mbdozing: filament must be greater than zero")
	// ErrInvalidMode is returned when the requested calculation mode is not SCALE or XSECTION.
	ErrInvalidMode = errors.New("mbdozing: invalid mode, must be SCALE or XSECTION")
	// ErrFactorNotFound is returned when no cross-section conversion factor was supplied
	// for the requested ordered pair. It aliases the mbcrosssection sentinel so callers
	// can match either package's error with errors.Is.
	ErrFactorNotFound = mbcrosssection.ErrFactorNotFound
)
