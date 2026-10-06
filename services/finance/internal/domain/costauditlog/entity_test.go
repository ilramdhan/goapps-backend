package costauditlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateOperationWhitelist(t *testing.T) {
	for _, op := range []string{
		OpInsert, OpUpdate, OpDelete, OpStatusChange, OpFeasibility, OpClassificationOverride,
		OpAssign, OpPromote, OpHide, OpUnhide, OpRuleCreate, OpRuleUpdate, OpRuleDelete,
		OpErpLink, OpErpPeriodLock, OpErpPeriodUnlock, OpErpPush, OpErpValuate,
		OpErpAdjApprove, OpErpRestore, OpErpBatchLock, OpErpAttrBackfill, OpErpWarnAck,
	} {
		assert.NoError(t, NewInput{Operation: op}.Validate(), op)
		assert.LessOrEqual(t, len(op), 30, "cal_operation is VARCHAR(30)")
	}
	for _, op := range []string{"", "ERP_UNKNOWN", "insert"} {
		assert.ErrorIs(t, NewInput{Operation: op}.Validate(), ErrInvalidOperation, op)
	}
}
