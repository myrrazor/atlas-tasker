package contracts

import "testing"

func TestCollaboratorIDValidationRejectsPathTraversal(t *testing.T) {
	for _, id := range []string{"alice", "rev-1", "Builder_2", "A"} {
		if !IsValidCollaboratorID(id) {
			t.Fatalf("expected collaborator id %q to be valid", id)
		}
	}
	for _, id := range []string{"", "../EVIL", "../../tmp/PWNED", "alice/bob", "alice bob", "~alice", ".alice"} {
		if IsValidCollaboratorID(id) {
			t.Fatalf("expected collaborator id %q to be invalid", id)
		}
	}
}
