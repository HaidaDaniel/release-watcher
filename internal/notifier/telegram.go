package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	CodeTelegramUnreachable = "telegram_unreachable"
	CodeTelegramRateLimited = "telegram_rate_limited"
	CodeTelegramForbidden   = "telegram_forbidden"
	CodeTelegramBadRequest  = "telegram_bad_request"
	CodeTelegramServer      = "telegram_server_error"
	CodeTelegramInvalid     = "telegram_invalid_response"
)

// Telegram sends messages through the Telegram Bot API.
type Telegram struct {
	Client  *http.Client
	APIBase string
	Token   string
	ChatID  string
}

func NewTelegram(apiBase, token, chatID string, timeout time.Duration) *Telegram {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Telegram{
		Client:  &http.Client{Timeout: timeout},
		APIBase: strings.TrimRight(apiBase, "/"),
		Token:   token,
		ChatID:  chatID,
	}
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) Send(ctx context.Context, text string) error {
	if t.Token == "" || t.ChatID == "" {
		return &Error{Code: CodeTelegramBadRequest, Message: "Telegram token or chat id is missing"}
	}
	form := url.Values{}
	form.Set("chat_id", t.ChatID)
	form.Set("text", text)
	form.Set("disable_web_page_preview", "true")
	endpoint := t.APIBase + "/bot" + t.Token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return &Error{Code: CodeTelegramBadRequest, Message: "cannot build Telegram request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.Client.Do(req)
	if err != nil {
		return &Error{Code: CodeTelegramUnreachable, Message: "Telegram request failed", Retryable: true, Cause: err}
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if readErr != nil {
		return &Error{Code: CodeTelegramUnreachable, Message: "cannot read Telegram response", Retryable: true, Cause: readErr}
	}
	var payload struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	parseErr := json.Unmarshal(body, &payload)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && parseErr == nil && payload.OK {
		return nil
	}
	if parseErr != nil {
		return &Error{Code: CodeTelegramInvalid, Message: fmt.Sprintf("Telegram returned HTTP %d with an invalid body", resp.StatusCode), StatusCode: resp.StatusCode}
	}
	message := "Telegram rejected the notification"
	if payload.Description != "" {
		message = fmt.Sprintf("Telegram error %d: %s", payload.ErrorCode, payload.Description)
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return &Error{
			Code: CodeTelegramRateLimited, Message: message, Retryable: true,
			RetryAfter: time.Duration(payload.Parameters.RetryAfter) * time.Second,
			StatusCode: resp.StatusCode,
		}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return &Error{Code: CodeTelegramForbidden, Message: message, StatusCode: resp.StatusCode}
	case resp.StatusCode >= 500:
		return &Error{Code: CodeTelegramServer, Message: message, Retryable: true, StatusCode: resp.StatusCode}
	default:
		return &Error{Code: CodeTelegramBadRequest, Message: message, StatusCode: resp.StatusCode}
	}
}
