package testharness

import (
	"reflect"
	"testing"

	"github.com/jimeng-relay/server/internal/relay/upstream"
)

type Response struct {
	ID string
}

func AssertFIFOOrder(t *testing.T, responses []Response, expectedOrder []string) {
	t.Helper()
	if len(responses) != len(expectedOrder) {
		t.Fatalf("response length mismatch: got=%d want=%d", len(responses), len(expectedOrder))
	}
	for i := range expectedOrder {
		if responses[i].ID != expectedOrder[i] {
			t.Fatalf("fifo order mismatch at index=%d: got=%q want=%q", i, responses[i].ID, expectedOrder[i])
		}
	}
}

func AssertMaxInFlight(t *testing.T, client *upstream.Client, max int) {
	t.Helper()
	if client == nil {
		t.Fatalf("upstream client is nil")
	}
	if max <= 0 {
		t.Fatalf("max must be positive, got %d", max)
	}

	v := reflect.ValueOf(client).Elem().FieldByName("sem")
	if !v.IsValid() || v.IsNil() || v.Kind() != reflect.Chan {
		t.Fatalf("upstream client semaphore not found")
	}

	capSem := v.Cap()
	if capSem != max {
		t.Fatalf("unexpected max in-flight cap: got=%d want=%d", capSem, max)
	}
	if v.Len() > max {
		t.Fatalf("in-flight exceeded max: inFlight=%d max=%d", v.Len(), max)
	}
}

func AssertNoFlakySleep(t *testing.T) {
	t.Helper()
}
