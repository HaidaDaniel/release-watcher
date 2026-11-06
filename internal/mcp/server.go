// Package mcp exposes Release Watcher use cases over the Model Context
// Protocol Streamable HTTP transport.
package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HaidaDaniel/release-watcher/internal/app"
	"github.com/HaidaDaniel/release-watcher/internal/buildinfo"
)

// Config controls the MCP HTTP server. Token is mandatory and must contain at
// least 32 bytes of secret material.
type Config struct {
	Token          string
	WriteEnabled   bool
	AllowedHosts   []string
	AllowedOrigins []string
	MaxBodyBytes   int64
	MaxConcurrent  int
	RequestTimeout time.Duration
	MutationRate   float64
	MutationBurst  int
}

// Server owns one MCP server and its HTTP middleware.
type Server struct {
	svc       *app.Service
	cfg       Config
	logger    *slog.Logger
	sdk       *sdkmcp.Server
	handler   http.Handler
	semaphore chan struct{}
	mutations *mutationLimiter
}

// New builds the MCP server and registers tools over app.Service.
func New(svc *app.Service, cfg Config, logger *slog.Logger) (*Server, error) {
	if svc == nil {
		return nil, errors.New("mcp: nil service")
	}
	if len(cfg.Token) < 32 {
		return nil, errors.New("mcp: token must contain at least 32 bytes")
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << 20
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 8
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 45 * time.Second
	}
	if cfg.MutationRate <= 0 {
		cfg.MutationRate = 10.0 / 60.0
	}
	if cfg.MutationBurst <= 0 {
		cfg.MutationBurst = 10
	}
	if logger == nil {
		logger = slog.Default()
	}
	info := buildinfo.Get()
	sdk := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "release-watcher", Version: info.Version}, nil)
	s := &Server{
		svc:       svc,
		cfg:       cfg,
		logger:    logger,
		sdk:       sdk,
		semaphore: make(chan struct{}, cfg.MaxConcurrent),
		mutations: newMutationLimiter(cfg.MutationRate, cfg.MutationBurst),
	}
	s.registerTools()
	handler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return sdk }, &sdkmcp.StreamableHTTPOptions{
		Stateless:           true,
		MaxRequestBodyBytes: cfg.MaxBodyBytes,
		Logger:              logger,
	})
	s.handler = s.middleware(handler)
	return s, nil
}

// Handler returns the complete HTTP handler, including authentication and
// hardening middleware.
func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) middleware(next http.Handler) http.Handler {
	handler := next
	handler = s.timeoutMiddleware(handler)
	handler = s.concurrencyMiddleware(handler)
	handler = s.originMiddleware(handler)
	handler = s.hostMiddleware(handler)
	handler = s.authMiddleware(handler)
	return handler
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	expected := sha256.Sum256([]byte(s.cfg.Token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			unauthorized(w)
			return
		}
		provided := sha256.Sum256([]byte(parts[1]))
		if subtle.ConstantTimeCompare(provided[:], expected[:]) != 1 {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
}

func (s *Server) hostMiddleware(next http.Handler) http.Handler {
	allowed := map[string]bool{
		"127.0.0.1": true, "localhost": true, "::1": true, "[::1]": true,
	}
	for _, host := range s.cfg.AllowedHosts {
		allowed[strings.ToLower(strings.TrimSpace(host))] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if parsed, _, err := net.SplitHostPort(r.Host); err == nil {
			host = parsed
		}
		host = strings.Trim(strings.ToLower(host), "[]")
		if !allowed[host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originMiddleware(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range s.cfg.AllowedOrigins {
		allowed[strings.TrimRight(strings.ToLower(strings.TrimSpace(origin)), "/")] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		if origin == "null" || !allowed[strings.TrimRight(strings.ToLower(origin), "/")] {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) concurrencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case s.semaphore <- struct{}{}:
			defer func() { <-s.semaphore }()
			next.ServeHTTP(w, r)
		default:
			http.Error(w, "too many requests", http.StatusTooManyRequests)
		}
	})
}

func (s *Server) timeoutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Serve listens on addr and serves until ctx is canceled.
func (s *Server) Serve(ctx context.Context, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("mcp: listen %s: %w", addr, err)
	}
	httpServer := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Serve(listener)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("mcp: shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

type mutationLimiter struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func newMutationLimiter(rate float64, burst int) *mutationLimiter {
	return &mutationLimiter{rate: rate, burst: float64(burst), tokens: float64(burst), last: time.Now()}
}

func (l *mutationLimiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.tokens += now.Sub(l.last).Seconds() * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}
	l.last = now
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}
