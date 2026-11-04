package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
)

const deliveryColumns = `id, event_id, channel, status, attempts, next_attempt_at,
	last_error, sent_at, created_at, updated_at`

// InsertDelivery inserts a delivery unless (event_id, channel) already exists.
func (q *Queries) InsertDelivery(ctx context.Context, d *domain.NotificationDelivery) (bool, error) {
	res, err := q.q.ExecContext(ctx,
		`INSERT INTO notification_delivery (`+deliveryColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (event_id, channel) DO NOTHING`,
		d.ID, d.EventID, d.Channel, string(d.Status), d.Attempts, formatTime(d.NextAttemptAt),
		nullIfEmpty(d.LastError), formatTimePtr(d.SentAt), formatTime(d.CreatedAt), formatTime(d.UpdatedAt),
	)
	if err != nil {
		return false, fmt.Errorf("store: insert delivery: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (q *Queries) ListDueDeliveries(ctx context.Context, now time.Time, maxAttempts, limit int) ([]domain.NotificationDelivery, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := q.q.QueryContext(ctx,
		`SELECT `+deliveryColumns+` FROM notification_delivery
		 WHERE status IN ('pending', 'failed') AND next_attempt_at <= ? AND attempts < ?
		 ORDER BY next_attempt_at ASC, id ASC LIMIT ?`,
		formatTime(now), maxAttempts, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list due deliveries: %w", err)
	}
	defer rows.Close()
	var out []domain.NotificationDelivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (q *Queries) ListDeliveriesByEvent(ctx context.Context, eventID string) ([]domain.NotificationDelivery, error) {
	rows, err := q.q.QueryContext(ctx,
		`SELECT `+deliveryColumns+` FROM notification_delivery WHERE event_id = ? ORDER BY created_at ASC`,
		eventID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list deliveries: %w", err)
	}
	defer rows.Close()
	var out []domain.NotificationDelivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (q *Queries) MarkDeliverySent(ctx context.Context, id string, now time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE notification_delivery SET status = 'sent', attempts = attempts + 1,
			sent_at = ?, last_error = NULL, updated_at = ? WHERE id = ?`,
		formatTime(now), formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("store: mark delivery sent: %w", err)
	}
	return expectOne(res)
}

func (q *Queries) MarkDeliveryFailed(ctx context.Context, id string, nextAttempt time.Time, lastError string, now time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE notification_delivery SET status = 'failed', attempts = attempts + 1,
			next_attempt_at = ?, last_error = ?, updated_at = ? WHERE id = ?`,
		formatTime(nextAttempt), lastError, formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("store: mark delivery failed: %w", err)
	}
	return expectOne(res)
}

func (q *Queries) CountDeliveriesByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := q.q.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM notification_delivery GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("store: count deliveries: %w", err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[status] = count
	}
	return out, rows.Err()
}

func scanDelivery(row rowScanner) (*domain.NotificationDelivery, error) {
	var (
		d                 domain.NotificationDelivery
		status            string
		nextAttempt       string
		lastError, sentAt sql.NullString
		created, updated  string
	)
	err := row.Scan(
		&d.ID, &d.EventID, &d.Channel, &status, &d.Attempts, &nextAttempt,
		&lastError, &sentAt, &created, &updated,
	)
	if err != nil {
		return nil, err
	}
	d.Status = domain.DeliveryStatus(status)
	d.LastError = lastError.String
	if d.NextAttemptAt, err = parseTime(nextAttempt); err != nil {
		return nil, err
	}
	if d.SentAt, err = scanTime(sentAt); err != nil {
		return nil, err
	}
	if d.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if d.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &d, nil
}
