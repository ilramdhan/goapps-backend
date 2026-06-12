package costfillassignment

import "testing"

func TestNewActorRef_Valid(t *testing.T) {
	ref, err := NewActorRef("DEPT", "RND")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Type() != ActorDept || ref.Value() != "RND" {
		t.Fatalf("got type=%s value=%s", ref.Type(), ref.Value())
	}
}

func TestNewActorRef_InvalidType(t *testing.T) {
	if _, err := NewActorRef("TEAM", "RND"); err == nil {
		t.Fatal("expected error for invalid actor type")
	}
}

func TestNewActorRef_EmptyValue(t *testing.T) {
	if _, err := NewActorRef("USER", ""); err == nil {
		t.Fatal("expected error for empty value")
	}
}
