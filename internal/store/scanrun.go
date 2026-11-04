package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
)

func (q *Queries) InsertScanRun(ctx context.Context, r *domain.ScanRun) error {
	if !r.Result.Valid() {
		return fmt.Errorf("store: invalid scan result %q", r.Result)
	}
	_, err := q.q.ExecContext(ctx,
		`INSERT INTO scan_run (id, watch_id, publication_id, started_at, finished_at,
			result, events_created, error_code, error_message, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.WatchID, r.PublicationID, formatTime(r.StartedAt), formatTime(r.FinishedAt),
		string(r.Result), r.EventsCreated, nullIfEmpty(r.ErrorCode), nullIfEmpty(r.ErrorMessage),
		r.DurationMillis,
	)
	if err != nil {
		return fmt.Errorf("store: insert scan run: %w", err)
	}
	return nil
}

func (q *Queries) ListScanRunsByWatch(ctx context.Context, watchID string, limit int) ([]domain.ScanRun, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	rows, err := q.q.QueryContext(ctx,
		`SELECT id, watch_id, publication_id, started_at, finished_at, result,
		        events_created, error_code, error_message, duration_ms
		 FROM scan_run WHERE watch_id = ? ORDER BY finished_at DESC, id DESC LIMIT ?`,
		watchID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list scan runs: %w", err)
	}
	defer rows.Close()
	var out []domain.ScanRun
	for rows.Next() {
		var (
			r                       domain.ScanRun
			result                  string
			errorCode, errorMessage sql.NullString
			started, finished       string
		)
		if err := rows.Scan(&r.ID, &r.WatchID, &r.PublicationID, &started, &finished, &result,
			&r.EventsCreated, &errorCode, &errorMessage, &r.DurationMillis); err != nil {
			return nil, err
		}
		r.Result = domain.ScanResult(result)
		r.ErrorCode = errorCode.String
		r.ErrorMessage = errorMessage.String
		if r.StartedAt, err = parseTime(started); err != nil {
			return nil, err
		}
		if r.FinishedAt, err = parseTime(finished); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
