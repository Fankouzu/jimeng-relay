package testharness

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
)

const (
	StatusInQueue    = "in_queue"
	StatusGenerating = "generating"
	StatusDone       = "done"
	StatusNotFound   = "not_found"
	StatusExpired    = "expired"
	StatusFailed     = "failed"
)

type BlockingGate struct {
	once sync.Once
	ch   chan struct{}
}

func NewBlockingGate() *BlockingGate {
	return &BlockingGate{ch: make(chan struct{})}
}

func (g *BlockingGate) Wait(ctx context.Context) error {
	if g == nil {
		return nil
	}
	select {
	case <-g.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *BlockingGate) Release() {
	if g == nil {
		return
	}
	g.once.Do(func() {
		close(g.ch)
	})
}

func (g *BlockingGate) Done() <-chan struct{} {
	if g == nil {
		return nil
	}
	return g.ch
}

type StatusTransition struct {
	Status   string
	Code     int
	Message  string
	VideoURL string
}

func StatusTransitions(finalStatus string) []StatusTransition {
	final := finalStatus
	if final == "" {
		final = StatusDone
	}
	return []StatusTransition{
		{Status: StatusInQueue, Code: 10000, Message: "ok"},
		{Status: StatusGenerating, Code: 10000, Message: "ok"},
		{Status: final, Code: 10000, Message: "ok"},
	}
}

func StatusTransitionsDone(videoURL string) []StatusTransition {
	out := StatusTransitions(StatusDone)
	out[len(out)-1].VideoURL = videoURL
	return out
}

func StatusTransitionsFailed(message string) []StatusTransition {
	out := StatusTransitions(StatusFailed)
	if message != "" {
		out[len(out)-1].Message = message
	}
	return out
}

type MockHTTPResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func NewJSONStatusResponses(transitions []StatusTransition) []MockHTTPResponse {
	if len(transitions) == 0 {
		transitions = StatusTransitions(StatusDone)
	}
	out := make([]MockHTTPResponse, 0, len(transitions))
	for _, tr := range transitions {
		data := map[string]any{"status": tr.Status}
		if tr.VideoURL != "" {
			data["video_url"] = tr.VideoURL
		}
		payload := map[string]any{
			"code":    tr.Code,
			"message": tr.Message,
			"data":    data,
		}
		body, _ := json.Marshal(payload)
		out = append(out, MockHTTPResponse{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       body,
		})
	}
	return out
}

type MockUpstreamServer struct {
	Server *httptest.Server
	Gate   *BlockingGate

	responses []MockHTTPResponse

	callCount   atomic.Int32
	inFlight    atomic.Int32
	maxInFlight atomic.Int32

	started chan struct{}
	mu      sync.Mutex
}

func NewMockUpstreamServer(responses []MockHTTPResponse, gate *BlockingGate) *MockUpstreamServer {
	m := &MockUpstreamServer{
		Gate:      gate,
		started:   make(chan struct{}),
		responses: append([]MockHTTPResponse(nil), responses...),
	}
	if len(m.responses) == 0 {
		m.responses = []MockHTTPResponse{{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       []byte(`{"code":10000,"message":"ok"}`),
		}}
	}
	m.Server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *MockUpstreamServer) Close() {
	if m == nil || m.Server == nil {
		return
	}
	m.Server.Close()
}

func (m *MockUpstreamServer) URL() string {
	if m == nil || m.Server == nil {
		return ""
	}
	return m.Server.URL
}

func (m *MockUpstreamServer) Started() <-chan struct{} {
	if m == nil {
		return nil
	}
	return m.started
}

func (m *MockUpstreamServer) CallCount() int {
	if m == nil {
		return 0
	}
	return int(m.callCount.Load())
}

func (m *MockUpstreamServer) MaxInFlight() int {
	if m == nil {
		return 0
	}
	return int(m.maxInFlight.Load())
}

func (m *MockUpstreamServer) handle(w http.ResponseWriter, r *http.Request) {
	idx := int(m.callCount.Add(1)) - 1

	m.mu.Lock()
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	resp := m.responses[idx]
	m.mu.Unlock()

	curInFlight := m.inFlight.Add(1)
	for {
		seen := m.maxInFlight.Load()
		if curInFlight <= seen {
			break
		}
		if m.maxInFlight.CompareAndSwap(seen, curInFlight) {
			break
		}
	}
	defer m.inFlight.Add(-1)

	select {
	case <-m.started:
	default:
		close(m.started)
	}

	if err := m.Gate.Wait(r.Context()); err != nil {
		w.WriteHeader(http.StatusGatewayTimeout)
		return
	}

	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	statusCode := resp.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)
	_, _ = w.Write(resp.Body)
}
