package costroute_test

import (
	"context"
	"testing"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costroute"
	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// fakeRepoForDup (attach_handler_test.go's shared use) already implements
// AttachRoute returning {NewHeadID: 199} -- see duplicate_handler_test.go.

func TestAttach_InvalidSourceHeadIDRejected(t *testing.T) {
	t.Parallel()
	h := app.NewAttachHandler(fakeRepoForDup{})
	_, err := h.Handle(context.Background(), costroute.AttachInput{
		SourceHeadID:       0,
		TargetProductSysID: 1,
	})
	if err == nil {
		t.Fatal("expected error for zero source head id, got nil")
	}
}

func TestAttach_InvalidTargetProductSysIDRejected(t *testing.T) {
	t.Parallel()
	h := app.NewAttachHandler(fakeRepoForDup{})
	_, err := h.Handle(context.Background(), costroute.AttachInput{
		SourceHeadID:       1,
		TargetProductSysID: 0,
	})
	if err == nil {
		t.Fatal("expected error for zero target product sys id, got nil")
	}
}

func TestAttach_Happy(t *testing.T) {
	t.Parallel()
	h := app.NewAttachHandler(fakeRepoForDup{})
	out, err := h.Handle(context.Background(), costroute.AttachInput{
		SourceHeadID:       1,
		TargetProductSysID: 2,
	})
	if err != nil {
		t.Fatalf("ok expected, got %v", err)
	}
	if out.NewHeadID != 199 {
		t.Fatalf("unexpected output: %+v", out)
	}
}
