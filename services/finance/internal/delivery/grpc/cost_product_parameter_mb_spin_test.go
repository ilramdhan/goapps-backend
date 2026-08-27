package grpc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// TestRequiredEntryToProto_MBSpinAmbiguityStates proves that the three
// read-time MB_SPIN states described in
// docs/superpowers/mbspin-tanda-varian-ganda-rancangan.md §6 (pertanyaan 3)
// stay distinguishable once mapped onto the wire proto: (1) already
// resolved, (2) unresolved with more than one candidate ("pick a variant"),
// and (3) unresolved with zero candidates ("code not found, fix the data").
// A prior version of this read path collapsed both unresolved cases into an
// indistinguishable nil, which is the exact bug this test guards against.
func TestRequiredEntryToProto_MBSpinAmbiguityStates(t *testing.T) {
	paramID := uuid.New()
	spinID := uuid.New()
	ambiguousCount := int32(2)
	zeroCount := int32(0)
	oneCount := int32(1)

	tests := []struct {
		name string
		val  *cpp.Value

		wantValueMBSpinID     string
		wantHasCandidateCount bool
		wantCandidateCount    int32
	}{
		{
			name: "already selected — resolved to exactly one spin at save time",
			val: &cpp.Value{
				ValueID:              1,
				ParamID:              paramID,
				ValueText:            strPtr("some-orion-code"),
				ValueMBSpinID:        &spinID,
				MBSpinCandidateCount: &oneCount,
				FilledAt:             time.Now(),
			},
			wantValueMBSpinID:     spinID.String(),
			wantHasCandidateCount: true,
			wantCandidateCount:    1,
		},
		{
			name: "unresolved, ambiguous — more than one candidate, needs manual pick",
			val: &cpp.Value{
				ValueID:              2,
				ParamID:              paramID,
				ValueText:            strPtr("dup-orion-code"),
				ValueMBSpinID:        nil,
				MBSpinCandidateCount: &ambiguousCount,
				FilledAt:             time.Now(),
			},
			wantValueMBSpinID:     "",
			wantHasCandidateCount: true,
			wantCandidateCount:    2,
		},
		{
			name: "unresolved, orphan — zero candidates, code not found in master data",
			val: &cpp.Value{
				ValueID:              3,
				ParamID:              paramID,
				ValueText:            strPtr("nonexistent-orion-code"),
				ValueMBSpinID:        nil,
				MBSpinCandidateCount: &zeroCount,
				FilledAt:             time.Now(),
			},
			wantValueMBSpinID:     "",
			wantHasCandidateCount: true,
			wantCandidateCount:    0,
		},
		{
			name: "not applicable — non-MB_SPIN parameter, candidate count never computed",
			val: &cpp.Value{
				ValueID:              4,
				ParamID:              paramID,
				ValueNumeric:         strPtr("12.5"),
				ValueMBSpinID:        nil,
				MBSpinCandidateCount: nil,
				FilledAt:             time.Now(),
			},
			wantValueMBSpinID:     "",
			wantHasCandidateCount: false,
			wantCandidateCount:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := cpp.RequiredEntry{
				Meta:  cpp.ParamMeta{ParamID: paramID, LookupMasterCode: "MB_SPIN"},
				Value: tc.val,
			}
			out := requiredEntryToProto(entry)

			assert.Equal(t, tc.wantValueMBSpinID, out.ValueMbSpinId)
			assert.Equal(t, tc.wantHasCandidateCount, out.HasMbSpinCandidateCount)
			assert.Equal(t, tc.wantCandidateCount, out.MbSpinCandidateCount)
		})
	}
}

// TestRequiredEntryToProto_AmbiguousAndOrphanAreDistinguishable is a direct
// pairwise check that state (2) and state (3) never produce the same proto
// shape — the specific regression this feature guards against per the
// 2026-08-27 decision that zero-candidate ("orphan") and >1-candidate
// ("ambiguous") must be told apart, since their remediation differs.
func TestRequiredEntryToProto_AmbiguousAndOrphanAreDistinguishable(t *testing.T) {
	paramID := uuid.New()
	ambiguousCount := int32(2)
	zeroCount := int32(0)

	ambiguous := requiredEntryToProto(cpp.RequiredEntry{
		Meta: cpp.ParamMeta{ParamID: paramID, LookupMasterCode: "MB_SPIN"},
		Value: &cpp.Value{
			ValueID:              1,
			ParamID:              paramID,
			ValueText:            strPtr("dup-code"),
			MBSpinCandidateCount: &ambiguousCount,
		},
	})
	orphan := requiredEntryToProto(cpp.RequiredEntry{
		Meta: cpp.ParamMeta{ParamID: paramID, LookupMasterCode: "MB_SPIN"},
		Value: &cpp.Value{
			ValueID:              2,
			ParamID:              paramID,
			ValueText:            strPtr("missing-code"),
			MBSpinCandidateCount: &zeroCount,
		},
	})

	assert.True(t, ambiguous.HasMbSpinCandidateCount)
	assert.True(t, orphan.HasMbSpinCandidateCount)
	assert.NotEqual(t, ambiguous.MbSpinCandidateCount, orphan.MbSpinCandidateCount,
		"ambiguous (>1 candidates) and orphan (0 candidates) must carry different counts downstream")
	assert.Empty(t, ambiguous.ValueMbSpinId, "unresolved states must not carry a spin id")
	assert.Empty(t, orphan.ValueMbSpinId, "unresolved states must not carry a spin id")
}
