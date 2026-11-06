package mcp_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HaidaDaniel/release-watcher/internal/app"
	"github.com/HaidaDaniel/release-watcher/internal/lock"
	rwmcp "github.com/HaidaDaniel/release-watcher/internal/mcp"
)

func postMCP(t *testing.T, endpoint, authorization, contentType, protocolVersion, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	} else {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", protocolVersion)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(payload)
}

func TestMCPAuthorizationVariants(t *testing.T) {
	env := newEnv(t, true)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	for _, header := range []string{"", "Bearer", "Bearer ", "Basic abc", "Bearer wrong-token"} {
		status, response := postMCP(t, env.server.URL+"/mcp", header, "application/json", "", body)
		if status != http.StatusUnauthorized {
			t.Fatalf("header %q status = %d", header, status)
		}
		if strings.Contains(response, testToken) {
			t.Fatalf("response leaked token for header %q", header)
		}
	}
}

func TestMCPWrongContentTypeAndProtocolVersion(t *testing.T) {
	env := newEnv(t, true)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	status, _ := postMCP(t, env.server.URL+"/mcp", "Bearer "+testToken, "text/plain", "", body)
	if status == http.StatusOK {
		t.Fatal("text/plain initialize was accepted")
	}
	status, _ = postMCP(t, env.server.URL+"/mcp", "Bearer "+testToken, "application/json", "1999-01-01", body)
	if status == http.StatusOK {
		t.Fatal("unsupported protocol version was accepted")
	}
}

func TestMCPUnknownAndForbiddenTools(t *testing.T) {
	env := newEnv(t, true)
	session := connect(t, env.server.URL+"/mcp", testToken)
	for _, name := range []string{"purge", "backup", "restore", "shell", "scan_all"} {
		if _, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: name}); err == nil {
			t.Fatalf("unknown/forbidden tool %q was callable", name)
		}
	}
}

func TestMCPConcurrentCallsAndDuplicateAdd(t *testing.T) {
	env := newEnv(t, true)
	url := env.feed.server.URL + "/feed.xml"
	var wg sync.WaitGroup
	results := make(chan app.AddResult, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session := connect(t, env.server.URL+"/mcp", testToken)
			result, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "add_watch", Arguments: map[string]any{"url": url, "interval": "24h"}})
			if err != nil {
				errs <- err
				return
			}
			if result.IsError {
				errs <- io.ErrUnexpectedEOF
				return
			}
			var added app.AddResult
			for _, content := range result.Content {
				if text, ok := content.(*sdkmcp.TextContent); ok {
					_ = json.Unmarshal([]byte(text.Text), &added)
				}
			}
			results <- added
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent add: %v", err)
	}
	created := 0
	var watchID string
	for result := range results {
		if result.Created {
			created++
		}
		if watchID == "" {
			watchID = result.WatchID
		}
		if result.WatchID != watchID {
			t.Fatalf("concurrent add produced different watches: %s / %s", watchID, result.WatchID)
		}
	}
	if created != 1 {
		t.Fatalf("concurrent add created %d watches", created)
	}

	var mu sync.Mutex
	var firstErr error
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session := connect(t, env.server.URL+"/mcp", testToken)
			if _, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "list_watches"}); err != nil {
				mu.Lock()
				firstErr = err
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		t.Fatal(firstErr)
	}
}

func TestMCPRestartPersistence(t *testing.T) {
	env := newEnv(t, true)
	var added app.AddResult
	decode(t, call(t, connect(t, env.server.URL+"/mcp", testToken), "add_watch", map[string]any{"url": env.feed.server.URL + "/feed.xml"}), &added)
	env.server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	server, err := rwmcp.New(env.svc, rwmcp.Config{Token: testToken, WriteEnabled: true}, logger)
	if err != nil {
		t.Fatal(err)
	}
	restarted := httptest.NewServer(server.Handler())
	defer restarted.Close()
	session := connect(t, restarted.URL+"/mcp", testToken)
	var list struct {
		Watches []app.WatchView `json:"watches"`
	}
	decode(t, call(t, session, "list_watches", map[string]any{}), &list)
	if len(list.Watches) != 1 || list.Watches[0].WatchID != added.WatchID {
		t.Fatalf("watches after MCP restart = %+v", list)
	}
}

func TestMCPSSRFBlocked(t *testing.T) {
	env := newEnvWith(t, true, nil, false)
	session := connect(t, env.server.URL+"/mcp", testToken)
	result, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "add_watch", Arguments: map[string]any{"url": "http://127.0.0.1:1/feed.xml"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("SSRF URL was accepted")
	}
	found := false
	for _, content := range result.Content {
		if text, ok := content.(*sdkmcp.TextContent); ok && strings.Contains(text.Text, "ssrf_blocked") {
			found = true
		}
	}
	if !found {
		t.Fatal("SSRF error did not contain ssrf_blocked")
	}
}

func TestMCPLockPropagation(t *testing.T) {
	env := newEnv(t, true)
	session := connect(t, env.server.URL+"/mcp", testToken)
	var added app.AddResult
	decode(t, call(t, session, "add_watch", map[string]any{"url": env.feed.server.URL + "/feed.xml"}), &added)
	release, err := lock.Acquire(context.Background(), env.cfg.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	result, callErr := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "remove_watch", Arguments: map[string]any{"watch_id": added.WatchID}})
	if callErr != nil {
		t.Fatal(callErr)
	}
	if !result.IsError {
		t.Fatal("remove succeeded while the scan lock was held")
	}
	release()
	var removed app.RemoveResult
	decode(t, call(t, session, "remove_watch", map[string]any{"watch_id": added.WatchID}), &removed)
	if !removed.Removed {
		t.Fatalf("remove after lock release = %+v", removed)
	}
}
