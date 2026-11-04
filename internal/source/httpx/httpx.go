// Package httpx is the single HTTP boundary for source adapters. It enforces
// timeouts, redirect limits, per-host pacing, bounded retries and SSRF-safe
// dialing.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	CodeInvalidURL    = "invalid_url"
	CodeSSRFBlocked   = "ssrf_blocked"
	CodeSourceTimeout = "source_timeout"
	CodeRateLimited   = "rate_limited"
	CodeNotFound      = "source_not_found"
	CodeForbidden     = "source_forbidden"
	CodeFetchFailed   = "fetch_failed"
)

// Error is a typed, diagnosable source transport error.
type Error struct {
	Code       string
	Message    string
	Temporary  bool
	StatusCode int
	Cause      error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// Config controls the shared client.
type Config struct {
	UserAgent         string
	Timeout           time.Duration
	MaxRedirects      int
	MaxRetries        int
	RateInterval      time.Duration
	MaxBodyBytes      int64
	AllowPrivateHosts bool
}

// Response is a bounded successful response.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	FinalURL   string
}

// Client is safe for concurrent use.
type Client struct {
	cfg      Config
	http     *http.Client
	limiter  *hostLimiter
	resolver *net.Resolver
}

func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 4 << 20
	}
	if cfg.MaxRedirects < 0 {
		cfg.MaxRedirects = 0
	}
	c := &Client{
		cfg:      cfg,
		limiter:  newHostLimiter(cfg.RateInterval),
		resolver: net.DefaultResolver,
	}
	transport := &http.Transport{
		DialContext:           c.dialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: cfg.Timeout,
	}
	c.http = &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= cfg.MaxRedirects {
				return &Error{Code: CodeFetchFailed, Message: fmt.Sprintf("too many redirects (limit %d)", cfg.MaxRedirects)}
			}
			if err := validateURL(req.URL); err != nil {
				return err
			}
			return nil
		},
	}
	return c
}

// ValidateURL checks scheme, host and userinfo without making a request.
func ValidateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, &Error{Code: CodeInvalidURL, Message: "malformed URL", Cause: err}
	}
	if err := validateURL(u); err != nil {
		return nil, err
	}
	return u, nil
}

func validateURL(u *url.URL) error {
	if u == nil {
		return &Error{Code: CodeInvalidURL, Message: "empty URL"}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return &Error{Code: CodeInvalidURL, Message: "only http and https URLs are supported"}
	}
	if u.Host == "" || u.Hostname() == "" {
		return &Error{Code: CodeInvalidURL, Message: "URL host is required"}
	}
	if u.User != nil {
		return &Error{Code: CodeInvalidURL, Message: "URL userinfo is not allowed"}
	}
	return nil
}

// Get performs a GET with pacing, retry and a bounded body.
func (c *Client) Get(ctx context.Context, raw string) (*Response, error) {
	u, err := ValidateURL(raw)
	if err != nil {
		return nil, err
	}
	var lastErr error
	attempts := c.cfg.MaxRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if err := c.limiter.wait(ctx, u.Hostname()); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, &Error{Code: CodeInvalidURL, Message: "cannot build request", Cause: err}
		}
		req.Header.Set("User-Agent", c.cfg.UserAgent)
		req.Header.Set("Accept", "application/json, application/rss+xml, application/atom+xml, application/xml, text/xml, text/html;q=0.8, */*;q=0.5")

		resp, err := c.http.Do(req)
		if err != nil {
			mapped := classifyError(err)
			if mapped.Temporary && attempt < attempts-1 {
				lastErr = mapped
				if !sleepContext(ctx, backoff(attempt)) {
					return nil, ctx.Err()
				}
				continue
			}
			return nil, mapped
		}

		body, readErr := readLimited(resp.Body, c.cfg.MaxBodyBytes)
		_ = resp.Body.Close()
		if readErr != nil {
			mapped := &Error{Code: CodeFetchFailed, Message: "cannot read response body", Temporary: true, Cause: readErr}
			if attempt < attempts-1 {
				lastErr = mapped
				if !sleepContext(ctx, backoff(attempt)) {
					return nil, ctx.Err()
				}
				continue
			}
			return nil, mapped
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return &Response{
				StatusCode: resp.StatusCode,
				Header:     resp.Header,
				Body:       body,
				FinalURL:   resp.Request.URL.String(),
			}, nil
		}

		mapped := statusError(resp.StatusCode)
		if mapped.Temporary && attempt < attempts-1 {
			lastErr = mapped
			wait := retryAfter(resp.Header.Get("Retry-After"))
			if wait <= 0 {
				wait = backoff(attempt)
			}
			if !sleepContext(ctx, wait) {
				return nil, ctx.Err()
			}
			continue
		}
		return nil, mapped
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &Error{Code: CodeFetchFailed, Message: "request failed"}
}

func (c *Client) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, &Error{Code: CodeFetchFailed, Message: "invalid network address", Cause: err}
	}
	ips, err := c.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, &Error{Code: CodeFetchFailed, Message: "cannot resolve host", Temporary: true, Cause: err}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, ip := range ips {
		if !c.allowIP(ip.IP) {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, &Error{Code: CodeFetchFailed, Message: "cannot connect to allowed address", Temporary: true, Cause: lastErr}
	}
	return nil, &Error{Code: CodeSSRFBlocked, Message: fmt.Sprintf("host %q resolves only to disallowed addresses", host)}
}

func (c *Client) allowIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if c.cfg.AllowPrivateHosts {
		return !ip.IsUnspecified()
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return false // 100.64.0.0/10 CGNAT
		}
		if v4[0] == 192 && v4[1] == 0 && v4[2] == 0 {
			return false // 192.0.0.0/24
		}
		if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) {
			return false // 198.18.0.0/15 benchmarking
		}
		if v4[0] >= 240 {
			return false // reserved
		}
		return true
	}
	if len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc {
		return false // fc00::/7 unique local
	}
	return true
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("response exceeds %d bytes", max)
	}
	return data, nil
}

func classifyError(err error) *Error {
	var hErr *Error
	if errors.As(err, &hErr) {
		return hErr
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &Error{Code: CodeSourceTimeout, Message: "request timed out", Temporary: true, Cause: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &Error{Code: CodeSourceTimeout, Message: "request timed out", Temporary: true, Cause: err}
	}
	return &Error{Code: CodeFetchFailed, Message: "request failed", Temporary: true, Cause: err}
}

func statusError(status int) *Error {
	switch status {
	case http.StatusNotFound:
		return &Error{Code: CodeNotFound, Message: "source returned 404", StatusCode: status}
	case http.StatusForbidden, http.StatusUnauthorized:
		return &Error{Code: CodeForbidden, Message: "source denied access", StatusCode: status}
	case http.StatusTooManyRequests:
		return &Error{Code: CodeRateLimited, Message: "source rate limited the request", Temporary: true, StatusCode: status}
	default:
		if status >= 500 {
			return &Error{Code: CodeFetchFailed, Message: fmt.Sprintf("source returned %d", status), Temporary: true, StatusCode: status}
		}
		return &Error{Code: CodeFetchFailed, Message: fmt.Sprintf("source returned %d", status), StatusCode: status}
	}
}

func backoff(attempt int) time.Duration {
	d := 500 * time.Millisecond << attempt
	if d > 8*time.Second {
		d = 8 * time.Second
	}
	return d
}

func retryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0
		}
		d := time.Duration(seconds) * time.Second
		if d > 30*time.Second {
			d = 30 * time.Second
		}
		return d
	}
	if t, err := http.ParseTime(value); err == nil {
		d := time.Until(t)
		if d > 30*time.Second {
			d = 30 * time.Second
		}
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type hostLimiter struct {
	interval time.Duration
	mu       sync.Mutex
	next     map[string]time.Time
}

func newHostLimiter(interval time.Duration) *hostLimiter {
	return &hostLimiter{interval: interval, next: map[string]time.Time{}}
}

func (l *hostLimiter) wait(ctx context.Context, host string) error {
	if l.interval <= 0 {
		return nil
	}
	for {
		l.mu.Lock()
		now := time.Now()
		next := l.next[host]
		if !now.Before(next) {
			l.next[host] = now.Add(l.interval)
			l.mu.Unlock()
			return nil
		}
		delay := next.Sub(now)
		l.mu.Unlock()
		if !sleepContext(ctx, delay) {
			return ctx.Err()
		}
	}
}
