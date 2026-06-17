package costcalc

import "time"

// ResultStatus enumerates cost result lifecycle.
type ResultStatus string

// Result status constants.
const (
	ResultStatusCalculated ResultStatus = "CALCULATED"
	ResultStatusVerified   ResultStatus = "VERIFIED"
	ResultStatusApproved   ResultStatus = "APPROVED"
	ResultStatusSuperseded ResultStatus = "SUPERSEDED"
)

// Result is the per-product cost result aggregate (one row in cst_product_cost).
type Result struct {
	id            int64
	productSysID  int64
	period        string
	calcType      CalculationType
	routeHeadID   int64
	version       int
	costPerUnit   float64
	totalRMCost   float64
	totalConv     float64
	totalCost     float64
	uomID         int
	currency      string
	costByLevel   []byte
	rmCostDetail  []byte
	paramSnapshot []byte
	formulaTrace  []byte
	inputHash     string
	status        ResultStatus
	jobID         int64
	calculatedAt  time.Time
	calculatedBy  string
	verifiedAt    *time.Time
	verifiedBy    string
	captiveCost   float64
	deliveryCost  float64
	vb1DelCost    float64
	vb2DelCost    float64
	vb3DelCost    float64
	vb4DelCost    float64
	vb5DelCost    float64
}

// NewResult constructs a fresh CALCULATED Result.
func NewResult(
	productSysID int64, period string, calcType CalculationType, routeHeadID int64, version int,
	costPerUnit, totalRM, totalConv, totalCost float64, uomID int, currency string,
	costByLevel, rmDetail, paramSnap, formulaTrace []byte, inputHash string,
	jobID int64, calculatedBy string,
	captiveCost, deliveryCost, vb1DelCost, vb2DelCost, vb3DelCost, vb4DelCost, vb5DelCost float64,
) *Result {
	return &Result{
		productSysID: productSysID, period: period, calcType: calcType, routeHeadID: routeHeadID, version: version,
		costPerUnit: costPerUnit, totalRMCost: totalRM, totalConv: totalConv, totalCost: totalCost,
		uomID: uomID, currency: currency,
		costByLevel: costByLevel, rmCostDetail: rmDetail, paramSnapshot: paramSnap, formulaTrace: formulaTrace,
		inputHash: inputHash, status: ResultStatusCalculated, jobID: jobID,
		calculatedAt: time.Now(), calculatedBy: calculatedBy,
		captiveCost: captiveCost, deliveryCost: deliveryCost,
		vb1DelCost: vb1DelCost, vb2DelCost: vb2DelCost, vb3DelCost: vb3DelCost,
		vb4DelCost: vb4DelCost, vb5DelCost: vb5DelCost,
	}
}

// HydrateResult reconstructs from DB state.
func HydrateResult(id, productSysID int64, period string, calcType CalculationType, routeHeadID int64, version int,
	costPerUnit, totalRM, totalConv, totalCost float64, uomID int, currency string,
	costByLevel, rmDetail, paramSnap, formulaTrace []byte, inputHash string, status ResultStatus,
	jobID int64, calculatedAt time.Time, calculatedBy string,
	verifiedAt *time.Time, verifiedBy string) *Result {
	return &Result{
		id: id, productSysID: productSysID, period: period, calcType: calcType, routeHeadID: routeHeadID, version: version,
		costPerUnit: costPerUnit, totalRMCost: totalRM, totalConv: totalConv, totalCost: totalCost,
		uomID: uomID, currency: currency,
		costByLevel: costByLevel, rmCostDetail: rmDetail, paramSnapshot: paramSnap, formulaTrace: formulaTrace,
		inputHash: inputHash, status: status, jobID: jobID,
		calculatedAt: calculatedAt, calculatedBy: calculatedBy,
		verifiedAt: verifiedAt, verifiedBy: verifiedBy,
	}
}

// AssignID records the surrogate ID after INSERT.
func (r *Result) AssignID(id int64) { r.id = id }

// ID returns the surrogate ID.
func (r *Result) ID() int64 { return r.id }

// ProductSysID returns the product surrogate key.
func (r *Result) ProductSysID() int64 { return r.productSysID }

// Period returns the YYYYMM period.
func (r *Result) Period() string { return r.period }

// CalcType returns the calculation type.
func (r *Result) CalcType() CalculationType { return r.calcType }

// RouteHeadID returns the costing route used.
func (r *Result) RouteHeadID() int64 { return r.routeHeadID }

// Version returns the result version.
func (r *Result) Version() int { return r.version }

// CostPerUnit returns the per-unit cost.
func (r *Result) CostPerUnit() float64 { return r.costPerUnit }

// TotalRMCost returns the total RM cost.
func (r *Result) TotalRMCost() float64 { return r.totalRMCost }

// TotalConv returns the total conversion cost.
func (r *Result) TotalConv() float64 { return r.totalConv }

// TotalCost returns the total cost.
func (r *Result) TotalCost() float64 { return r.totalCost }

// UomID returns the unit-of-measure ID.
func (r *Result) UomID() int { return r.uomID }

// Currency returns the currency code.
func (r *Result) Currency() string { return r.currency }

// CostByLevel returns the cost-by-level JSON blob.
func (r *Result) CostByLevel() []byte { return r.costByLevel }

// RMCostDetail returns the RM cost detail blob.
func (r *Result) RMCostDetail() []byte { return r.rmCostDetail }

// ParamSnapshot returns the parameter snapshot blob.
func (r *Result) ParamSnapshot() []byte { return r.paramSnapshot }

// FormulaTrace returns the formula evaluation trace blob.
func (r *Result) FormulaTrace() []byte { return r.formulaTrace }

// InputHash returns the deterministic hash of all inputs.
func (r *Result) InputHash() string { return r.inputHash }

// Status returns the current result status.
func (r *Result) Status() ResultStatus { return r.status }

// JobID returns the job that produced this result.
func (r *Result) JobID() int64 { return r.jobID }

// CalculatedAt returns the calculation timestamp.
func (r *Result) CalculatedAt() time.Time { return r.calculatedAt }

// CalculatedBy returns the user who triggered the calc.
func (r *Result) CalculatedBy() string { return r.calculatedBy }

// VerifiedAt returns the verification/approval timestamp.
func (r *Result) VerifiedAt() *time.Time { return r.verifiedAt }

// VerifiedBy returns the verifier/approver.
func (r *Result) VerifiedBy() string { return r.verifiedBy }

// CaptiveCost returns COST_CAP_FINAL — captive packaging cost per kg.
func (r *Result) CaptiveCost() float64 { return r.captiveCost }

// DeliveryCost returns COST_DEL_FINAL — delivery packaging cost per kg.
func (r *Result) DeliveryCost() float64 { return r.deliveryCost }

// VB1DelCost returns VB1_DEL_COST — delivery cost at volume bucket 1 threshold.
func (r *Result) VB1DelCost() float64 { return r.vb1DelCost }

// VB2DelCost returns VB2_DEL_COST — delivery cost at volume bucket 2 threshold.
func (r *Result) VB2DelCost() float64 { return r.vb2DelCost }

// VB3DelCost returns VB3_DEL_COST — delivery cost at volume bucket 3 threshold.
func (r *Result) VB3DelCost() float64 { return r.vb3DelCost }

// VB4DelCost returns VB4_DEL_COST — delivery cost at volume bucket 4 threshold.
func (r *Result) VB4DelCost() float64 { return r.vb4DelCost }

// VB5DelCost returns VB5_DEL_COST — delivery cost at volume bucket 5 threshold.
func (r *Result) VB5DelCost() float64 { return r.vb5DelCost }

// MarkVerified transitions CALCULATED -> VERIFIED.
func (r *Result) MarkVerified(by string) error {
	if r.status != ResultStatusCalculated {
		return ErrCostInvalidStatus
	}
	r.status = ResultStatusVerified
	now := time.Now()
	r.verifiedAt = &now
	r.verifiedBy = by
	return nil
}

// MarkApproved transitions VERIFIED -> APPROVED.
func (r *Result) MarkApproved(by string) error {
	if r.status != ResultStatusVerified {
		return ErrCostInvalidStatus
	}
	r.status = ResultStatusApproved
	now := time.Now()
	r.verifiedAt = &now
	r.verifiedBy = by
	return nil
}

// AuditHistoryEntry captures a recompute event (writes to aud_cost_history).
type AuditHistoryEntry struct {
	ProductSysID int64
	Period       string
	CalcType     CalculationType
	OldCostID    int64
	NewCostID    int64
	OldTotal     float64
	NewTotal     float64
	VariancePct  float64
	OldJobID     int64
	NewJobID     int64
	ChangeReason string
	ChangedBy    string
}
