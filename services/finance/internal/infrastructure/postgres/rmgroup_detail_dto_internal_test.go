package postgres

import (
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

func TestDetailDTOToEntityEmptyItemCode(t *testing.T) {
	dummy := detailDTO{ItemCode: "", IsDummy: true}
	got, err := dummy.toEntity()
	if err != nil {
		t.Fatalf("dummy row with empty item_code: unexpected error %v", err)
	}
	if !got.ItemCode().IsEmpty() || !got.IsDummy() {
		t.Fatalf("dummy row: want empty item code and IsDummy, got %q dummy=%v", got.ItemCode().String(), got.IsDummy())
	}

	real := detailDTO{ItemCode: "  ", IsDummy: false}
	if _, err := real.toEntity(); !errors.Is(err, rmgroup.ErrEmptyItemCode) {
		t.Fatalf("non-dummy row with empty item_code: want ErrEmptyItemCode, got %v", err)
	}
}
