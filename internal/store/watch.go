package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
)

const watchColumns = `id, publication_id, enabled, interval_seconds, baseline_established_at,
	baseline_count, mode, next_check_at, last_check_at, last_success_at, last_error_category,
	consecutive_failures, paused_at, created_at, updated_at`

func (q *Queries) CreateWatch(ctx context.Context, w *domain.Watch) error {
	_, err := q.q.ExecContext(ctx,
		`INSERT INTO watch (`+watchColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.PublicationID, boolInt(w.Enabled), w.IntervalSeconds,
		formatTimePtr(w.BaselineEstablishedAt), int64PtrValue(w.BaselineCount), string(w.Mode),
		formatTime(w.NextCheckAt), formatTimePtr(w.LastCheckAt), formatTimePtr(w.LastSuccessAt),
		nullIfEmpty(w.LastErrorCategory), w.ConsecutiveFailures, formatTimePtr(w.PausedAt),
		formatTime(w.CreatedAt), formatTime(w.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create watch: %w", err)
	}
	return nil
}

func (q *Queries) GetWatch(ctx context.Context, id string) (*domain.Watch, error) {
	row := q.q.QueryRowContext(ctx, `SELECT `+watchColumns+` FROM watch WHERE id = ?`, id)
	w, err := scanWatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get watch: %w", err)
	}
	return w, nil
}

func (q *Queries) GetWatchByPublication(ctx context.Context, publicationID string) (*domain.Watch, error) {
	row := q.q.QueryRowContext(ctx, `SELECT `+watchColumns+` FROM watch WHERE publication_id = ?`, publicationID)
	w, err := scanWatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get watch by publication: %w", err)
	}
	return w, nil
}

func (q *Queries) ListWatches(ctx context.Context) ([]domain.Watch, error) {
	return q.listWatch(ctx, `SELECT `+watchColumns+` FROM watch ORDER BY created_at ASC, id ASC`)
}

func (q *Queries) ListDueWatches(ctx context.Context, now time.Time, limit int) ([]domain.Watch, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	return q.listWatch(ctx,
		`SELECT `+watchColumns+` FROM watch
		 WHERE enabled = 1 AND next_check_at <= ?
		 ORDER BY next_check_at ASC, id ASC LIMIT ?`,
		formatTime(now), limit,
	)
}

func (q *Queries) listWatch(ctx context.Context, query string, args ...any) ([]domain.Watch, error) {
	rows, err := q.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list watches: %w", err)
	}
	defer rows.Close()
	var out []domain.Watch
	for rows.Next() {
		w, err := scanWatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWatch(row rowScanner) (*domain.Watch, error) {
	var (
		w                                  domain.Watch
		enabled                            int
		mode                               string
		baselineAt, lastCheck, lastSuccess sql.NullString
		pausedAt                           sql.NullString
		baselineCount                      sql.NullInt64
		errorCategory                      sql.NullString
		nextCheck, created, updated        string
	)
	err := row.Scan(
		&w.ID, &w.PublicationID, &enabled, &w.IntervalSeconds, &baselineAt,
		&baselineCount, &mode, &nextCheck, &lastCheck, &lastSuccess, &errorCategory,
		&w.ConsecutiveFailures, &pausedAt, &created, &updated,
	)
	if err != nil {
		return nil, err
	}
	w.Enabled = enabled != 0
	w.Mode = domain.WatchMode(mode)
	w.LastErrorCategory = errorCategory.String
	if w.BaselineEstablishedAt, err = scanTime(baselineAt); err != nil {
		return nil, err
	}
	if w.LastCheckAt, err = scanTime(lastCheck); err != nil {
		return nil, err
	}
	if w.LastSuccessAt, err = scanTime(lastSuccess); err != nil {
		return nil, err
	}
	if w.PausedAt, err = scanTime(pausedAt); err != nil {
		return nil, err
	}
	w.BaselineCount = scanInt64Ptr(baselineCount)
	if w.NextCheckAt, err = parseTime(nextCheck); err != nil {
		return nil, err
	}
	if w.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if w.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &w, nil
}

func (q *Queries) UpdateWatchAfterSuccess(ctx context.Context, id string, lastCheck, lastSuccess, nextCheck time.Time, mode domain.WatchMode, now time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE watch SET last_check_at = ?, last_success_at = ?, next_check_at = ?,
			mode = ?, last_error_category = NULL, consecutive_failures = 0, updated_at = ?
		 WHERE id = ?`,
		formatTime(lastCheck), formatTime(lastSuccess), formatTime(nextCheck),
		string(mode), formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("store: update watch after success: %w", err)
	}
	return expectOne(res)
}

func (q *Queries) UpdateWatchAfterError(ctx context.Context, id string, lastCheck, nextCheck time.Time, category string, failures int, now time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE watch SET last_check_at = ?, next_check_at = ?, last_error_category = ?,
			consecutive_failures = ?, updated_at = ?
		 WHERE id = ?`,
		formatTime(lastCheck), formatTime(nextCheck), nullIfEmpty(category),
		failures, formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("store: update watch after error: %w", err)
	}
	return expectOne(res)
}

func (q *Queries) UpdateWatchBaseline(ctx context.Context, id string, established time.Time, count *int64, mode domain.WatchMode, now time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE watch SET baseline_established_at = ?, baseline_count = ?, mode = ?, updated_at = ?
		 WHERE id = ?`,
		formatTime(established), int64PtrValue(count), string(mode), formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("store: update watch baseline: %w", err)
	}
	return expectOne(res)
}

func (q *Queries) UpdateWatchSchedule(ctx context.Context, id string, intervalSeconds int64, nextCheck time.Time, now time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE watch SET interval_seconds = ?, next_check_at = ?, updated_at = ? WHERE id = ?`,
		intervalSeconds, formatTime(nextCheck), formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("store: update watch schedule: %w", err)
	}
	return expectOne(res)
}

func (q *Queries) SetWatchEnabled(ctx context.Context, id string, enabled bool, pausedAt *time.Time, now time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE watch SET enabled = ?, paused_at = ?, updated_at = ? WHERE id = ?`,
		boolInt(enabled), formatTimePtr(pausedAt), formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("store: set watch enabled: %w", err)
	}
	return expectOne(res)
}

func (q *Queries) DeleteWatch(ctx context.Context, id string) error {
	res, err := q.q.ExecContext(ctx, `DELETE FROM watch WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete watch: %w", err)
	}
	return expectOne(res)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
