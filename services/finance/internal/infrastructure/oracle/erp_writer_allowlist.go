package oracle

// StatementKey names one allowlisted ERP write statement (design §3.3).
type StatementKey string

// The six allowlisted statement keys. No other write may reach Oracle.
const (
	KeyW1InsertBatch StatementKey = "W1_INSERT_BATCH"
	KeyW1InsertCost  StatementKey = "W1_INSERT_COST"
	KeyW2ValuateAdj  StatementKey = "W2_VALUATE_ADJ"
	KeyW2ApproveAdj  StatementKey = "W2_APPROVE_ADJ"
	KeyW2RestoreAdj  StatementKey = "W2_RESTORE_ADJ"
	KeyW2LockBatch   StatementKey = "W2_LOCK_BATCH"
)

// W1InsertCostColumns is the CST_GOAPPS_STD_COST column list in bind order
// (every GSC_* column except GSC_PUSHED_DT, which Oracle defaults).
var W1InsertCostColumns = []string{
	"GSC_BATCH_ID", "GSC_PERIOD", "GSC_ITEM_CODE", "GSC_GRADE_CODE", "GSC_SHADE_CODE",
	"GSC_ITEM_NAME", "GSC_SHADE_NAME", "GSC_SOURCE", "GSC_STD_COST", "GSC_CONV_COST",
	"GSC_CONV_COST1", "GSC_CONV_COST2", "GSC_CONV_COST4", "GSC_CONV_COST5", "GSC_CHP_CON_KG",
	"GSC_CHP_COST", "GSC_CHP_ITEM_CODE", "GSC_FG_TYPE", "GSC_BASIS", "GSC_SELLING_PRICE",
	"GSC_AX_COST", "GSC_AX_CONV_COST", "GSC_VALUE_LOSS", "GSC_PROD_VAL_LOSS", "GSC_MS_BATCH_ITEM",
	"GSC_ITEM_TYPE", "GSC_PRD_PER_DAY", "GSC_AX_COST_SYS_ID", "GSC_AX_COST_VERSION",
}

// allowlist holds the constant texts. It is unexported so no caller can add
// to or change it at runtime; changing it requires a reviewed code change
// that also updates the golden test (CODEOWNERS, PR-7).
var allowlist = map[StatementKey]string{
	KeyW1InsertBatch: "INSERT INTO MGTDAT.CST_GOAPPS_STD_BATCH (GSB_BATCH_ID, GSB_PERIOD, GSB_SEQ, GSB_STATUS, " +
		"GSB_RULE_HASH, GSB_ROW_COUNT, GSB_SUM_STD, GSB_SUM_CONV, GSB_SUM_PVL, GSB_PUSHED_BY) " +
		"VALUES (:1, :2, :3, 'PUSHED', :4, :5, :6, :7, :8, :9)",
	KeyW1InsertCost: "INSERT INTO MGTDAT.CST_GOAPPS_STD_COST (GSC_BATCH_ID, GSC_PERIOD, GSC_ITEM_CODE, " +
		"GSC_GRADE_CODE, GSC_SHADE_CODE, GSC_ITEM_NAME, GSC_SHADE_NAME, GSC_SOURCE, GSC_STD_COST, " +
		"GSC_CONV_COST, GSC_CONV_COST1, GSC_CONV_COST2, GSC_CONV_COST4, GSC_CONV_COST5, GSC_CHP_CON_KG, " +
		"GSC_CHP_COST, GSC_CHP_ITEM_CODE, GSC_FG_TYPE, GSC_BASIS, GSC_SELLING_PRICE, GSC_AX_COST, " +
		"GSC_AX_CONV_COST, GSC_VALUE_LOSS, GSC_PROD_VAL_LOSS, GSC_MS_BATCH_ITEM, GSC_ITEM_TYPE, " +
		"GSC_PRD_PER_DAY, GSC_AX_COST_SYS_ID, GSC_AX_COST_VERSION) " +
		"VALUES (:1, :2, :3, :4, :5, :6, :7, :8, :9, :10, :11, :12, :13, :14, :15, :16, :17, :18, " +
		":19, :20, :21, :22, :23, :24, :25, :26, :27, :28, :29)",
	KeyW2ValuateAdj: "BEGIN MGTDAT.PKG_GOAPPS_ADJ.VALUATE_ADJ(:1, :2); END;",
	KeyW2ApproveAdj: "BEGIN MGTDAT.PKG_GOAPPS_ADJ.APPROVE_ADJ(:1, :2, :3); END;",
	KeyW2RestoreAdj: "BEGIN MGTDAT.PKG_GOAPPS_ADJ.RESTORE_ADJ(:1, :2); END;",
	KeyW2LockBatch:  "BEGIN MGTDAT.PKG_GOAPPS_ADJ.LOCK_BATCH(:1, :2); END;",
}

// Statement returns the allowlisted text for key and whether it exists.
func Statement(key StatementKey) (string, bool) {
	s, ok := allowlist[key]
	return s, ok
}

// AllowlistKeys returns the allowlisted keys (unordered copy).
func AllowlistKeys() []StatementKey {
	keys := make([]StatementKey, 0, len(allowlist))
	for k := range allowlist {
		keys = append(keys, k)
	}
	return keys
}
