package audit

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	internalerrors "github.com/jimeng-relay/server/internal/errors"
	"github.com/jimeng-relay/server/internal/models"
)

type fakeDownstreamRepo struct {
	called  int
	created []models.DownstreamRequest
	err     error
}

func (f *fakeDownstreamRepo) Create(_ context.Context, request models.DownstreamRequest) error {
	f.called++
	if f.err != nil {
		return f.err
	}
	f.created = append(f.created, request)
	return nil
}

func (f *fakeDownstreamRepo) GetByID(_ context.Context, _ string) (models.DownstreamRequest, error) {
	return models.DownstreamRequest{}, errors.New("not implemented")
}

func (f *fakeDownstreamRepo) GetByRequestID(_ context.Context, _ string) (models.DownstreamRequest, error) {
	return models.DownstreamRequest{}, errors.New("not implemented")
}

type fakeUpstreamRepo struct {
	called  int
	created []models.UpstreamAttempt
	err     error
}

func (f *fakeUpstreamRepo) Create(_ context.Context, attempt models.UpstreamAttempt) error {
	f.called++
	if f.err != nil {
		return f.err
	}
	f.created = append(f.created, attempt)
	return nil
}

func (f *fakeUpstreamRepo) ListByRequestID(_ context.Context, _ string) ([]models.UpstreamAttempt, error) {
	return nil, errors.New("not implemented")
}

type fakeAuditRepo struct {
	called  int
	created []models.AuditEvent
	err     error
}

func (f *fakeAuditRepo) Create(_ context.Context, event models.AuditEvent) error {
	f.called++
	if f.err != nil {
		return f.err
	}
	f.created = append(f.created, event)
	return nil
}

func (f *fakeAuditRepo) ListByRequestID(_ context.Context, _ string) ([]models.AuditEvent, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAuditRepo) ListByTimeRange(_ context.Context, _, _ time.Time) ([]models.AuditEvent, error) {
	return nil, errors.New("not implemented")
}

func TestService_RecordRelayCall_Success_WritesChainAndRedacts(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)

	ds := &fakeDownstreamRepo{}
	us := &fakeUpstreamRepo{}
	ar := &fakeAuditRepo{}

	const randomBytes = 24
	rnd := bytes.NewReader(bytes.Repeat([]byte{0x01}, randomBytes))
	svc := NewService(ds, us, ar, Config{Now: func() time.Time { return base }, Random: rnd})

	err := svc.RecordRelayCall(ctx, RelayCall{
		RequestID: "req-1",
		APIKeyID:  "k1",
		Action:    models.DownstreamActionCVSync2AsyncSubmitTask,
		Method:    "POST",
		Path:      "/v1/submit",
		Query:     "a=1",
		ClientIP:  "127.0.0.1",
		DownstreamHeaders: map[string]any{
			"Authorization":   "Bearer abcdef",
			"X-Amz-Signature": "sig",
			"Content-Type":    "application/json",
			"meta":            map[string]any{"sk": "sk_plain"},
		},
		DownstreamBody: map[string]any{"prompt": "x"},
		Upstream: UpstreamAttempt{
			AttemptNumber:  1,
			UpstreamAction: "CVSync2AsyncSubmitTask",
			RequestHeaders: map[string]any{"authorization": "AWS4-HMAC-SHA256 ...", "signature": "plain"},
			RequestBody:    map[string]any{"foo": "bar"},
			ResponseStatus: 200,
			ResponseHeaders: map[string]any{
				"Content-Type":     "application/json",
				"X-Security-Token": "token",
			},
			ResponseBody: map[string]any{"ok": true},
			LatencyMs:    123,
		},
		Events: []Event{
			{
				Type:     models.EventTypeUpstreamResponse,
				Actor:    "system",
				Action:   "upstream_response",
				Resource: "relay.upstream",
				Metadata: map[string]any{"signature": "plain"},
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordRelayCall: %v", err)
	}

	if ds.called != 1 {
		t.Fatalf("expected 1 downstream Create call, got %d", ds.called)
	}
	if us.called != 1 {
		t.Fatalf("expected 1 upstream Create call, got %d", us.called)
	}
	if ar.called != 1 {
		t.Fatalf("expected 1 audit Create call, got %d", ar.called)
	}

	if len(ds.created) != 1 || ds.created[0].RequestID != "req-1" {
		t.Fatalf("unexpected downstream request")
	}
	if ds.created[0].Headers["Authorization"] != "***" {
		t.Fatalf("expected downstream Authorization redacted")
	}
	if ds.created[0].Headers["X-Amz-Signature"] != "***" {
		t.Fatalf("expected downstream signature header redacted")
	}
	if ds.created[0].Headers["Content-Type"] != "application/json" {
		t.Fatalf("expected downstream Content-Type preserved")
	}
	meta, ok := ds.created[0].Headers["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta header map, got %T", ds.created[0].Headers["meta"])
	}
	if meta["sk"] != "***" {
		t.Fatalf("expected nested sk redacted")
	}

	if len(us.created) != 1 || us.created[0].RequestID != "req-1" {
		t.Fatalf("unexpected upstream attempt")
	}
	if us.created[0].RequestHeaders["authorization"] != "***" {
		t.Fatalf("expected upstream request authorization redacted")
	}
	if us.created[0].RequestHeaders["signature"] != "***" {
		t.Fatalf("expected upstream request signature redacted")
	}
	if us.created[0].ResponseHeaders["X-Security-Token"] != "***" {
		t.Fatalf("expected upstream response security token redacted")
	}

	if len(ar.created) != 1 || ar.created[0].RequestID != "req-1" {
		t.Fatalf("unexpected audit event")
	}
	if ar.created[0].Metadata["signature"] != "***" {
		t.Fatalf("expected audit metadata signature redacted")
	}
}

func TestService_RecordRelayCall_FailClosed_OnDownstreamWriteFailure(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)

	ds := &fakeDownstreamRepo{err: errors.New("db down")}
	us := &fakeUpstreamRepo{}
	ar := &fakeAuditRepo{}
	rnd := bytes.NewReader(bytes.Repeat([]byte{0x01}, 24))
	svc := NewService(ds, us, ar, Config{Now: func() time.Time { return base }, Random: rnd})

	err := svc.RecordRelayCall(ctx, RelayCall{
		RequestID: "req-1",
		APIKeyID:  "k1",
		Action:    models.DownstreamActionCVSync2AsyncSubmitTask,
		Method:    "POST",
		Path:      "/",
		Upstream:  UpstreamAttempt{AttemptNumber: 1, UpstreamAction: "x", ResponseStatus: 200},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if internalerrors.GetCode(err) != internalerrors.ErrAuditFailed {
		t.Fatalf("expected error code %s, got %s", internalerrors.ErrAuditFailed, internalerrors.GetCode(err))
	}
	if us.called != 0 {
		t.Fatalf("expected upstream not called")
	}
	if ar.called != 0 {
		t.Fatalf("expected audit not called")
	}
}

func TestService_RecordRelayCall_FailClosed_OnUpstreamWriteFailure(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)

	ds := &fakeDownstreamRepo{}
	us := &fakeUpstreamRepo{err: errors.New("db down")}
	ar := &fakeAuditRepo{}
	rnd := bytes.NewReader(bytes.Repeat([]byte{0x01}, 24))
	svc := NewService(ds, us, ar, Config{Now: func() time.Time { return base }, Random: rnd})

	err := svc.RecordRelayCall(ctx, RelayCall{
		RequestID: "req-1",
		APIKeyID:  "k1",
		Action:    models.DownstreamActionCVSync2AsyncSubmitTask,
		Method:    "POST",
		Path:      "/",
		Upstream:  UpstreamAttempt{AttemptNumber: 1, UpstreamAction: "x", ResponseStatus: 200},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if internalerrors.GetCode(err) != internalerrors.ErrAuditFailed {
		t.Fatalf("expected error code %s, got %s", internalerrors.ErrAuditFailed, internalerrors.GetCode(err))
	}
	if ar.called != 0 {
		t.Fatalf("expected audit not called")
	}
}

func TestService_RecordRelayCall_FailClosed_OnAuditWriteFailure(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)

	ds := &fakeDownstreamRepo{}
	us := &fakeUpstreamRepo{}
	ar := &fakeAuditRepo{err: errors.New("db down")}
	rnd := bytes.NewReader(bytes.Repeat([]byte{0x01}, 24))
	svc := NewService(ds, us, ar, Config{Now: func() time.Time { return base }, Random: rnd})

	err := svc.RecordRelayCall(ctx, RelayCall{
		RequestID: "req-1",
		APIKeyID:  "k1",
		Action:    models.DownstreamActionCVSync2AsyncSubmitTask,
		Method:    "POST",
		Path:      "/",
		Upstream:  UpstreamAttempt{AttemptNumber: 1, UpstreamAction: "x", ResponseStatus: 200},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if internalerrors.GetCode(err) != internalerrors.ErrAuditFailed {
		t.Fatalf("expected error code %s, got %s", internalerrors.ErrAuditFailed, internalerrors.GetCode(err))
	}
}
