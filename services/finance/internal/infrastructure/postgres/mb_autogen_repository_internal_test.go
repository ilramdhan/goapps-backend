package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A composition row that prices through an RM group must actually name one. An empty
// crm_rm_group_code is not NULL, so it slips past chk_crm_one_ref and
// chk_crm_type_ref_match (migration 000222) and the route silently prices that line at
// 0 at month-end instead of raising ErrMissingRMCost. Failing at Validate is visible
// and fixable; failing at month-end is neither.
func TestMBGroupRef_EmptyGroupCodeIsRejected(t *testing.T) {
	for _, sourceType := range []string{"CARRIER", "GROUP"} {
		t.Run(sourceType, func(t *testing.T) {
			_, err := mbGroupRef(VersionRow{SourceType: sourceType, SeqNo: 7, GroupCode: ""})
			require.Error(t, err)
			assert.Contains(t, err.Error(), sourceType)
			assert.Contains(t, err.Error(), "7", "the error must name the seq no so the user can find the row")
		})
	}
}

func TestMBGroupRef_ReturnsPointerToGroupCode(t *testing.T) {
	got, err := mbGroupRef(VersionRow{SourceType: "GROUP", SeqNo: 1, GroupCode: "RG-001"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "RG-001", *got)
}
