package upstream

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jimeng-relay/server/internal/config"
	internalerrors "github.com/jimeng-relay/server/internal/errors"
)

const (
	actionSubmit    = "CVSync2AsyncSubmitTask"
	actionGetResult = "CVSync2AsyncGetResult"

	defaultService = "cv"
	defaultVersion = "2022-08-31"

	defaultMaxRetries       = 2
	defaultRetryBackoffBase = 200 * time.Millisecond
	defaultRetryBackoffMax  = 2 * time.Second

	xDateLayout = "20060102T150405Z"

	defaultMaxConcurrent = 10
	defaultMaxQueue      = 100
)

type Options struct {
	HTTPClient    *http.Client
	Now           func() time.Time
	Sleep         func(context.Context, time.Duration) error
	MaxRetries    int
	Service       string
	Version       string
	MaxConcurrent int
	MaxQueue      int
}

type queueWaiter struct {
	ready chan struct{}
}

type Client struct {
	ak       string
	sk       string
	region   string
	service  string
	version  string
	baseURL  *url.URL
	now      func() time.Time
	sleep    func(context.Context, time.Duration) error
	maxRetry int
	hc       *http.Client

	mu      sync.Mutex
	sem     chan struct{}
	waiters []*queueWaiter
	maxQueue int
}

type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func NewClient(cfg config.Config, opts Options) (*Client, error) {
	missing := make([]string, 0, 4)
	if strings.TrimSpace(cfg.Credentials.AccessKey) == "" {
		missing = append(missing, config.EnvAccessKey)
	}
	if strings.TrimSpace(cfg.Credentials.SecretKey) == "" {
		missing = append(missing, config.EnvSecretKey)
	}
	if strings.TrimSpace(cfg.Host) == "" {
		missing = append(missing, config.EnvHost)
	}
	if strings.TrimSpace(cfg.Region) == "" {
		missing = append(missing, config.EnvRegion)
	}
	if len(missing) > 0 {
		return nil, internalerrors.New(
			internalerrors.ErrValidationFailed,
			"missing upstream config: "+strings.Join(missing, ", "),
			nil,
		)
	}

	service := strings.TrimSpace(opts.Service)
	if service == "" {
		service = defaultService
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = defaultVersion
	}

	baseURL, err := parseBaseURL(cfg.Host)
	if err != nil {
		return nil, internalerrors.New(internalerrors.ErrValidationFailed, "invalid upstream host", err)
	}

	nowFn := opts.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}

	sleepFn := opts.Sleep
	if sleepFn == nil {
		sleepFn = sleepContext
	}

	maxRetry := opts.MaxRetries
	if maxRetry < 0 {
		maxRetry = 0
	}
	if opts.MaxRetries == 0 {
		maxRetry = defaultMaxRetries
	}

	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{}
		if cfg.Timeout > 0 {
			hc.Timeout = cfg.Timeout
		}
	}

	maxConcurrent := opts.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = cfg.UpstreamMaxConcurrent
	}
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrent
	}

	maxQueue := opts.MaxQueue
	if maxQueue <= 0 {
		maxQueue = cfg.UpstreamMaxQueue
	}
	if maxQueue <= 0 {
		maxQueue = defaultMaxQueue
	}

	return &Client{
		ak:        strings.TrimSpace(cfg.Credentials.AccessKey),
		sk:        strings.TrimSpace(cfg.Credentials.SecretKey),
		region:    strings.TrimSpace(cfg.Region),
		service:   service,
		version:   version,
		baseURL:   baseURL,
		now:       nowFn,
		sleep:     sleepFn,
		maxRetry:  maxRetry,
		hc:        hc,
		sem:       make(chan struct{}, maxConcurrent),
		waiters:   make([]*queueWaiter, 0, maxQueue),
		maxQueue:  maxQueue,
	}, nil
}

func (c *Client) Submit(ctx context.Context, body []byte, headers http.Header) (*Response, error) {
	return c.do(ctx, actionSubmit, body, headers)
}

func (c *Client) GetResult(ctx context.Context, body []byte, headers http.Header) (*Response, error) {
	return c.do(ctx, actionGetResult, body, headers)
}

func (c *Client) do(ctx context.Context, action string, body []byte, headers http.Header) (*Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, internalerrors.New(internalerrors.ErrUpstreamFailed, "context done before upstream call", err)
	}

	if c == nil || c.baseURL == nil || c.hc == nil || c.now == nil {
		return nil, internalerrors.New(internalerrors.ErrInternalError, "upstream client is not initialized", nil)
	}
	if c.sleep == nil {
		return nil, internalerrors.New(internalerrors.ErrInternalError, "upstream client sleeper is not initialized", nil)
	}

	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()

	maxRetry := c.maxRetry
	if maxRetry < 0 {
		maxRetry = 0
	}

	for attempt := 0; attempt <= maxRetry; attempt++ {
		out, err := c.doOnce(ctx, action, body, headers)
		if !isRetriableStatus(out) || attempt == maxRetry {
			return out, err
		}

		delay := retryDelay(out.Header.Get("Retry-After"), c.now())
		if delay <= 0 {
			delay = boundedBackoff(attempt)
		}

		if sleepErr := c.sleep(ctx, delay); sleepErr != nil {
			return out, internalerrors.New(internalerrors.ErrUpstreamFailed, "context done during upstream retry backoff", sleepErr)
		}
	}

	return nil, internalerrors.New(internalerrors.ErrInternalError, "unreachable upstream retry state", nil)
}

func (c *Client) acquire(ctx context.Context) error {
	if c.sem == nil {
		return nil
	}

	c.mu.Lock()

	select {
	case c.sem <- struct{}{}:
		c.mu.Unlock()
		return nil
	default:
	}

	if len(c.waiters) >= c.maxQueue {
		c.mu.Unlock()
		return internalerrors.New(internalerrors.ErrRateLimited, "upstream queue is full", nil)
	}

	w := &queueWaiter{ready: make(chan struct{})}
	c.waiters = append(c.waiters, w)
	c.mu.Unlock()

	select {
	case <-w.ready:
		return nil
	case <-ctx.Done():
		c.removeWaiter(w)
		return internalerrors.New(internalerrors.ErrUpstreamFailed, "context cancelled while waiting in queue", ctx.Err())
	}
}

func (c *Client) removeWaiter(w *queueWaiter) {
	c.mu.Lock()
	for i, waiter := range c.waiters {
		if waiter == w {
			c.waiters = append(c.waiters[:i], c.waiters[i+1:]...)
			break
		}
	}
	c.mu.Unlock()
}

func (c *Client) release() {
	if c.sem == nil {
		return
	}

	<-c.sem

	c.mu.Lock()
	if len(c.waiters) > 0 {
		w := c.waiters[0]
		c.waiters = c.waiters[1:]
		close(w.ready)
	}
	c.mu.Unlock()
}

func (c *Client) doOnce(ctx context.Context, action string, body []byte, headers http.Header) (*Response, error) {

	endpoint := *c.baseURL
	endpoint.Path = "/"
	q := endpoint.Query()
	q.Set("Action", action)
	q.Set("Version", c.version)
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, internalerrors.New(internalerrors.ErrUpstreamFailed, "build upstream request", err)
	}

	if endpoint.Host != "" {
		req.Host = endpoint.Host
	}

	applyHeaders(req.Header, headers)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Del("Authorization")
	req.Header.Del("X-Date")
	req.Header.Del("X-Content-Sha256")

	now := c.now().UTC()
	xDate := now.Format(xDateLayout)
	dateScope := now.Format("20060102")
	payloadHash := sha256Hex(body)

	req.Header.Set("X-Date", xDate)
	req.Header.Set("X-Content-Sha256", payloadHash)

	signedHeaders := []string{"content-type", "host", "x-content-sha256", "x-date"}
	sort.Strings(signedHeaders)

	canonicalRequest, err := buildCanonicalRequest(req, signedHeaders, payloadHash)
	if err != nil {
		return nil, internalerrors.New(internalerrors.ErrUpstreamFailed, "build canonical request", err)
	}
	scope := strings.Join([]string{dateScope, c.region, c.service, "request"}, "/")
	stringToSign := strings.Join([]string{
		"HMAC-SHA256",
		xDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signingKey := deriveSigningKey(c.sk, dateScope, c.region, c.service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	authorization := "HMAC-SHA256 Credential=" + c.ak + "/" + scope + ", SignedHeaders=" + strings.Join(signedHeaders, ";") + ", Signature=" + signature
	req.Header.Set("Authorization", authorization)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, internalerrors.New(internalerrors.ErrUpstreamFailed, "upstream request failed", err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, internalerrors.New(internalerrors.ErrUpstreamFailed, "read upstream response", readErr)
	}

	out := &Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       respBody,
	}

	if err := ctx.Err(); err != nil {
		return out, internalerrors.New(internalerrors.ErrUpstreamFailed, "context done after upstream call", err)
	}

	if resp.StatusCode >= 400 {
		return out, internalerrors.New(internalerrors.ErrUpstreamFailed, fmt.Sprintf("upstream %s returned %d", action, resp.StatusCode), nil)
	}

	return out, nil
}

func isRetriableStatus(resp *Response) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode >= http.StatusInternalServerError && resp.StatusCode <= http.StatusNetworkAuthenticationRequired
}

func retryDelay(retryAfter string, now time.Time) time.Duration {
	retryAfter = strings.TrimSpace(retryAfter)
	if retryAfter == "" {
		return 0
	}

	if seconds, err := time.ParseDuration(retryAfter + "s"); err == nil {
		if seconds < 0 {
			return 0
		}
		return seconds
	}

	if at, err := http.ParseTime(retryAfter); err == nil {
		d := at.Sub(now.UTC())
		if d < 0 {
			return 0
		}
		return d
	}

	return 0
}

func boundedBackoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 5 {
		attempt = 5
	}
	d := defaultRetryBackoffBase * time.Duration(1<<attempt)
	if d > defaultRetryBackoffMax {
		return defaultRetryBackoffMax
	}
	return d
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func parseBaseURL(host string) (*url.URL, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("empty")
	}
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		parsed, err := url.Parse(host)
		if err != nil {
			return nil, err
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid url")
		}
		return parsed, nil
	}
	parsed, err := url.Parse("https://" + host)
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("invalid host")
	}
	return parsed, nil
}

func applyHeaders(dst http.Header, src http.Header) {
	if len(src) == 0 {
		return
	}
	for k, vals := range src {
		if strings.EqualFold(k, "Authorization") {
			continue
		}
		if strings.EqualFold(k, "Host") {
			continue
		}
		if strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

func buildCanonicalRequest(r *http.Request, signedHeaders []string, payloadHash string) (string, error) {
	headers := append([]string(nil), signedHeaders...)
	sort.Strings(headers)
	canonHeaders := strings.Builder{}
	for _, h := range headers {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			return "", fmt.Errorf("empty signed header")
		}
		v := canonicalHeaderValue(r, h)
		if v == "" {
			return "", fmt.Errorf("missing signed header: %s", h)
		}
		canonHeaders.WriteString(h)
		canonHeaders.WriteByte(':')
		canonHeaders.WriteString(v)
		canonHeaders.WriteByte('\n')
	}

	canon := strings.Join([]string{
		r.Method,
		canonicalURI(r.URL.Path),
		canonicalQueryString(r.URL.Query()),
		canonHeaders.String(),
		strings.Join(headers, ";"),
		payloadHash,
	}, "\n")
	return canon, nil
}

func canonicalHeaderValue(r *http.Request, name string) string {
	if name == "host" {
		return strings.TrimSpace(strings.ToLower(r.Host))
	}
	vals := r.Header.Values(name)
	if len(vals) == 0 {
		return ""
	}
	for i := range vals {
		vals[i] = strings.Join(strings.Fields(vals[i]), " ")
	}
	return strings.TrimSpace(strings.Join(vals, ","))
}

func canonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	parts := strings.Split(path, "/")
	for i := range parts {
		parts[i] = awsEscape(parts[i])
	}
	uri := strings.Join(parts, "/")
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	return uri
}

func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0)
	for _, k := range keys {
		vals := append([]string(nil), values[k]...)
		sort.Strings(vals)
		ek := awsEscape(k)
		for _, v := range vals {
			pairs = append(pairs, ek+"="+awsEscape(v))
		}
	}
	return strings.Join(pairs, "&")
}

func awsEscape(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	e = strings.ReplaceAll(e, "*", "%2A")
	e = strings.ReplaceAll(e, "%7E", "~")
	return e
}

func deriveSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte(secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "request")
}

func hmacSHA256(key []byte, message string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(message))
	return h.Sum(nil)
}

func sha256Hex(v []byte) string {
	s := sha256.Sum256(v)
	return hex.EncodeToString(s[:])
}
