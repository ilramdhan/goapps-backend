package audit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/audit"
)

func TestEntryType_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   audit.EntryType
		want string
	}{
		{name: "dashboard", in: audit.EntryTypeDashboard, want: "dashboard"},
		{name: "group", in: audit.EntryTypeGroup, want: "group"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.in.String())
		})
	}
}

func TestAction_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   audit.Action
		want string
	}{
		{name: "create", in: audit.ActionCreate, want: "CREATE"},
		{name: "update", in: audit.ActionUpdate, want: "UPDATE"},
		{name: "delete", in: audit.ActionDelete, want: "DELETE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.in.String())
		})
	}
}

func TestEntry_FieldsRetained(t *testing.T) {
	t.Parallel()
	e := audit.Entry{
		AuditID:     7,
		EntityType:  audit.EntryTypeDashboard,
		EntityCode:  "EBITDA",
		EntityTitle: "EBITDA Margin",
		Action:      audit.ActionCreate,
		ChangedBy:   "00000000-0000-0000-0000-000000000001",
		Summary:     "Created dashboard EBITDA",
	}
	assert.Equal(t, int64(7), e.AuditID)
	assert.Equal(t, audit.EntryTypeDashboard, e.EntityType)
	assert.Equal(t, "EBITDA", e.EntityCode)
	assert.Equal(t, audit.ActionCreate, e.Action)
	assert.True(t, e.ChangedAt.IsZero())
}
