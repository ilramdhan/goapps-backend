// Package financeclient wraps the gRPC client for finance.v1.CostMasterLookupService.
package financeclient

import "errors"

// Sentinel errors for finance product lookups.
var (
	// ErrDegraded is returned when the client is disabled (empty host) and a
	// data-returning method is called. Read/display paths may soften this to an
	// empty result; write paths must not.
	ErrDegraded = errors.New("financeclient: running in degraded mode (no finance connection)")

	// ErrProductValidationUnavailable is returned by ValidateProduct when the
	// client is degraded. Write paths must refuse rather than persist an
	// unvalidated cpm_product_sys_id: a planner blocked for thirty seconds is
	// cheaper than a corrupt row. Read/display paths use
	// ValidateProductForDisplay, which stays permissive.
	ErrProductValidationUnavailable = errors.New("product validation unavailable: finance connection is degraded")

	// ErrProductNotFound is returned when a product sys id does not resolve.
	ErrProductNotFound = errors.New("invalid cpm_product_sys_id: not found in finance")

	// ErrProductInactive is returned when a product resolves but is inactive.
	ErrProductInactive = errors.New("invalid cpm_product_sys_id: product is inactive")

	// ErrFinanceRefused is returned when finance answers with a gRPC OK status
	// but an unsuccessful BaseResponse — its convention for application-level
	// refusals such as a rejected internal service token or a failed request
	// validation. Reading only the payload would render such a refusal
	// indistinguishable from an empty result, which is how a service-token
	// mismatch once stayed invisible in staging and production while every
	// sales-order-staging row silently remained UNRESOLVED.
	ErrFinanceRefused = errors.New("finance refused the request")
)
