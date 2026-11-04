package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
)

func (q *Queries) GetSetting(ctx context.Context, key string) (*domain.Setting, error) {
	var (
		s       domain.Setting
		updated string
	)
	err := q.q.QueryRowContext(ctx,
		`SELECT key, value, updated_at FROM setting WHERE key = ?`, key,
	).Scan(&s.Key, &s.Value, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get setting: %w", err)
	}
	if s.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &s, nil
}

func (q *Queries) SetSetting(ctx context.Context, key, value string, now time.Time) error {
	_, err := q.q.ExecContext(ctx,
		`INSERT INTO setting (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, formatTime(now),
	)
	if err != nil {
		return fmt.Errorf("store: set setting: %w", err)
	}
	return nil
}

func (q *Queries) DeleteSetting(ctx context.Context, key string) error {
	_, err := q.q.ExecContext(ctx, `DELETE FROM setting WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("store: delete setting: %w", err)
	}
	return nil
}
