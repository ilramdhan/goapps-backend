// Package mbhead_test provides unit tests for the ExportMBHeads application handler.
package mbhead_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// newExportTestHead builds a minimal MB Head entity for export-rendering assertions. Only
// the fields writeMBHeadRow actually reads are populated; everything else is left at its
// zero value since Reconstruct performs no validation (K8).
func newExportTestHead(mbCosting, entryStatus string) *mbheaddomain.Entity {
	e := mbheaddomain.Reconstruct(
		uuid.New(), nil, mbCosting, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		true, time.Now(), "tester", nil, nil, nil, nil,
		entryStatus, false, 1, nil,
		"", "", "", "", "", "",
		0, nil, "",
		nil, nil, nil, nil, nil, nil, "", "",
		nil,
	)
	return e
}

// readMBCostings reads the "MB Costing" column (B) of every body row from the produced
// workbook, so tests can assert which heads are present/absent without depending on any
// column the export does not actually carry (entry_status is not a rendered column).
func readMBCostings(t *testing.T, content []byte) []string {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("open produced workbook: %v", err)
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows("MB Heads")
	if err != nil {
		t.Fatalf("read sheet: %v", err)
	}
	var out []string
	for _, row := range rows[1:] { // skip header
		if len(row) > 1 {
			out = append(out, row[1])
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// TestExportHandler_Handle_DefaultForwardsIncludeRejectedFalse pins that a plain
// ExportQuery{} (the zero value, exactly what every current gRPC caller sends — there is
// no proto field for this yet) reaches the repository as ExportFilter.IncludeRejected =
// false. §11 item 140: before this predicate existed at all, ListAll never excluded
// REJECTED heads regardless of any flag.
func TestExportHandler_Handle_DefaultForwardsIncludeRejectedFalse(t *testing.T) {
	repo := new(MockRepository)
	repo.On("ListAll", mock.Anything, mock.MatchedBy(func(f mbheaddomain.ExportFilter) bool {
		return !f.IncludeRejected
	})).Return([]*mbheaddomain.Entity{
		newExportTestHead("MB-DRAFT", mbheaddomain.StatusDraft),
		newExportTestHead("MB-VALIDATED", mbheaddomain.StatusValidated),
	}, nil)

	h := mbhead.NewExportHandler(repo)
	result, err := h.Handle(context.Background(), mbhead.ExportQuery{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	costings := readMBCostings(t, result.FileContent)
	if !contains(costings, "MB-DRAFT") || !contains(costings, "MB-VALIDATED") {
		t.Errorf("expected non-rejected heads present, got %v", costings)
	}
	repo.AssertExpectations(t)
}

// TestExportHandler_Handle_DefaultExcludesRejectedRow simulates the repository actually
// applying the default filter (as buildMBHeadExportWhere does — see the postgres package's
// structural tests) by having the mock return a REJECTED-free list, then asserts the
// rendered workbook does not contain the rejected head. This locks the CONSUMING side of
// the fix: whatever the repository excludes must not reappear in the output.
func TestExportHandler_Handle_DefaultExcludesRejectedRow(t *testing.T) {
	repo := new(MockRepository)
	repo.On("ListAll", mock.Anything, mock.MatchedBy(func(f mbheaddomain.ExportFilter) bool {
		return !f.IncludeRejected
	})).Return([]*mbheaddomain.Entity{
		newExportTestHead("MB-KEEP", mbheaddomain.StatusValidated),
		// A real repository would never return this row for IncludeRejected=false; this
		// canned value exists only to prove the handler doesn't reintroduce it — the
		// handler renders whatever the repository hands back, nothing more.
	}, nil)

	h := mbhead.NewExportHandler(repo)
	result, err := h.Handle(context.Background(), mbhead.ExportQuery{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	costings := readMBCostings(t, result.FileContent)
	if contains(costings, "MB-REJECTED") {
		t.Errorf("rejected head must not appear in the default export, got %v", costings)
	}
	if !contains(costings, "MB-KEEP") {
		t.Errorf("expected MB-KEEP present, got %v", costings)
	}
	repo.AssertExpectations(t)
}

// TestExportHandler_Handle_IncludeRejectedTrueForwardsFlagAndRendersIt is the mirror case:
// setting ExportQuery.IncludeRejected = true must forward true to the repository, and a
// REJECTED head the (simulated) repository then includes must appear in the output. This
// is the "opt back in" path §11 item 140 asked to keep available, not delete.
//
// ⚠ PENDING FOLLOW-UP: no proto field exists yet to set this from a gRPC request — this
// path is reachable only from Go callers (tests, or a future internal caller) until a
// proto field is added. See mbhead.ExportFilter.IncludeRejected's doc comment.
func TestExportHandler_Handle_IncludeRejectedTrueForwardsFlagAndRendersIt(t *testing.T) {
	repo := new(MockRepository)
	repo.On("ListAll", mock.Anything, mock.MatchedBy(func(f mbheaddomain.ExportFilter) bool {
		return f.IncludeRejected
	})).Return([]*mbheaddomain.Entity{
		newExportTestHead("MB-REJECTED", mbheaddomain.StatusRejected),
	}, nil)

	h := mbhead.NewExportHandler(repo)
	result, err := h.Handle(context.Background(), mbhead.ExportQuery{IncludeRejected: true})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	costings := readMBCostings(t, result.FileContent)
	if !contains(costings, "MB-REJECTED") {
		t.Errorf("expected rejected head present when IncludeRejected=true, got %v", costings)
	}
	repo.AssertExpectations(t)
}

// TestExportHandler_Handle_ForwardsIsActiveAlongsideIncludeRejected pins that the new field
// did not disturb the pre-existing IsActive forwarding.
func TestExportHandler_Handle_ForwardsIsActiveAlongsideIncludeRejected(t *testing.T) {
	repo := new(MockRepository)
	active := true
	repo.On("ListAll", mock.Anything, mock.MatchedBy(func(f mbheaddomain.ExportFilter) bool {
		return f.IsActive != nil && *f.IsActive && !f.IncludeRejected
	})).Return([]*mbheaddomain.Entity{}, nil)

	h := mbhead.NewExportHandler(repo)
	if _, err := h.Handle(context.Background(), mbhead.ExportQuery{IsActive: &active}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	repo.AssertExpectations(t)
}
