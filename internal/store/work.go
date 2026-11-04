package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
)

func (q *Queries) CreateWork(ctx context.Context, w *domain.Work) error {
	_, err := q.q.ExecContext(ctx,
		`INSERT INTO work (id, original_title, title_ru, title_en, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		w.ID, w.OriginalTitle, stringPtrValue(w.TitleRU), stringPtrValue(w.TitleEN),
		formatTime(w.CreatedAt), formatTime(w.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create work: %w", err)
	}
	return nil
}

func (q *Queries) GetWork(ctx context.Context, id string) (*domain.Work, error) {
	var (
		w               domain.Work
		ru, en          sql.NullString
		created, update string
	)
	err := q.q.QueryRowContext(ctx,
		`SELECT id, original_title, title_ru, title_en, created_at, updated_at
		 FROM work WHERE id = ?`, id,
	).Scan(&w.ID, &w.OriginalTitle, &ru, &en, &created, &update)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get work: %w", err)
	}
	w.TitleRU = scanStringPtr(ru)
	w.TitleEN = scanStringPtr(en)
	if w.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if w.UpdatedAt, err = parseTime(update); err != nil {
		return nil, err
	}
	return &w, nil
}

func (q *Queries) UpdateWorkTitles(ctx context.Context, id string, ru, en *string, now time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE work SET title_ru = ?, title_en = ?, updated_at = ? WHERE id = ?`,
		stringPtrValue(ru), stringPtrValue(en), formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("store: update work titles: %w", err)
	}
	return expectOne(res)
}

func expectOne(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *Queries) UpdateWorkOriginalTitle(ctx context.Context, id, title string, now time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE work SET original_title = ?, updated_at = ? WHERE id = ?`,
		title, formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("store: update work title: %w", err)
	}
	return expectOne(res)
}
