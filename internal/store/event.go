package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
)

const eventColumns = `id, event_type, publication_id, watch_id, source_id,
	external_work_reference, release_ids, previous_count, current_count, count_delta,
	payload, dedupe_key, schema_version, detected_at, published_at, url, created_at`

// InsertEvent inserts an event unless its dedupe key already exists. It returns
// true when a new row was created.
func (q *Queries) InsertEvent(ctx context.Context, e *domain.Event) (bool, error) {
	if !e.Type.Valid() {
		return false, fmt.Errorf("store: invalid event type %q", e.Type)
	}
	if e.SchemaVersion == "" || e.DedupeKey == "" {
		return false, fmt.Errorf("store: event schema version and dedupe key are required")
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return false, fmt.Errorf("store: event payload must be valid JSON")
	}
	releaseIDs, err := json.Marshal(nonNilStrings(e.ReleaseIDs))
	if err != nil {
		return false, fmt.Errorf("store: encode release ids: %w", err)
	}
	res, err := q.q.ExecContext(ctx,
		`INSERT INTO event (`+eventColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (dedupe_key) DO NOTHING`,
		e.ID, string(e.Type), e.PublicationID, e.WatchID, e.SourceID,
		e.ExternalWorkReference, string(releaseIDs), int64PtrValue(e.PreviousCount),
		int64PtrValue(e.CurrentCount), int64PtrValue(e.CountDelta), string(e.Payload),
		e.DedupeKey, e.SchemaVersion, formatTime(e.DetectedAt), formatTimePtr(e.PublishedAt),
		stringPtrValue(e.URL), formatTime(e.CreatedAt),
	)
	if err != nil {
		return false, fmt.Errorf("store: insert event: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (q *Queries) GetEvent(ctx context.Context, id string) (*domain.Event, error) {
	row := q.q.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM event WHERE id = ?`, id)
	e, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get event: %w", err)
	}
	return e, nil
}

type EventFilter struct {
	PublicationID string
	WatchID       string
	Since         *time.Time
	After         string
	Limit         int
}

func (q *Queries) ListEvents(ctx context.Context, filter EventFilter) ([]domain.Event, error) {
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	query := `SELECT ` + eventColumns + ` FROM event WHERE 1 = 1`
	args := []any{}
	if filter.PublicationID != "" {
		query += ` AND publication_id = ?`
		args = append(args, filter.PublicationID)
	}
	if filter.WatchID != "" {
		query += ` AND watch_id = ?`
		args = append(args, filter.WatchID)
	}
	if filter.Since != nil {
		query += ` AND detected_at >= ?`
		args = append(args, formatTime(*filter.Since))
	}
	if filter.After != "" {
		var cursorTime string
		err := q.q.QueryRowContext(ctx, `SELECT detected_at FROM event WHERE id = ?`, filter.After).Scan(&cursorTime)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("store: resolve event cursor: %w", err)
		}
		query += ` AND (detected_at > ? OR (detected_at = ? AND id > ?))`
		args = append(args, cursorTime, cursorTime, filter.After)
	}
	query += ` ORDER BY detected_at ASC, id ASC LIMIT ?`
	args = append(args, filter.Limit)

	rows, err := q.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list events: %w", err)
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (q *Queries) CountEvents(ctx context.Context, publicationID string) (int64, error) {
	var count int64
	query := `SELECT COUNT(*) FROM event`
	args := []any{}
	if publicationID != "" {
		query += ` WHERE publication_id = ?`
		args = append(args, publicationID)
	}
	if err := q.q.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("store: count events: %w", err)
	}
	return count, nil
}

func scanEvent(row rowScanner) (*domain.Event, error) {
	var (
		e                        domain.Event
		eventType, releaseIDs    string
		previous, current, delta sql.NullInt64
		payload, url, published  sql.NullString
		detected, created        string
	)
	err := row.Scan(
		&e.ID, &eventType, &e.PublicationID, &e.WatchID, &e.SourceID,
		&e.ExternalWorkReference, &releaseIDs, &previous, &current, &delta,
		&payload, &e.DedupeKey, &e.SchemaVersion, &detected, &published, &url, &created,
	)
	if err != nil {
		return nil, err
	}
	e.Type = domain.EventType(eventType)
	if err := json.Unmarshal([]byte(releaseIDs), &e.ReleaseIDs); err != nil {
		return nil, fmt.Errorf("store: decode release ids: %w", err)
	}
	e.ReleaseIDs = nonNilStrings(e.ReleaseIDs)
	e.PreviousCount = scanInt64Ptr(previous)
	e.CurrentCount = scanInt64Ptr(current)
	e.CountDelta = scanInt64Ptr(delta)
	if payload.Valid {
		e.Payload = json.RawMessage(payload.String)
	}
	if e.PublishedAt, err = scanTime(published); err != nil {
		return nil, err
	}
	e.URL = scanStringPtr(url)
	if e.DetectedAt, err = parseTime(detected); err != nil {
		return nil, err
	}
	if e.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	return &e, nil
}
