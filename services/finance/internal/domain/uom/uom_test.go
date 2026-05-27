package uom_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

func TestNewCode_ValidInput_ReturnsCode(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple uppercase", "KG", "KG"},
		{"with underscore", "MTR_SQ", "MTR_SQ"},
		{"with numbers", "M3", "M3"},
		{"max length", "ABCDEFGHIJKLMNOPQRST", "ABCDEFGHIJKLMNOPQRST"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			code, err := uom.NewCode(tc.input)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if code.String() != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, code.String())
			}
		})
	}
}

func TestNewCode_InvalidInput_ReturnsError(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"lowercase", "kg"},
		{"starts with number", "1KG"},
		{"special chars", "KG@"},
		{"spaces", "KG KG"},
		{"too long", "ABCDEFGHIJKLMNOPQRSTU"}, // 21 chars
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := uom.NewCode(tc.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestNewUOM_ValidInput_ReturnsUOM(t *testing.T) {
	code, _ := uom.NewCode("KG")
	categoryID := uuid.New()

	entity, err := uom.NewUOM(code, "Kilogram", categoryID, "Weight in kg", "admin")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if entity.Code() != code {
		t.Errorf("expected code %s, got %s", code, entity.Code())
	}
	if entity.Name() != "Kilogram" {
		t.Errorf("expected name Kilogram, got %s", entity.Name())
	}
	if entity.CategoryID() != categoryID {
		t.Errorf("expected category ID %s, got %s", categoryID, entity.CategoryID())
	}
	if !entity.IsActive() {
		t.Error("expected entity to be active")
	}
	if entity.IsDeleted() {
		t.Error("expected entity to not be deleted")
	}
}

func TestNewUOM_EmptyName_ReturnsError(t *testing.T) {
	code, _ := uom.NewCode("KG")
	categoryID := uuid.New()

	_, err := uom.NewUOM(code, "", categoryID, "desc", "admin")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != uom.ErrEmptyName {
		t.Errorf("expected ErrEmptyName, got %v", err)
	}
}

func TestNewUOM_EmptyCreatedBy_ReturnsError(t *testing.T) {
	code, _ := uom.NewCode("KG")
	categoryID := uuid.New()

	_, err := uom.NewUOM(code, "Kilogram", categoryID, "desc", "")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != uom.ErrEmptyCreatedBy {
		t.Errorf("expected ErrEmptyCreatedBy, got %v", err)
	}
}

func TestNewUOM_NilCategoryID_ReturnsError(t *testing.T) {
	code, _ := uom.NewCode("KG")

	_, err := uom.NewUOM(code, "Kilogram", uuid.Nil, "desc", "admin")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != uom.ErrInvalidCategory {
		t.Errorf("expected ErrInvalidCategory, got %v", err)
	}
}

func TestUOM_Update_Success(t *testing.T) {
	code, _ := uom.NewCode("KG")
	categoryID := uuid.New()
	entity, _ := uom.NewUOM(code, "Kilogram", categoryID, "desc", "admin")

	newName := "Kilogram Updated"
	newCategoryID := uuid.New()
	newDesc := "updated desc"
	newActive := false

	err := entity.Update(&newName, &newCategoryID, &newDesc, &newActive, "updater")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if entity.Name() != newName {
		t.Errorf("expected name %s, got %s", newName, entity.Name())
	}
	if entity.CategoryID() != newCategoryID {
		t.Errorf("expected category ID %s, got %s", newCategoryID, entity.CategoryID())
	}
	if entity.Description() != newDesc {
		t.Errorf("expected description %s, got %s", newDesc, entity.Description())
	}
	if entity.IsActive() != newActive {
		t.Errorf("expected active %v, got %v", newActive, entity.IsActive())
	}
	if entity.UpdatedAt() == nil {
		t.Error("expected updated_at to be set")
	}
	if entity.UpdatedBy() == nil || *entity.UpdatedBy() != "updater" {
		t.Error("expected updated_by to be 'updater'")
	}
}

func TestUOM_SoftDelete_Success(t *testing.T) {
	code, _ := uom.NewCode("KG")
	categoryID := uuid.New()
	entity, _ := uom.NewUOM(code, "Kilogram", categoryID, "desc", "admin")

	err := entity.SoftDelete("deleter")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !entity.IsDeleted() {
		t.Error("expected entity to be deleted")
	}
	if entity.IsActive() {
		t.Error("expected entity to be inactive after delete")
	}
	if entity.DeletedAt() == nil {
		t.Error("expected deleted_at to be set")
	}
	if entity.DeletedBy() == nil || *entity.DeletedBy() != "deleter" {
		t.Error("expected deleted_by to be 'deleter'")
	}
}

func TestUOM_SoftDelete_AlreadyDeleted_ReturnsError(t *testing.T) {
	code, _ := uom.NewCode("KG")
	categoryID := uuid.New()
	entity, _ := uom.NewUOM(code, "Kilogram", categoryID, "desc", "admin")
	_ = entity.SoftDelete("deleter")

	err := entity.SoftDelete("another")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != uom.ErrAlreadyDeleted {
		t.Errorf("expected ErrAlreadyDeleted, got %v", err)
	}
}
