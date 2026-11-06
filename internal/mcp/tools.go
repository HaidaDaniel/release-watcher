package mcp

import (
	"context"
	"encoding/json"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HaidaDaniel/release-watcher/internal/app"
)

const (
	readListWatches = "list_watches"
	readGetWatch    = "get_watch"
	readListEvents  = "list_events"
	writeAddWatch   = "add_watch"
	writePause      = "pause_watch"
	writeResume     = "resume_watch"
	writeInterval   = "set_watch_interval"
	writeRemove     = "remove_watch"
)

type emptyInput struct{}

type watchIDInput struct {
	WatchID string `json:"watch_id" jsonschema:"Watch ID returned by list_watches or add_watch"`
}

type addWatchInput struct {
	URL      string  `json:"url" jsonschema:"Public source URL to watch, for example a Narō novel URL or RSS/Atom feed"`
	Interval string  `json:"interval,omitempty" jsonschema:"Optional check interval such as 24h; defaults to the server default"`
	TitleRU  *string `json:"title_ru,omitempty" jsonschema:"Optional Russian title supplied by the user; never invented by the server"`
}

type intervalInput struct {
	WatchID  string `json:"watch_id" jsonschema:"Watch ID returned by list_watches"`
	Interval string `json:"interval" jsonschema:"Check interval such as 24h or 48h (minimum 1m)"`
}

type listEventsInput struct {
	Limit         int    `json:"limit,omitempty" jsonschema:"Maximum number of events to return (1-100, default 20)"`
	After         string `json:"after,omitempty" jsonschema:"Opaque cursor from a previous list_events response"`
	Since         string `json:"since,omitempty" jsonschema:"Inclusive RFC3339 lower bound on detected_at"`
	PublicationID string `json:"publication_id,omitempty" jsonschema:"Optional publication ID filter"`
	WatchID       string `json:"watch_id,omitempty" jsonschema:"Optional watch ID filter"`
}

type listWatchesOutput struct {
	Watches   []app.WatchView `json:"watches"`
	Total     int             `json:"total"`
	Truncated bool            `json:"truncated"`
}

func (s *Server) registerTools() {
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        readListWatches,
		Title:       "List watches",
		Description: "List active Release Watcher watches. Returns original titles and URLs; never invents translations.",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: boolPtr(false)},
	}, s.listWatches)
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        readGetWatch,
		Title:       "Get watch",
		Description: "Get one watch with status, release count, check dates, bounded releases, recent events and recent scans.",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: boolPtr(false)},
	}, s.getWatch)
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        readListEvents,
		Title:       "List events",
		Description: "List stored release events with cursor pagination. Returns at most 100 events per call.",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: boolPtr(false)},
	}, s.listEvents)
	if !s.cfg.WriteEnabled {
		return
	}
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        writeAddWatch,
		Title:       "Add watch",
		Description: "Add a source URL and store a silent baseline. Requires explicit user confirmation before calling.",
		Annotations: &sdkmcp.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(true)},
	}, s.addWatch)
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        writePause,
		Title:       "Pause watch",
		Description: "Pause scanning for one watch. Pending notifications are not canceled. Requires explicit user confirmation.",
		Annotations: &sdkmcp.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(false)},
	}, s.pauseWatch)
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        writeResume,
		Title:       "Resume watch",
		Description: "Resume scanning for one watch. Requires explicit user confirmation.",
		Annotations: &sdkmcp.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(false)},
	}, s.resumeWatch)
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        writeInterval,
		Title:       "Set watch interval",
		Description: "Change one watch's check interval. Requires explicit user confirmation.",
		Annotations: &sdkmcp.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(false)},
	}, s.setWatchInterval)
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        writeRemove,
		Title:       "Stop watching",
		Description: "Stop watching a source while preserving releases and events, canceling pending notifications. This never purges history. Requires explicit user confirmation.",
		Annotations: &sdkmcp.ToolAnnotations{DestructiveHint: boolPtr(true), IdempotentHint: true, OpenWorldHint: boolPtr(false)},
	}, s.removeWatch)
}

func boolPtr(value bool) *bool { return &value }

func (s *Server) listWatches(ctx context.Context, _ *sdkmcp.CallToolRequest, _ emptyInput) (*sdkmcp.CallToolResult, any, error) {
	result, appErr := s.svc.List(ctx)
	if appErr != nil {
		return toolError(appErr), nil, nil
	}
	watches := result.Watches
	total := len(watches)
	truncated := false
	if len(watches) > 200 {
		watches = watches[:200]
		truncated = true
	}
	return toolResult(listWatchesOutput{Watches: watches, Total: total, Truncated: truncated})
}

func (s *Server) getWatch(ctx context.Context, _ *sdkmcp.CallToolRequest, in watchIDInput) (*sdkmcp.CallToolResult, any, error) {
	result, appErr := s.svc.Show(ctx, in.WatchID)
	if appErr != nil {
		return toolError(appErr), nil, nil
	}
	return toolResult(result)
}

func (s *Server) listEvents(ctx context.Context, _ *sdkmcp.CallToolRequest, in listEventsInput) (*sdkmcp.CallToolResult, any, error) {
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return toolErrorCode("invalid_argument", "limit must be between 1 and 100"), nil, nil
	}
	var since *time.Time
	if in.Since != "" {
		parsed, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			return toolErrorCode("invalid_argument", "since must be RFC3339"), nil, nil
		}
		since = &parsed
	}
	result, appErr := s.svc.Events(ctx, app.EventQuery{
		PublicationID: in.PublicationID,
		WatchID:       in.WatchID,
		Since:         since,
		After:         in.After,
		Limit:         limit,
	})
	if appErr != nil {
		return toolError(appErr), nil, nil
	}
	return toolResult(result)
}

func (s *Server) addWatch(ctx context.Context, _ *sdkmcp.CallToolRequest, in addWatchInput) (*sdkmcp.CallToolResult, any, error) {
	if !s.allowMutation() {
		return toolErrorCode("mcp_rate_limited", "too many mutation requests"), nil, nil
	}
	var interval *time.Duration
	if in.Interval != "" {
		parsed, err := time.ParseDuration(in.Interval)
		if err != nil {
			return toolErrorCode("invalid_argument", "interval must be a duration such as 24h"), nil, nil
		}
		interval = &parsed
	}
	result, appErr := s.svc.Add(ctx, app.AddInput{URL: in.URL, TitleRU: in.TitleRU, Interval: interval})
	if appErr != nil {
		return toolError(appErr), nil, nil
	}
	return toolResult(result)
}

func (s *Server) pauseWatch(ctx context.Context, _ *sdkmcp.CallToolRequest, in watchIDInput) (*sdkmcp.CallToolResult, any, error) {
	if !s.allowMutation() {
		return toolErrorCode("mcp_rate_limited", "too many mutation requests"), nil, nil
	}
	result, appErr := s.svc.Pause(ctx, in.WatchID)
	if appErr != nil {
		return toolError(appErr), nil, nil
	}
	return toolResult(result)
}

func (s *Server) resumeWatch(ctx context.Context, _ *sdkmcp.CallToolRequest, in watchIDInput) (*sdkmcp.CallToolResult, any, error) {
	if !s.allowMutation() {
		return toolErrorCode("mcp_rate_limited", "too many mutation requests"), nil, nil
	}
	result, appErr := s.svc.Resume(ctx, in.WatchID)
	if appErr != nil {
		return toolError(appErr), nil, nil
	}
	return toolResult(result)
}

func (s *Server) setWatchInterval(ctx context.Context, _ *sdkmcp.CallToolRequest, in intervalInput) (*sdkmcp.CallToolResult, any, error) {
	if !s.allowMutation() {
		return toolErrorCode("mcp_rate_limited", "too many mutation requests"), nil, nil
	}
	duration, err := time.ParseDuration(in.Interval)
	if err != nil {
		return toolErrorCode("invalid_argument", "interval must be a duration such as 24h"), nil, nil
	}
	result, appErr := s.svc.SetInterval(ctx, in.WatchID, duration)
	if appErr != nil {
		return toolError(appErr), nil, nil
	}
	return toolResult(result)
}

func (s *Server) removeWatch(ctx context.Context, _ *sdkmcp.CallToolRequest, in watchIDInput) (*sdkmcp.CallToolResult, any, error) {
	if !s.allowMutation() {
		return toolErrorCode("mcp_rate_limited", "too many mutation requests"), nil, nil
	}
	result, appErr := s.svc.Remove(ctx, in.WatchID)
	if appErr != nil {
		return toolError(appErr), nil, nil
	}
	return toolResult(result)
}

func (s *Server) allowMutation() bool {
	return s.mutations.allow()
}

func toolResult(value any) (*sdkmcp.CallToolResult, any, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return toolErrorCode("internal_error", "cannot encode result"), nil, nil
	}
	return &sdkmcp.CallToolResult{
		Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: string(payload)}},
		StructuredContent: value,
	}, nil, nil
}

func toolError(appErr *app.Error) *sdkmcp.CallToolResult {
	return toolErrorCode(appErr.Code, appErr.Message)
}

func toolErrorCode(code, message string) *sdkmcp.CallToolResult {
	payload, _ := json.Marshal(map[string]any{"error": map[string]any{"code": code, "message": message}})
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(payload)}},
		StructuredContent: map[string]any{
			"error": map[string]any{"code": code, "message": message},
		},
	}
}
