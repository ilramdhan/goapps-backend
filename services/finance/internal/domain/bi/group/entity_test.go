package group_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/group"
)

func TestNewGroup_HappyPath(t *testing.T) {
	g, err := group.NewGroup(group.NewGroupParams{
		Code: "FINANCE", Name: "Finance", Icon: "Wallet", DisplayOrder: 10, IsActive: true, CreatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if g.Code() != "FINANCE" || g.Name() != "Finance" {
		t.Errorf("unexpected: %s/%s", g.Code(), g.Name())
	}
}

func TestNewGroup_Validation(t *testing.T) {
	tests := []struct {
		name    string
		p       group.NewGroupParams
		wantErr error
	}{
		{"bad code", group.NewGroupParams{Code: "bad-code", Name: "X"}, group.ErrInvalidCode},
		{"empty code", group.NewGroupParams{Code: "", Name: "X"}, group.ErrInvalidCode},
		{"empty name", group.NewGroupParams{Code: "X", Name: ""}, group.ErrInvalidCode}, // code too short trips first
		{"empty name (code valid)", group.NewGroupParams{Code: "FX", Name: ""}, group.ErrInvalidName},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := group.NewGroup(tc.p)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestGroup_Update(t *testing.T) {
	g, _ := group.NewGroup(group.NewGroupParams{Code: "XY", Name: "Old", CreatedBy: uuid.New()})
	newName := "New Name"
	if err := g.Update(group.UpdateParams{Name: &newName, UpdatedBy: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if g.Name() != newName {
		t.Errorf("name not updated: %q", g.Name())
	}
}

func TestGroup_Update_InvalidNoMutation(t *testing.T) {
	g, _ := group.NewGroup(group.NewGroupParams{Code: "XY", Name: "Old", CreatedBy: uuid.New()})
	tooLong := string(make([]byte, 121))
	for i := range tooLong {
		tooLong = tooLong[:i] + "a" + tooLong[i+1:]
	}
	err := g.Update(group.UpdateParams{Name: &tooLong})
	if !errors.Is(err, group.ErrInvalidName) {
		t.Errorf("want ErrInvalidName, got %v", err)
	}
	if g.Name() != "Old" {
		t.Errorf("name leaked: %q", g.Name())
	}
}
