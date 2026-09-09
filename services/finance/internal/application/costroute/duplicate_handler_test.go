package costroute_test

import (
	"context"
	"testing"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costroute"
	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

type fakeRepoForDup struct{}

func (fakeRepoForDup) PromoteFromDraft(_ context.Context, _ costroute.PromoteInput) (int64, error) {
	return 0, nil
}
func (fakeRepoForDup) GetActiveByProduct(_ context.Context, _ int64) (*costroute.Head, error) {
	return nil, costroute.ErrNotFound
}
func (fakeRepoForDup) GetHead(_ context.Context, _ int64) (*costroute.Head, error) {
	return nil, costroute.ErrNotFound
}
func (fakeRepoForDup) GetGraph(_ context.Context, _ int64) (*costroute.Graph, error) {
	return nil, costroute.ErrNotFound
}
func (fakeRepoForDup) SaveGraph(_ context.Context, _ int64, _ *costroute.Graph, _ string) (*costroute.Graph, error) {
	return nil, nil
}
func (fakeRepoForDup) SaveHead(_ context.Context, _ *costroute.Head, _ string) error { return nil }
func (fakeRepoForDup) DeleteHead(_ context.Context, _ int64, _ string) error         { return nil }
func (fakeRepoForDup) ListHeads(_ context.Context, _ costroute.Filter) ([]*costroute.Head, int64, error) {
	return nil, 0, nil
}
func (fakeRepoForDup) DuplicateRoute(_ context.Context, _ costroute.DuplicateInput) (costroute.DuplicateOutput, error) {
	return costroute.DuplicateOutput{NewHeadID: 99, NewProductSysID: 50, NewProductCode: "TEST_F1"}, nil
}
func (fakeRepoForDup) AttachRoute(_ context.Context, _ costroute.AttachInput) (costroute.AttachOutput, error) {
	return costroute.AttachOutput{NewHeadID: 199}, nil
}
func (fakeRepoForDup) ListLinkedRequests(_ context.Context, _ int64) ([]costroute.LinkedRequest, error) {
	return nil, nil
}
func (fakeRepoForDup) BulkUpsertHeads(_ context.Context, _ []costroute.HeadUpsertInput, _ string) ([]costroute.HeadUpsertResult, error) {
	return nil, nil
}
func (fakeRepoForDup) BulkUpsertSeqs(_ context.Context, _ []costroute.SeqUpsertInput, _ string) ([]costroute.SeqUpsertResult, error) {
	return nil, nil
}
func (fakeRepoForDup) BulkReplaceRMs(_ context.Context, _ int64, _ []costroute.RMInput, _ string) error {
	return nil
}
func (fakeRepoForDup) ListAllHeadsForExport(_ context.Context, _ []int64) ([]costroute.ExportRouteHead, error) {
	return nil, nil
}
func (fakeRepoForDup) ListAllSeqsForExport(_ context.Context, _ []int64) ([]costroute.ExportRouteSeq, error) {
	return nil, nil
}
func (fakeRepoForDup) ListAllRMsForExport(_ context.Context, _ []int64) ([]costroute.ExportRouteRM, error) {
	return nil, nil
}

func TestDuplicate_ValuesWithoutApplicabilityRejected(t *testing.T) {
	t.Parallel()
	h := app.NewDuplicateHandler(fakeRepoForDup{})
	_, err := h.Handle(context.Background(), costroute.DuplicateInput{
		SourceHeadID:         1,
		IncludeApplicability: false,
		IncludeValues:        true,
	})
	if err == nil {
		t.Fatal("expected error for values-without-applicability, got nil")
	}
}

func TestDuplicate_InvalidHeadIDRejected(t *testing.T) {
	t.Parallel()
	h := app.NewDuplicateHandler(fakeRepoForDup{})
	_, err := h.Handle(context.Background(), costroute.DuplicateInput{SourceHeadID: 0})
	if err == nil {
		t.Fatal("expected error for zero head id, got nil")
	}
}

func TestDuplicate_SameProductModeRejectsOtherFlags(t *testing.T) {
	t.Parallel()
	h := app.NewDuplicateHandler(fakeRepoForDup{})
	cases := []struct {
		name string
		in   costroute.DuplicateInput
	}{
		{"include_upstream", costroute.DuplicateInput{SourceHeadID: 1, TargetMode: costroute.DuplicateTargetModeSameProduct, IncludeUpstream: true}},
		{"include_applicability", costroute.DuplicateInput{SourceHeadID: 1, TargetMode: costroute.DuplicateTargetModeSameProduct, IncludeApplicability: true}},
		{"include_values (with applicability)", costroute.DuplicateInput{SourceHeadID: 1, TargetMode: costroute.DuplicateTargetModeSameProduct, IncludeApplicability: true, IncludeValues: true}},
		{"new_code_prefix", costroute.DuplicateInput{SourceHeadID: 1, TargetMode: costroute.DuplicateTargetModeSameProduct, NewCodePrefix: "X"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := h.Handle(context.Background(), tc.in)
			if err == nil {
				t.Fatalf("expected error for %s combined with SAME_PRODUCT mode, got nil", tc.name)
			}
		})
	}
}

func TestDuplicate_SameProductModeAcceptsMinimalRequest(t *testing.T) {
	t.Parallel()
	h := app.NewDuplicateHandler(fakeRepoForDup{})
	_, err := h.Handle(context.Background(), costroute.DuplicateInput{
		SourceHeadID:   1,
		TargetMode:     costroute.DuplicateTargetModeSameProduct,
		IncludeRouting: true,
	})
	if err != nil {
		t.Fatalf("expected minimal SAME_PRODUCT request to be accepted, got %v", err)
	}
}

func TestDuplicate_Happy(t *testing.T) {
	t.Parallel()
	h := app.NewDuplicateHandler(fakeRepoForDup{})
	out, err := h.Handle(context.Background(), costroute.DuplicateInput{
		SourceHeadID:         1,
		IncludeRouting:       true,
		IncludeUpstream:      true,
		IncludeApplicability: true,
		IncludeValues:        true,
	})
	if err != nil {
		t.Fatalf("ok expected, got %v", err)
	}
	if out.NewHeadID != 99 || out.NewProductSysID != 50 || out.NewProductCode != "TEST_F1" {
		t.Fatalf("unexpected output: %+v", out)
	}
}
