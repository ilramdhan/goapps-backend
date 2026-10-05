package erpintegration

// batch_ports.go declares the ports of the ERP batch steps (plan-04 P3-T4):
// period lock (G10), step job publisher and the PG coverage source.

import (
	"context"
	"errors"
	"strings"

	"github.com/shopspring/decimal"
)

// Step port errors.
var (
	// ErrPublisherUnavailable is returned when no RabbitMQ publisher is
	// wired, so no step job can be queued.
	ErrPublisherUnavailable = errors.New("erpintegration: message queue unavailable")
	// ErrPeriodLockNotConfigured is returned when no period-lock checker is
	// wired: batch creation fails closed rather than assume "locked".
	ErrPeriodLockNotConfigured = errors.New("erpintegration: period lock checker not configured")
	// ErrPostedProbeNotConfigured is returned when no read-only ADJ head
	// probe is wired: a LIVE batch cannot be created (fails closed, V-10).
	ErrPostedProbeNotConfigured = errors.New("erpintegration: ADJ posted probe not configured")
	// ErrDemandReaderNotConfigured is returned by LoadDemand when the
	// read-only Oracle demand reader is not wired (Oracle absent).
	ErrDemandReaderNotConfigured = errors.New("erpintegration: ADJ demand reader not configured")
	// ErrStepNotAllowed is returned when a step is requested in a batch
	// status that cannot run it.
	ErrStepNotAllowed = errors.New("erpintegration: step not allowed in the current batch status")
	// ErrUnknownStep is returned for an unsupported erp_integration subtype.
	ErrUnknownStep = errors.New("erpintegration: unknown erp_integration step")
	// ErrInvalidJobParams is returned for a job whose params lack batch_id.
	ErrInvalidJobParams = errors.New("erpintegration: invalid erp_integration job params")
)

// PeriodLockChecker is the G10 port (implemented by the postgres period lock
// repository with a plain SELECT). calcType is always ACTUAL here.
type PeriodLockChecker interface {
	IsLocked(ctx context.Context, period string, calcType string) (bool, error)
}

// periodLockCalcType is the cost type the ERP integration locks (ACTUAL).
const periodLockCalcType = "ACTUAL"

// ErpJobPublisher publishes erp_integration step jobs to
// finance.jobs.erp_integration (rabbitmq.JobPublisherAdapter).
type ErpJobPublisher interface {
	PublishErpIntegration(ctx context.Context, jobID, subtype, period, createdBy string) error
}

// ErpProductKey is the normalized D-LINK key: ERP item code and shade code,
// trimmed and upper-cased (a NULL shade is ""). cpm_erp_grade_code_1/2 are
// never part of it.
type ErpProductKey struct {
	ItemCode  string
	ShadeCode string
}

// NewErpProductKey normalizes an (item, shade) pair.
func NewErpProductKey(item, shade string) ErpProductKey {
	return ErpProductKey{
		ItemCode:  strings.ToUpper(strings.TrimSpace(item)),
		ShadeCode: strings.ToUpper(strings.TrimSpace(shade)),
	}
}

// ProductCandidate is one active AX cost_product_master row holding a key.
type ProductCandidate struct {
	SysID       int64
	ProductCode string
	TypeCode    string // cost_product_type.cpt_type_code (MB for masterbatch)
}

// ActualCost is the active (non-SUPERSEDED) ACTUAL cst_product_cost row of a
// product for the period (uk_cpc_active: at most one).
type ActualCost struct {
	CostID      int64
	Version     int32
	Status      string
	Currency    string
	CostPerUnit decimal.Decimal
}

// CoverageSource is the read-only PG port of the coverage step. Every method
// is a SELECT.
type CoverageSource interface {
	// ResolveProducts returns, per requested key, every active AX product
	// whose (cpm_erp_item_code, cpm_shade_code) equals the key. Keys with no
	// product are absent from the map.
	ResolveProducts(ctx context.Context, keys []ErpProductKey) (map[ErpProductKey][]ProductCandidate, error)
	// ActualCosts returns the active ACTUAL cost row of each product for the
	// period. Products with no such row are absent from the map.
	ActualCosts(ctx context.Context, period string, productSysIDs []int64) (map[int64]ActualCost, error)
}
