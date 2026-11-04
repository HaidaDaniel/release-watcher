package httpx

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		UserAgent:         "release-watcher-test",
		Timeout:           2 * time.Second,
		MaxRedirects:      3,
		MaxRetries:        0,
		RateInterval:      0,
		MaxBodyBytes:      1 << 20,
		AllowPrivateHosts: true,
	}
}

func TestAllowIP(t *testing.T) {
	cfg := testConfig()
	cfg.AllowPrivateHosts = false
	client := New(cfg)
	blocked := []string{
		"127.0.0.1", "::1", "10.0.0.1", "172.16.0.1", "192.168.1.1",
		"169.254.1.1", "100.64.0.1", "198.18.0.1", "240.0.0.1",
		"fc00::1", "fe80::1", "0.0.0.0", "224.0.0.1",
		"::127.0.0.1", "64:ff9b::7f00:1", "2002:7f00:1::", "fec0::1",
	}
	for _, raw := range blocked {
		if client.allowIP(net.ParseIP(raw)) {
			t.Errorf("%s should be blocked", raw)
		}
	}
	allowed := []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"}
	for _, raw := range allowed {
		if !client.allowIP(net.ParseIP(raw)) {
			t.Errorf("%s should be allowed", raw)
		}
	}
	client.cfg.AllowPrivateHosts = true
	if !client.allowIP(net.ParseIP("127.0.0.1")) {
		t.Error("explicit test mode should allow loopback")
	}
}

func TestSSRFBlocksLoopback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.AllowPrivateHosts = false
	client := New(cfg)
	_, err := client.Get(context.Background(), server.URL)
	if err == nil {
		t.Fatal("loopback request was allowed")
	}
	hErr, ok := err.(*Error)
	if !ok || hErr.Code != CodeSSRFBlocked {
		t.Fatalf("err = %#v", err)
	}
}

func TestDialTimeSSRFCheck(t *testing.T) {
	cfg := testConfig()
	cfg.AllowPrivateHosts = false
	client := New(cfg)
	_, err := client.dialContext(context.Background(), "tcp", "127.0.0.1:80")
	if err == nil {
		t.Fatal("dial to loopback allowed")
	}
	hErr, ok := err.(*Error)
	if !ok || hErr.Code != CodeSSRFBlocked {
		t.Fatalf("err = %#v", err)
	}
}

func TestAllowPrivateHosts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	_, err := New(testConfig()).Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("explicit test mode failed: %v", err)
	}
}

func TestRetry5xx(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			http.Error(w, "try again", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	cfg := testConfig()
	cfg.MaxRetries = 1
	resp, err := New(cfg).Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if string(resp.Body) != "ok" || atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("body=%q attempts=%d", resp.Body, attempts)
	}
}

func TestStatusErrorMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/404":
			http.NotFound(w, r)
		case "/403":
			http.Error(w, "no", http.StatusForbidden)
		default:
			http.Error(w, "no", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client := New(testConfig())
	cases := map[string]string{"/404": CodeNotFound, "/403": CodeForbidden, "/400": CodeFetchFailed}
	for path, want := range cases {
		_, err := client.Get(context.Background(), server.URL+path)
		hErr, ok := err.(*Error)
		if !ok || hErr.Code != want {
			t.Errorf("%s: err = %#v, want %s", path, err, want)
		}
	}
}

func TestRedirectLimit(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			http.Redirect(w, r, server.URL+"/b", http.StatusFound)
		case "/b":
			http.Redirect(w, r, server.URL+"/c", http.StatusFound)
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer server.Close()
	cfg := testConfig()
	cfg.MaxRedirects = 1
	_, err := New(cfg).Get(context.Background(), server.URL+"/a")
	if err == nil {
		t.Fatal("redirect limit not enforced")
	}
}

func TestBodyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer server.Close()
	cfg := testConfig()
	cfg.MaxBodyBytes = 10
	_, err := New(cfg).Get(context.Background(), server.URL)
	hErr, ok := err.(*Error)
	if !ok || hErr.Code != CodeFetchFailed {
		t.Fatalf("err = %#v", err)
	}
}

func TestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer server.Close()
	cfg := testConfig()
	cfg.Timeout = 30 * time.Millisecond
	_, err := New(cfg).Get(context.Background(), server.URL)
	hErr, ok := err.(*Error)
	if !ok || hErr.Code != CodeSourceTimeout {
		t.Fatalf("err = %#v", err)
	}
}

func TestRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	cfg := testConfig()
	cfg.RateInterval = 50 * time.Millisecond
	client := New(cfg)
	start := time.Now()
	for i := 0; i < 2; i++ {
		if _, err := client.Get(context.Background(), server.URL); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("requests not paced: %v", elapsed)
	}
}

func TestInvalidURLs(t *testing.T) {
	for _, raw := range []string{"ftp://example.test/x", "http://user:pass@example.test/x", "not a url", "http:///x"} {
		_, err := New(testConfig()).Get(context.Background(), raw)
		hErr, ok := err.(*Error)
		if !ok || hErr.Code != CodeInvalidURL {
			t.Errorf("%q: err = %#v", raw, err)
		}
	}
}

func TestBodyReadTimeoutIsTyped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(150 * time.Millisecond)
	}))
	defer server.Close()
	cfg := testConfig()
	cfg.Timeout = 40 * time.Millisecond
	_, err := New(cfg).Get(context.Background(), server.URL)
	hErr, ok := err.(*Error)
	if !ok || hErr.Code != CodeSourceTimeout {
		t.Fatalf("err = %#v", err)
	}
}

func TestCancellationDuringRetryWaitIsTyped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "10")
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer server.Close()
	cfg := testConfig()
	cfg.MaxRetries = 1
	cfg.Timeout = 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	_, err := New(cfg).Get(ctx, server.URL)
	hErr, ok := err.(*Error)
	if !ok || hErr.Code != CodeSourceTimeout {
		t.Fatalf("err = %#v", err)
	}
}
