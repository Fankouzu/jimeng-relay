package testharness

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jimeng-relay/server/internal/config"
	"github.com/jimeng-relay/server/internal/relay/upstream"
)

func TestStatusTransitions(t *testing.T) {
	seq := StatusTransitions(StatusDone)
	if len(seq) != 3 {
		t.Fatalf("unexpected sequence length: got=%d", len(seq))
	}
	if seq[0].Status != StatusInQueue || seq[1].Status != StatusGenerating || seq[2].Status != StatusDone {
		t.Fatalf("unexpected status sequence: %#v", seq)
	}
}

func TestDeterministicWaiter(t *testing.T) {
	seq := NewTransitionSequence().InQueue().Generating().Done("https://example.com/a.mp4").Build()
	w := NewDeterministicWaiter(seq)

	first := w.Next()
	second := w.Next()
	third := w.Next()
	fourth := w.Next()

	if first.Status != StatusInQueue || second.Status != StatusGenerating || third.Status != StatusDone {
		t.Fatalf("unexpected deterministic waiter transition order")
	}
	if fourth.Status != StatusDone {
		t.Fatalf("waiter should keep returning terminal state, got=%s", fourth.Status)
	}
	if w.PollCount() != 4 {
		t.Fatalf("unexpected poll count: got=%d want=4", w.PollCount())
	}
}

func TestMockUpstreamServerBlockingGate(t *testing.T) {
	gate := NewBlockingGate()
	seq := NewTransitionSequence().InQueue().Generating().Done("https://example.com/video.mp4").Build()
	srv := NewMockUpstreamServer(NewJSONStatusResponses(seq), gate)
	t.Cleanup(srv.Close)

	client := &http.Client{Timeout: 2 * time.Second}

	done := make(chan struct {
		status int
		body   []byte
		err    error
	}, 1)
	go func() {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL(), bytes.NewReader([]byte(`{}`)))
		if err != nil {
			done <- struct {
				status int
				body   []byte
				err    error
			}{err: err}
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			done <- struct {
				status int
				body   []byte
				err    error
			}{err: err}
			return
		}
		defer resp.Body.Close()
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		done <- struct {
			status int
			body   []byte
			err    error
		}{status: resp.StatusCode, body: buf.Bytes()}
	}()

	select {
	case <-srv.Started():
	case <-time.After(2 * time.Second):
		t.Fatalf("request did not reach upstream mock")
	}

	select {
	case <-done:
		t.Fatalf("request should still be blocked before gate release")
	default:
	}

	gate.Release()

	select {
	case out := <-done:
		if out.err != nil {
			t.Fatalf("request failed: %v", out.err)
		}
		if out.status != http.StatusOK {
			t.Fatalf("unexpected status: got=%d want=%d", out.status, http.StatusOK)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("request did not finish after gate release")
	}

	if srv.CallCount() != 1 {
		t.Fatalf("unexpected call count: got=%d want=1", srv.CallCount())
	}
}

func TestAssertFIFOOrder(t *testing.T) {
	responses := []Response{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	AssertFIFOOrder(t, responses, []string{"a", "b", "c"})
}

func TestAssertMaxInFlight(t *testing.T) {
	gate := NewBlockingGate()
	srv := NewMockUpstreamServer([]MockHTTPResponse{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"code":10000,"message":"ok"}`),
	}}, gate)
	t.Cleanup(srv.Close)

	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL(),
		Timeout:     2 * time.Second,
	}, upstream.Options{
		MaxConcurrent: 2,
		MaxQueue:      10,
	})
	if err != nil {
		t.Fatalf("upstream.NewClient: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_, _ = c.Submit(context.Background(), []byte(`{"prompt":"p"}`), http.Header{"X-Request-Id": []string{"rid"}})
		}()
	}

	select {
	case <-srv.Started():
	case <-time.After(2 * time.Second):
		t.Fatalf("request did not reach upstream mock")
	}

	AssertMaxInFlight(t, c, 2)
	AssertNoFlakySleep(t)

	gate.Release()
	wg.Wait()
}
