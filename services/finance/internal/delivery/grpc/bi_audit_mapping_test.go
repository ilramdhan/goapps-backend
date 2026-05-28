package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/audit"
)

func TestAuditEntryToProto(t *testing.T) {
	t.Parallel()
	changedAt := time.Date(2026, time.May, 27, 10, 0, 0, 0, time.UTC)
	e := auditdomain.Entry{
		AuditID:     42,
		EntityType:  auditdomain.EntryTypeDashboard,
		EntityCode:  "EBITDA",
		EntityTitle: "EBITDA Margin",
		Action:      auditdomain.ActionUpdate,
		ChangedBy:   "user-uuid",
		ChangedAt:   changedAt,
		Summary:     "Updated dashboard EBITDA",
	}

	got := auditEntryToProto(e)

	require.NotNil(t, got)
	assert.Equal(t, int64(42), got.GetAuditId())
	assert.Equal(t, "dashboard", got.GetEntityType())
	assert.Equal(t, "EBITDA", got.GetEntityCode())
	assert.Equal(t, "EBITDA Margin", got.GetEntityTitle())
	assert.Equal(t, "UPDATE", got.GetAction())
	assert.Equal(t, "user-uuid", got.GetChangedBy())
	assert.Equal(t, "Updated dashboard EBITDA", got.GetSummary())
	require.NotNil(t, got.GetChangedAt())
	assert.Equal(t, changedAt.Unix(), got.GetChangedAt().GetSeconds())
}

func TestAuditEntryToProto_ZeroChangedAt(t *testing.T) {
	t.Parallel()
	got := auditEntryToProto(auditdomain.Entry{EntityType: auditdomain.EntryTypeGroup, Action: auditdomain.ActionDelete})
	require.NotNil(t, got)
	assert.Nil(t, got.GetChangedAt())
	assert.Equal(t, "group", got.GetEntityType())
}

func TestAuditLabel(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "SALES", auditLabel("SALES", "id-123"))
	assert.Equal(t, "id-123", auditLabel("", "id-123"))
}
