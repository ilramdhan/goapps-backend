package mbcrosssection

import "errors"

// Domain errors for MB cross-section conversion factor operations.
var (
	// ErrFactorCodeRequired is returned when from_code or to_code is empty.
	ErrFactorCodeRequired = errors.New("mbcrosssection: from_code and to_code are required")
	// ErrFactorSelfPair is returned when from_code equals to_code (chk_mbcf_not_self).
	ErrFactorSelfPair = errors.New("mbcrosssection: invalid factor, from_code and to_code must differ")
	// ErrFactorNotPositive is returned when the factor is not strictly greater than zero.
	ErrFactorNotPositive = errors.New("mbcrosssection: invalid factor, must be greater than zero")
	// ErrFactorInvalidOperation is returned when the operation is neither MULTIPLY nor DIVIDE.
	ErrFactorInvalidOperation = errors.New("mbcrosssection: invalid operation, must be MULTIPLY or DIVIDE")
	// ErrFactorAlreadyExists is returned when a live row already covers the ordered pair.
	ErrFactorAlreadyExists = errors.New("mbcrosssection: factor for this code pair already exists")
	// ErrFactorNotFound is returned when a factor row is not found.
	ErrFactorNotFound = errors.New("mbcrosssection: factor not found")
)
