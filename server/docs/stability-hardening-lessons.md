# Stability Hardening Lessons (2026-02)

## 1) Incident Timeline and Fixes

### A. Same-key concurrency caused misleading client failure
- **Symptom**: under two clients using the same key, first client failed with:
  `BUSINESS_FAILED: code=0 status=0 message= request_id=`
- **Root cause**: client get-result parser only read `code/status/message/request_id` from success-shaped payload. When relay returned `{"error": {...}}`, those fields were empty and defaulted to zero/empty.
- **Fix**:
  - detect relay `error` object first and map error code to client error categories;
  - add defensive fallback: all-empty business fields become `DECODE_FAILED` instead of fake `BUSINESS_FAILED`.
- **Files**: `client/internal/jimeng/getresult.go`

### B. Queue handoff race in upstream concurrency gate
- **Symptom**: potential waiter handoff steal and semaphore inconsistency in cancel/handoff race window.
- **Root cause**: queued waiter wake-up and token ownership protocol allowed edge-case reorder/steal.
- **Fix**:
  - direct handoff ownership semantics;
  - cancellation compensation path tightened;
  - deterministic regression tests for A/B/C race and FIFO order.
- **Files**: `server/internal/relay/upstream/client.go`, `server/internal/relay/upstream/client_test.go`, `server/internal/relay/upstream/client_internal_test.go`

### C. Docs/workflow drift and evidence mismatch
- **Symptom**: release checklist default drift and missing/misnamed evidence artifacts.
- **Fix**:
  - align default `UPSTREAM_MAX_CONCURRENT=1` in release docs;
  - add required Wave3 evidence files and CI/replay consistency checks.
- **Files**: `server/docs/release-checklist.md`, `.sisyphus/evidence/*.txt`, workflows/docs updates.

## 2) Hardening Changes Applied in This Pass

### Runtime crash containment
- Added top-level panic recovery middleware to prevent single request panic from crashing process.
- Middleware logs panic+stack and returns normalized 500 JSON when header not yet written.
- **Files**: `server/internal/middleware/observability/recover.go`

### Server timeout hardening
- Replaced implicit `http.ListenAndServe` with explicit `http.Server` and timeout guards:
  - `ReadHeaderTimeout`
  - `ReadTimeout`
  - `WriteTimeout`
  - `IdleTimeout`
  - `MaxHeaderBytes`
- **Files**: `server/cmd/server/main.go`

### Upstream resource bound hardening
- Added upper bound for upstream response body reads to avoid unbounded memory growth.
- Added cap for parsed `Retry-After` delay to avoid excessive sleep from hostile/malformed headers.
- Removed dead variable in queue path (`queuePos`).
- **Files**: `server/internal/relay/upstream/client.go`

### Code hygiene cleanup
- Removed tracked backup source file from repository.
- **Files**: removed `server/internal/middleware/sigv4/middleware.go.bak`

## 3) Security, Stability, and Performance Review Summary

### Verified strengths
- Request body size limits are enforced on auth path via `http.MaxBytesReader`.
- Signature validation uses constant-time comparison.
- Error to HTTP status mapping is explicit in relay util layer.

### Risks addressed now
- Panic crash blast radius.
- Slowloris-like header/body hangs from missing server-level timeouts.
- Upstream response OOM risk from unbounded `ReadAll`.
- Retry delay abuse risk from unbounded `Retry-After`.

### Remaining medium-term recommendations
- Add route-specific timeout policy if future streaming/long-poll endpoints are introduced.
- Add optional host allowlist for client-side download to further reduce SSRF-like misuse surface.
- Consider background cleanup strategy for long-lived in-memory key-state map if key cardinality grows very large.

## 4) Validation Standard

Every hardening change should pass:
- `go test ./... -count=1`
- `go test -race ./... -count=1`
- `go build ./...`

And for end-to-end behavior:
- `go run ./scripts/local_e2e_concurrency.go`
