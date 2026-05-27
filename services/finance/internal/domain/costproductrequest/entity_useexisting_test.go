package costproductrequest_test

import (
	"errors"
	"testing"

	cpr "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
)

// Helper: build a request that has progressed through SUBMITTED → UNDER_REVIEW
// and had its classification verified, so it's eligible for UseExistingCosting.
func underReviewExisting(t *testing.T) *cpr.Request {
	t.Helper()
	r, err := cpr.New(cpr.NewInput{
		RequestTypeID:         1,
		Title:                 "Reuse PTY 75/72 quote",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassExisting,
		UrgencyLevel:          cpr.UrgencyMedium,
		RequesterUserID:       "user-1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := r.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := r.StartReview(); err != nil {
		t.Fatalf("StartReview: %v", err)
	}
	if err := r.VerifyClassification(cpr.ClassExisting, ""); err != nil {
		t.Fatalf("VerifyClassification: %v", err)
	}
	return r
}

func TestUseExistingCosting_RequiresProduct(t *testing.T) {
	t.Parallel()

	r := underReviewExisting(t)
	err := r.UseExistingCosting(0)
	if !errors.Is(err, cpr.ErrExistingProductRequired) {
		t.Fatalf("want ErrExistingProductRequired, got %v", err)
	}
}

func TestUseExistingCosting_RecordsProductAndAdvances(t *testing.T) {
	t.Parallel()

	r := underReviewExisting(t)
	if err := r.UseExistingCosting(123); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if r.Status() != cpr.StatusQuoteReady {
		t.Fatalf("want QUOTE_READY status, got %s", r.Status())
	}
	if r.ExistingProductSysID() != 123 {
		t.Fatalf("want existing_product_sys_id=123, got %d", r.ExistingProductSysID())
	}
}

func TestUseExistingCosting_BlockedFromWrongState(t *testing.T) {
	t.Parallel()

	r, err := cpr.New(cpr.NewInput{
		RequestTypeID:         1,
		Title:                 "still draft",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassExisting,
		UrgencyLevel:          cpr.UrgencyMedium,
		RequesterUserID:       "user-1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := r.UseExistingCosting(1); !errors.Is(err, cpr.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition from DRAFT, got %v", err)
	}
}

func TestUseExistingCosting_BlockedWhenVerifiedIsNew(t *testing.T) {
	t.Parallel()

	r, err := cpr.New(cpr.NewInput{
		RequestTypeID:         1,
		Title:                 "verified new",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassNew,
		UrgencyLevel:          cpr.UrgencyMedium,
		RequesterUserID:       "user-1",
		Spec: &cpr.SpecInput{
			RawMaterialType:    "POY_BOUGHTOUT",
			ProductDescription: "test product",
			PaperTubeTypeID:    1,
			WeightPerBobbinKg:  "1.5",
			BoxType:            cpr.BoxTypeNormal,
			ShadeCustomText:    "BRIGHT",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := r.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := r.StartReview(); err != nil {
		t.Fatalf("StartReview: %v", err)
	}
	if err := r.VerifyClassification(cpr.ClassNew, ""); err != nil {
		t.Fatalf("VerifyClassification: %v", err)
	}
	if err := r.UseExistingCosting(99); !errors.Is(err, cpr.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition when verified=new, got %v", err)
	}
}
