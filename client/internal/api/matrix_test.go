package api

import "testing"

func TestApiMatrix(t *testing.T) {
	matrix := FrozenApiMatrix()

	if len(matrix.ActionOrder) != 2 {
		t.Fatalf("expected 2 actions in frozen matrix, got %d", len(matrix.ActionOrder))
	}

	for _, action := range []string{ActionSubmitTask, ActionGetResult} {
		spec, ok := matrix.Actions[action]
		if !ok {
			t.Fatalf("expected action %q in matrix", action)
		}

		if spec.ReqKey != ReqKeyJimengT2IV40 {
			t.Fatalf("expected req_key %q for %s, got %q", ReqKeyJimengT2IV40, action, spec.ReqKey)
		}

		expectedStatuses := map[string]bool{
			StatusInQueue:   true,
			StatusGenerating: true,
			StatusDone:      true,
			StatusNotFound:  true,
			StatusExpired:   true,
			StatusFailed:    true,
		}

		if len(spec.AllowedStatuses) != len(expectedStatuses) {
			t.Fatalf("expected %d statuses for %s, got %d", len(expectedStatuses), action, len(spec.AllowedStatuses))
		}

		for _, status := range spec.AllowedStatuses {
			if !expectedStatuses[status] {
				t.Fatalf("unexpected status %q for %s", status, action)
			}
		}
	}
}

func TestApiMatrixRejectsInvalidAction(t *testing.T) {
	matrix := FrozenApiMatrix()

	err := matrix.ValidateAction("CVProcess")
	if err == nil {
		t.Fatalf("expected invalid action to be rejected")
	}
}
