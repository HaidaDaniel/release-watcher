package app

import (
	"encoding/json"
	"time"
)

// WatchView is the stable JSON shape for one observation.
type WatchView struct {
	WatchID           string     `json:"watch_id"`
	PublicationID     string     `json:"publication_id"`
	WorkID            string     `json:"work_id"`
	OriginalTitle     string     `json:"original_title"`
	TitleRU           *string    `json:"title_ru"`
	TitleEN           *string    `json:"title_en"`
	PublicationType   string     `json:"publication_type"`
	SourceType        string     `json:"source_type"`
	OriginalURL       string     `json:"original_url"`
	Status            string     `json:"status"`
	StatusKnown       bool       `json:"status_known"`
	StatusSource      string     `json:"status_source"`
	LastPublishedAt   *time.Time `json:"last_published_at"`
	Enabled           bool       `json:"enabled"`
	IntervalSeconds   int64      `json:"interval_seconds"`
	LastCheckedAt     *time.Time `json:"last_checked_at"`
	LastSuccessAt     *time.Time `json:"last_success_at"`
	NextCheckAt       time.Time  `json:"next_check_at"`
	RemovedAt         *time.Time `json:"removed_at"`
	LastErrorCategory *string    `json:"last_error_category"`
	ReleaseCount      *int64     `json:"release_count"`
	Mode              string     `json:"mode"`
	Owner             *string    `json:"owner"`
}

type AddResult struct {
	WatchView
	Created               bool     `json:"created"`
	Reason                string   `json:"reason,omitempty"`
	BaselineReleases      int64    `json:"baseline_releases"`
	FieldsAvailable       []string `json:"fields_available"`
	Warnings              []string `json:"warnings"`
	ExternalWorkReference string   `json:"external_work_reference"`
}

type ListResult struct {
	Watches []WatchView `json:"watches"`
}

type WorkView struct {
	ID            string  `json:"id"`
	OriginalTitle string  `json:"original_title"`
	TitleRU       *string `json:"title_ru"`
	TitleEN       *string `json:"title_en"`
}

type PublicationView struct {
	ID              string     `json:"id"`
	WorkID          string     `json:"work_id"`
	SourceID        string     `json:"source_id"`
	PublicationType string     `json:"publication_type"`
	OriginalURL     string     `json:"original_url"`
	Status          string     `json:"status"`
	StatusKnown     bool       `json:"status_known"`
	StatusSource    string     `json:"status_source"`
	LastPublishedAt *time.Time `json:"last_published_at"`
	LastCheckedAt   *time.Time `json:"last_checked_at"`
	NextCheckAt     *time.Time `json:"next_check_at"`
	ReleaseCount    *int64     `json:"release_count"`
}

type SourceView struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	URI             string   `json:"uri"`
	ExternalID      string   `json:"external_id"`
	FieldsAvailable []string `json:"fields_available"`
}

type ReleaseView struct {
	ID              string     `json:"id"`
	ExternalID      string     `json:"external_id"`
	Kind            string     `json:"kind"`
	Number          *string    `json:"number"`
	Title           *string    `json:"title"`
	URL             *string    `json:"url"`
	PublishedAt     *time.Time `json:"published_at"`
	SourceUpdatedAt *time.Time `json:"source_updated_at"`
	IsBaseline      bool       `json:"is_baseline"`
	FirstSeenAt     time.Time  `json:"first_seen_at"`
	LastSeenAt      time.Time  `json:"last_seen_at"`
	RemovedAt       *time.Time `json:"removed_at"`
}

type ScanRunView struct {
	ID             string    `json:"id"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at"`
	Result         string    `json:"result"`
	EventsCreated  int       `json:"events_created"`
	ErrorCode      string    `json:"error_code,omitempty"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	DurationMillis int64     `json:"duration_ms"`
}

type ShowResult struct {
	Watch       WatchView         `json:"watch"`
	Work        WorkView          `json:"work"`
	Publication PublicationView   `json:"publication"`
	Source      SourceView        `json:"source"`
	Releases    []ReleaseView     `json:"releases"`
	Events      []json.RawMessage `json:"events"`
	Scans       []ScanRunView     `json:"scans"`
}

type EventsResult struct {
	Events     []json.RawMessage `json:"events"`
	NextCursor string            `json:"next_cursor"`
}

type ErrorView struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type NotificationCounts struct {
	Pending int `json:"pending"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
}

type ScanWatchResult struct {
	WatchID       string     `json:"watch_id"`
	PublicationID string     `json:"publication_id"`
	Outcome       string     `json:"outcome"`
	EventsCreated int        `json:"events_created"`
	Mode          *string    `json:"mode"`
	Error         *ErrorView `json:"error"`
	DurationMS    int64      `json:"duration_ms"`
}

type ScanResult struct {
	StartedAt     time.Time          `json:"started_at"`
	FinishedAt    time.Time          `json:"finished_at"`
	Scanned       int                `json:"scanned"`
	Succeeded     int                `json:"succeeded"`
	Failed        int                `json:"failed"`
	EventsCreated int                `json:"events_created"`
	Notifications NotificationCounts `json:"notifications"`
	Results       []ScanWatchResult  `json:"results"`
}

type RemoveResult struct {
	Removed          bool   `json:"removed"`
	HistoryPreserved bool   `json:"history_preserved"`
	WatchID          string `json:"watch_id"`
	PublicationID    string `json:"publication_id"`
}

type PurgeResult struct {
	Purged        bool   `json:"purged"`
	WatchID       string `json:"watch_id"`
	PublicationID string `json:"publication_id"`
}

type GlobalIntervalResult struct {
	GlobalIntervalSeconds int64 `json:"global_interval_seconds"`
}
