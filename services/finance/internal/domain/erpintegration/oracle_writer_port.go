package erpintegration

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// OracleWriter is the ONLY way GoApps changes ERP state (design §3.3, §9.3).
// It exposes typed operations only; there is deliberately no generic
// Exec(sql) method. Every implementation maps each method to exactly one
// allowlisted statement key.
type OracleWriter interface {
	// InsertBatch writes the batch header (W1_INSERT_BATCH) and all cost
	// rows (W1_INSERT_COST) in ONE Oracle transaction.
	InsertBatch(ctx context.Context, b PushBatch, rows []PushRow) (WriteResult, error)
	// ValuateAdj runs W2_VALUATE_ADJ (period-wide, P_BATCH_ID only).
	ValuateAdj(ctx context.Context, batchID int64) (Summary, error)
	// ApproveAdj runs W2_APPROVE_ADJ.
	ApproveAdj(ctx context.Context, batchID int64, apprUID string) (Summary, error)
	// RestoreAdj runs W2_RESTORE_ADJ.
	RestoreAdj(ctx context.Context, batchID int64) (Summary, error)
	// LockBatch runs W2_LOCK_BATCH.
	LockBatch(ctx context.Context, batchID int64) (Summary, error)
}

// WriterMode selects the OracleWriter implementation.
type WriterMode string

// Writer modes (erp_integration.writer_mode).
const (
	WriterModeDisabled WriterMode = "disabled"
	WriterModeFake     WriterMode = "fake"
	WriterModeOracle   WriterMode = "oracle"
)

// IsValid reports whether m is a known writer mode.
func (m WriterMode) IsValid() bool {
	switch m {
	case WriterModeDisabled, WriterModeFake, WriterModeOracle:
		return true
	}
	return false
}

// PushBatch is the header row of CST_GOAPPS_STD_BATCH (W1_INSERT_BATCH).
type PushBatch struct {
	BatchID  int64
	Period   string // YYYYMM
	Seq      int32
	RuleHash string
	RowCount int64
	SumStd   decimal.Decimal
	SumConv  decimal.Decimal
	SumPvl   decimal.Decimal
	PushedBy string
}

// PushRow is one CST_GOAPPS_STD_COST row (W1_INSERT_COST). GSC_BATCH_ID and
// GSC_PERIOD come from the PushBatch; GSC_PUSHED_DT is set by Oracle.
type PushRow struct {
	ItemCode      string
	GradeCode     string
	ShadeCode     string
	ItemName      string
	ShadeName     string
	Source        string
	StdCost       decimal.Decimal
	ConvCost      decimal.NullDecimal
	ConvCost1     decimal.NullDecimal
	ConvCost2     decimal.NullDecimal
	ConvCost4     decimal.NullDecimal
	ConvCost5     decimal.NullDecimal
	ChpConKg      decimal.NullDecimal
	ChpCost       decimal.NullDecimal
	ChpItemCode   string
	FgType        string
	Basis         string
	SellingPrice  decimal.NullDecimal
	AxCost        decimal.NullDecimal
	AxConvCost    decimal.NullDecimal
	ValueLoss     decimal.NullDecimal
	ProdValLoss   decimal.NullDecimal
	MsBatchItem   string
	ItemType      string
	PrdPerDay     decimal.NullDecimal
	AxCostSysID   *int64
	AxCostVersion *int32
}

// WriteResult reports what a W1 push inserted.
type WriteResult struct {
	BatchRows int64
	CostRows  int64
}

// Summary is the P_SUMMARY text returned by a PKG_GOAPPS_ADJ call.
type Summary struct {
	Text string
}

// Writer sentinel errors.
var (
	// ErrWriterDisabled is returned by every method of the disabled writer.
	ErrWriterDisabled = errors.New("erpintegration: oracle writer disabled")
	// ErrOracleBusy maps ORA-00054 / ORA-30006 (resource busy, NOWAIT).
	ErrOracleBusy = errors.New("erpintegration: oracle resource busy")
	// ErrOracleTimeout is a call timeout where Oracle definitely did not commit.
	ErrOracleTimeout = errors.New("erpintegration: oracle call timed out")
	// ErrOutcomeUnknown means the call may or may not have committed; the
	// caller must resolve it with a read-only probe.
	ErrOutcomeUnknown = errors.New("erpintegration: oracle call outcome unknown")
)

// Oracle application error codes raised by PKG_GOAPPS_ADJ and guard triggers.
const (
	OraPeriodFrozen     = 20901
	OraCode20902        = 20902
	OraCode20903        = 20903
	OraCode20904        = 20904
	OraCode20905        = 20905
	OraGuardWrongUser   = 20910
	OraGuardUpdateDel   = 20911
	OraGuardNotPushed   = 20912
	OraResourceBusy     = 54
	OraResourceBusyWait = 30006
)

// OracleAppError is an Oracle application error (ORA-2090x/2091x).
type OracleAppError struct {
	Code    int
	Message string
}

// Error implements error.
func (e *OracleAppError) Error() string {
	return fmt.Sprintf("ORA-%05d: %s", e.Code, e.Message)
}
