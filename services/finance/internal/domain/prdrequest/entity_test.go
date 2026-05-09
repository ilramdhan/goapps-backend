// Package prdrequest_test contains domain-layer tests for the Request aggregate.
package prdrequest_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
)

// =============================================================================
// Helpers
// =============================================================================

func makeTicketNo(t *testing.T) prdrequest.TicketNo {
	t.Helper()
	tn, err := prdrequest.NewTicketNo("PR-202504-001")
	require.NoError(t, err)
	return tn
}

func validRequest(t *testing.T) *prdrequest.Request {
	t.Helper()
	r, err := prdrequest.NewRequest(
		makeTicketNo(t),
		uuid.New(), "john.doe",
		uuid.New(), "DEPT-A",
		"Request Title Here", "Some description", `{"color":"red"}`,
		nil,
		"admin",
	)
	require.NoError(t, err)
	return r
}

// =============================================================================
// NewRequest
// =============================================================================

func TestNewRequest_Valid(t *testing.T) {
	requesterID := uuid.New()
	deptID := uuid.New()
	due := time.Now().Add(72 * time.Hour)
	tn := makeTicketNo(t)

	r, err := prdrequest.NewRequest(
		tn,
		requesterID, "john.doe",
		deptID, "DEPT-A",
		"My Request Title", "Detailed description", `{"spec":"value"}`,
		&due,
		"admin",
	)

	require.NoError(t, err)
	require.NotNil(t, r)

	assert.NotEqual(t, uuid.Nil, r.ID())
	assert.Equal(t, "PR-202504-001", r.TicketNo().String())
	assert.Equal(t, requesterID, r.RequesterID())
	assert.Equal(t, "john.doe", r.RequesterUsername())
	assert.Equal(t, deptID, r.RequesterDeptID())
	assert.Equal(t, "DEPT-A", r.RequesterDeptCode())
	assert.Equal(t, "My Request Title", r.Title().String())
	assert.Equal(t, "Detailed description", r.Description().String())
	assert.Equal(t, `{"spec":"value"}`, r.TargetSpecs().String())
	assert.Equal(t, prdrequest.StatusOpen, r.Status())
	assert.Equal(t, uuid.Nil, r.ResolvedProductID())
	assert.True(t, r.ResolutionNote().IsEmpty())
	assert.True(t, r.RejectReason().IsEmpty())
	assert.Equal(t, uuid.Nil, r.AssignedTo())
	assert.NotNil(t, r.DueDate())
	assert.Equal(t, "admin", r.CreatedBy())
	assert.False(t, r.CreatedAt().IsZero())
	assert.Nil(t, r.UpdatedAt())
	assert.Equal(t, "", r.UpdatedBy())
	assert.Nil(t, r.DeletedAt())
	assert.False(t, r.IsDeleted())
}

func TestNewRequest_Invalid(t *testing.T) {
	requesterID := uuid.New()
	deptID := uuid.New()
	tn := makeTicketNo(t)

	longStr := func(n int) string {
		return strings.Repeat("A", n)
	}

	tests := []struct {
		name        string
		requesterID uuid.UUID
		deptID      uuid.UUID
		title       string
		description string
		targetSpecs string
		wantErr     error
	}{
		{
			name:        "zero requester ID",
			requesterID: uuid.Nil, deptID: deptID,
			title: "Valid Title", description: "", targetSpecs: "",
			wantErr: prdrequest.ErrInvalidRequester,
		},
		{
			name:        "zero dept ID",
			requesterID: requesterID, deptID: uuid.Nil,
			title: "Valid Title", description: "", targetSpecs: "",
			wantErr: prdrequest.ErrInvalidRequester,
		},
		{
			name:        "empty title",
			requesterID: requesterID, deptID: deptID,
			title: "", description: "", targetSpecs: "",
			wantErr: prdrequest.ErrInvalidTitle,
		},
		{
			name:        "title too short (2 chars)",
			requesterID: requesterID, deptID: deptID,
			title: "ab", description: "", targetSpecs: "",
			wantErr: prdrequest.ErrInvalidTitle,
		},
		{
			name:        "title too long (201 chars)",
			requesterID: requesterID, deptID: deptID,
			title: longStr(201), description: "", targetSpecs: "",
			wantErr: prdrequest.ErrInvalidTitle,
		},
		{
			name:        "description too long (5001 chars)",
			requesterID: requesterID, deptID: deptID,
			title: "Valid Title", description: longStr(5001), targetSpecs: "",
			wantErr: prdrequest.ErrInvalidDescription,
		},
		{
			name:        "target specs too long (10001 chars)",
			requesterID: requesterID, deptID: deptID,
			title: "Valid Title", description: "", targetSpecs: longStr(10001),
			wantErr: prdrequest.ErrInvalidTargetSpecs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := prdrequest.NewRequest(
				tn,
				tc.requesterID, "user",
				tc.deptID, "DEPT",
				tc.title, tc.description, tc.targetSpecs,
				nil, "admin",
			)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tc.wantErr), "expected %v but got %v", tc.wantErr, err)
		})
	}
}

// =============================================================================
// Assign
// =============================================================================

func TestRequest_Assign_TransitionsOpenToInReview(t *testing.T) {
	r := validRequest(t)
	require.Equal(t, prdrequest.StatusOpen, r.Status())

	assigneeID := uuid.New()
	err := r.Assign(assigneeID, "manager")

	require.NoError(t, err)
	assert.Equal(t, prdrequest.StatusInReview, r.Status())
	assert.Equal(t, assigneeID, r.AssignedTo())
	assert.NotNil(t, r.UpdatedAt())
	assert.Equal(t, "manager", r.UpdatedBy())
}

func TestRequest_Assign_InReview_StaysInReview(t *testing.T) {
	r := validRequest(t)
	// First assign transitions OPEN -> IN_REVIEW.
	require.NoError(t, r.Assign(uuid.New(), "manager"))
	require.Equal(t, prdrequest.StatusInReview, r.Status())

	// Second assign keeps status as IN_REVIEW.
	newAssignee := uuid.New()
	err := r.Assign(newAssignee, "manager2")

	require.NoError(t, err)
	assert.Equal(t, prdrequest.StatusInReview, r.Status())
	assert.Equal(t, newAssignee, r.AssignedTo())
}

func TestRequest_Assign_ZeroAssignee_Fails(t *testing.T) {
	r := validRequest(t)

	err := r.Assign(uuid.Nil, "manager")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrInvalidAssignee))
}

func TestRequest_Assign_OnTerminal_Fails(t *testing.T) {
	r := validRequest(t)
	require.NoError(t, r.Reject("The request has been evaluated and cannot proceed.", "manager"))
	require.Equal(t, prdrequest.StatusRejected, r.Status())

	err := r.Assign(uuid.New(), "manager")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrCannotAssign))
}

// =============================================================================
// Resolve
// =============================================================================

func TestRequest_Resolve_Success(t *testing.T) {
	r := validRequest(t)
	productID := uuid.New()

	err := r.Resolve(productID, "Linked to product XYZ.", "resolver")

	require.NoError(t, err)
	assert.Equal(t, prdrequest.StatusResolved, r.Status())
	assert.Equal(t, productID, r.ResolvedProductID())
	assert.Equal(t, "Linked to product XYZ.", r.ResolutionNote().String())
	assert.NotNil(t, r.UpdatedAt())
	assert.Equal(t, "resolver", r.UpdatedBy())
}

func TestRequest_Resolve_EmptyNote_Success(t *testing.T) {
	r := validRequest(t)

	err := r.Resolve(uuid.New(), "", "resolver")

	require.NoError(t, err)
	assert.Equal(t, prdrequest.StatusResolved, r.Status())
	assert.True(t, r.ResolutionNote().IsEmpty())
}

func TestRequest_Resolve_AlreadyResolved(t *testing.T) {
	r := validRequest(t)
	require.NoError(t, r.Resolve(uuid.New(), "", "resolver"))

	err := r.Resolve(uuid.New(), "", "resolver")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrAlreadyResolved))
}

func TestRequest_Resolve_OnRejected_Fails(t *testing.T) {
	r := validRequest(t)
	require.NoError(t, r.Reject("Cannot proceed with this specification.", "manager"))

	err := r.Resolve(uuid.New(), "", "resolver")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrAlreadyRejected))
}

func TestRequest_Resolve_ZeroProductID_Fails(t *testing.T) {
	r := validRequest(t)

	err := r.Resolve(uuid.Nil, "", "resolver")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrInvalidProductLink))
}

func TestRequest_Resolve_NoteTooLong_Fails(t *testing.T) {
	r := validRequest(t)

	err := r.Resolve(uuid.New(), strings.Repeat("x", 1001), "resolver")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrInvalidResolution))
}

// =============================================================================
// Reject
// =============================================================================

func TestRequest_Reject_Success(t *testing.T) {
	r := validRequest(t)

	err := r.Reject("Does not meet minimum specifications.", "manager")

	require.NoError(t, err)
	assert.Equal(t, prdrequest.StatusRejected, r.Status())
	assert.Equal(t, "Does not meet minimum specifications.", r.RejectReason().String())
	assert.NotNil(t, r.UpdatedAt())
	assert.Equal(t, "manager", r.UpdatedBy())
}

func TestRequest_Reject_AlreadyRejected_Fails(t *testing.T) {
	r := validRequest(t)
	require.NoError(t, r.Reject("Initial rejection reason here.", "manager"))

	err := r.Reject("Another reason", "manager")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrAlreadyRejected))
}

func TestRequest_Reject_OnResolved_Fails(t *testing.T) {
	r := validRequest(t)
	require.NoError(t, r.Resolve(uuid.New(), "", "resolver"))

	err := r.Reject("Late rejection attempt.", "manager")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrAlreadyResolved))
}

func TestRequest_Reject_ShortReason_Fails(t *testing.T) {
	r := validRequest(t)

	// Less than 5 chars (trimmed).
	err := r.Reject("No", "manager")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrInvalidRejectReason))
}

func TestRequest_Reject_ReasonTooLong_Fails(t *testing.T) {
	r := validRequest(t)

	err := r.Reject(strings.Repeat("x", 1001), "manager")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrInvalidRejectReason))
}

// =============================================================================
// Update
// =============================================================================

func TestRequest_Update_Success(t *testing.T) {
	r := validRequest(t)
	due := time.Now().Add(48 * time.Hour)

	err := r.Update("Updated title text", "New description here", `{"key":"val"}`, &due, "editor")

	require.NoError(t, err)
	assert.Equal(t, "Updated title text", r.Title().String())
	assert.Equal(t, "New description here", r.Description().String())
	assert.Equal(t, `{"key":"val"}`, r.TargetSpecs().String())
	assert.NotNil(t, r.DueDate())
	assert.NotNil(t, r.UpdatedAt())
	assert.Equal(t, "editor", r.UpdatedBy())
}

func TestRequest_Update_OnTerminal_Fails(t *testing.T) {
	r := validRequest(t)
	require.NoError(t, r.Resolve(uuid.New(), "", "resolver"))

	err := r.Update("New title text here", "", "", nil, "editor")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrInvalidTransition))
}

func TestRequest_Update_InvalidTitle_Fails(t *testing.T) {
	r := validRequest(t)

	// Too short (2 chars).
	err := r.Update("ab", "", "", nil, "editor")

	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrInvalidTitle))
}

// =============================================================================
// SoftDelete
// =============================================================================

func TestRequest_SoftDelete_Twice(t *testing.T) {
	r := validRequest(t)

	err := r.SoftDelete("admin")
	require.NoError(t, err)
	assert.True(t, r.IsDeleted())
	assert.NotNil(t, r.DeletedAt())
	assert.Equal(t, "admin", r.DeletedBy())

	// Second call must return ErrNotFound.
	err = r.SoftDelete("admin")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrNotFound))
}

// =============================================================================
// TicketStatus helpers
// =============================================================================

func TestTicketStatus_Methods(t *testing.T) {
	tests := []struct {
		status     prdrequest.TicketStatus
		isValid    bool
		isTerminal bool
		canAssign  bool
		canResolve bool
		canReject  bool
	}{
		{prdrequest.StatusOpen, true, false, true, true, true},
		{prdrequest.StatusInReview, true, false, true, true, true},
		{prdrequest.StatusProductProposed, true, false, true, true, true},
		{prdrequest.StatusResolved, true, true, false, false, false},
		{prdrequest.StatusRejected, true, true, false, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.status.String(), func(t *testing.T) {
			assert.Equal(t, tc.isValid, tc.status.IsValid())
			assert.Equal(t, tc.isTerminal, tc.status.IsTerminal())
			assert.Equal(t, tc.canAssign, tc.status.CanAssign())
			assert.Equal(t, tc.canResolve, tc.status.CanResolve())
			assert.Equal(t, tc.canReject, tc.status.CanReject())
		})
	}
}

func TestTicketStatus_InvalidStatus(t *testing.T) {
	ts := prdrequest.TicketStatus("UNKNOWN")
	assert.False(t, ts.IsValid())
}

func TestNewTicketStatus_Invalid(t *testing.T) {
	_, err := prdrequest.NewTicketStatus("BOGUS")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrInvalidStatus))
}

// =============================================================================
// ReconstructRequest
// =============================================================================

func TestReconstructRequest(t *testing.T) {
	id := uuid.New()
	requesterID := uuid.New()
	deptID := uuid.New()
	productID := uuid.New()
	now := time.Now()
	updatedAt := now.Add(time.Hour)
	due := now.Add(72 * time.Hour)

	r := prdrequest.ReconstructRequest(
		id,
		"PR-202504-001",
		requesterID, "john.doe",
		deptID, "DEPT-A",
		"Request title", "Description text", `{"spec":"v"}`,
		"RESOLVED",
		productID,
		"Resolution note.",
		"",
		uuid.Nil,
		&due,
		now, "admin",
		&updatedAt, "editor",
		nil, "",
	)

	require.NotNil(t, r)
	assert.Equal(t, id, r.ID())
	assert.Equal(t, "PR-202504-001", r.TicketNo().String())
	assert.Equal(t, requesterID, r.RequesterID())
	assert.Equal(t, "john.doe", r.RequesterUsername())
	assert.Equal(t, deptID, r.RequesterDeptID())
	assert.Equal(t, prdrequest.StatusResolved, r.Status())
	assert.Equal(t, productID, r.ResolvedProductID())
	assert.Equal(t, "Resolution note.", r.ResolutionNote().String())
	assert.True(t, r.RejectReason().IsEmpty())
	assert.NotNil(t, r.DueDate())
	assert.Equal(t, "admin", r.CreatedBy())
	assert.Equal(t, "editor", r.UpdatedBy())
	assert.False(t, r.IsDeleted())
}
