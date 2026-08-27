package shade

import (
	"errors"
	"testing"
	"time"
)

func TestNew_ValidInput_Success(t *testing.T) {
	s, err := New(NewParams{Code: " nl ", Name: "Natural", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Code() != "NL" {
		t.Errorf("expected normalized code NL, got %q", s.Code())
	}
	if s.Source() != SourceManual {
		t.Errorf("expected source MANUAL, got %q", s.Source())
	}
	if !s.IsActive() {
		t.Error("expected new shade to be active")
	}
}

func TestNew_EmptyCode_ReturnsError(t *testing.T) {
	_, err := New(NewParams{Code: "  ", Name: "Natural", CreatedBy: "tester"})
	if !errors.Is(err, ErrEmptyCode) {
		t.Errorf("expected ErrEmptyCode, got %v", err)
	}
}

func TestNew_EmptyName_ReturnsError(t *testing.T) {
	_, err := New(NewParams{Code: "NL", Name: "  ", CreatedBy: "tester"})
	if !errors.Is(err, ErrEmptyName) {
		t.Errorf("expected ErrEmptyName, got %v", err)
	}
}

func TestNew_EmptyCreatedBy_ReturnsError(t *testing.T) {
	_, err := New(NewParams{Code: "NL", Name: "Natural", CreatedBy: "  "})
	if !errors.Is(err, ErrEmptyCreatedBy) {
		t.Errorf("expected ErrEmptyCreatedBy, got %v", err)
	}
}

func TestNew_CodeTooLong_ReturnsError(t *testing.T) {
	longCode := make([]byte, maxCodeLen+1)
	for i := range longCode {
		longCode[i] = 'A'
	}
	_, err := New(NewParams{Code: string(longCode), Name: "Natural", CreatedBy: "tester"})
	if !errors.Is(err, ErrCodeTooLong) {
		t.Errorf("expected ErrCodeTooLong, got %v", err)
	}
}

func TestUpdate_PartialFields_OnlyChangesProvided(t *testing.T) {
	s, err := New(NewParams{Code: "NL", Name: "Natural", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	inactive := false
	if err := s.Update(UpdateParams{IsActive: &inactive, UpdatedBy: "editor"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name() != "Natural" {
		t.Errorf("name should be unchanged, got %q", s.Name())
	}
	if s.IsActive() {
		t.Error("expected shade to be deactivated")
	}
	if s.UpdatedBy() == nil || *s.UpdatedBy() != "editor" {
		t.Error("expected updated_by to be set to editor")
	}
}

func TestUpdate_EmptyName_ReturnsError(t *testing.T) {
	s, err := New(NewParams{Code: "NL", Name: "Natural", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	empty := "   "
	if err := s.Update(UpdateParams{Name: &empty, UpdatedBy: "editor"}); !errors.Is(err, ErrEmptyName) {
		t.Errorf("expected ErrEmptyName, got %v", err)
	}
}

func TestNormalizeCode_TrimsAndUppercases(t *testing.T) {
	if got := NormalizeCode("  z114s  "); got != "Z114S" {
		t.Errorf("expected Z114S, got %q", got)
	}
}

func TestReconstruct_PreservesAllFields(t *testing.T) {
	now := time.Now()
	sourceBy := "ORCLUSR1"
	s := Reconstruct(ReconstructParams{
		ID: 7, Code: "NL", Name: "Natural", IsActive: true,
		Source: SourceOracle, SourceCreatedAt: &now, SourceCreatedBy: &sourceBy,
		SyncedAt: &now, CreatedAt: now, CreatedBy: "system",
	})
	if s.ID() != 7 || s.Code() != "NL" || s.Source() != SourceOracle {
		t.Errorf("reconstruct did not preserve fields: %+v", s)
	}
	if s.SourceCreatedBy() == nil || *s.SourceCreatedBy() != sourceBy {
		t.Error("expected source created_by to be preserved")
	}
}
