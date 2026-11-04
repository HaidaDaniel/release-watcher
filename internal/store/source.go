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

func (q *Queries) CreateSource(ctx context.Context, s *domain.Source) error {
	fields, err := json.Marshal(s.FieldsAvailable)
	if err != nil {
		return fmt.Errorf("store: encode source fields: %w", err)
	}
	_, err = q.q.ExecContext(ctx,
		`INSERT INTO source (id, kind, uri, external_id, fields_available, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.ID, string(s.Kind), s.URI, s.ExternalID, string(fields),
		formatTime(s.CreatedAt), formatTime(s.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create source: %w", err)
	}
	return nil
}

func (q *Queries) GetSource(ctx context.Context, id string) (*domain.Source, error) {
	var (
		s               domain.Source
		kind            string
		fields          string
		created, update string
	)
	err := q.q.QueryRowContext(ctx,
		`SELECT id, kind, uri, external_id, fields_available, created_at, updated_at
		 FROM source WHERE id = ?`, id,
	).Scan(&s.ID, &kind, &s.URI, &s.ExternalID, &fields, &created, &update)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get source: %w", err)
	}
	s.Kind = domain.SourceKind(kind)
	if err := json.Unmarshal([]byte(fields), &s.FieldsAvailable); err != nil {
		return nil, fmt.Errorf("store: decode source fields: %w", err)
	}
	if s.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if s.UpdatedAt, err = parseTime(update); err != nil {
		return nil, err
	}
	return &s, nil
}

func (q *Queries) GetSourceByKindExternal(ctx context.Context, kind domain.SourceKind, externalID string) (*domain.Source, error) {
	var id string
	err := q.q.QueryRowContext(ctx,
		`SELECT id FROM source WHERE kind = ? AND external_id = ?`,
		string(kind), externalID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: find source: %w", err)
	}
	return q.GetSource(ctx, id)
}

func (q *Queries) UpdateSourceFields(ctx context.Context, id string, fields []string, now time.Time) error {
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	res, err := q.q.ExecContext(ctx,
		`UPDATE source SET fields_available = ?, updated_at = ? WHERE id = ?`,
		string(data), formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("store: update source fields: %w", err)
	}
	return expectOne(res)
}
