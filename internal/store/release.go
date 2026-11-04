package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
)

const releaseColumns = `id, publication_id, external_id, kind, number, title, url,
	published_at, source_updated_at, content_hash, is_baseline, first_seen_at, last_seen_at, removed_at`

func (q *Queries) InsertRelease(ctx context.Context, r *domain.Release) error {
	_, err := q.q.ExecContext(ctx,
		`INSERT INTO release (`+releaseColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.PublicationID, r.ExternalID, string(r.Kind), stringPtrValue(r.Number),
		stringPtrValue(r.Title), stringPtrValue(r.URL), formatTimePtr(r.PublishedAt),
		formatTimePtr(r.SourceUpdatedAt), r.ContentHash, boolInt(r.IsBaseline),
		formatTime(r.FirstSeenAt), formatTime(r.LastSeenAt), formatTimePtr(r.RemovedAt),
	)
	if err != nil {
		return fmt.Errorf("store: insert release: %w", err)
	}
	return nil
}

func (q *Queries) GetReleaseByExternalID(ctx context.Context, publicationID, externalID string) (*domain.Release, error) {
	row := q.q.QueryRowContext(ctx,
		`SELECT `+releaseColumns+` FROM release WHERE publication_id = ? AND external_id = ?`,
		publicationID, externalID,
	)
	r, err := scanRelease(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get release: %w", err)
	}
	return r, nil
}

func (q *Queries) ListReleasesByPublication(ctx context.Context, publicationID string, includeRemoved bool, limit int) ([]domain.Release, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	query := `SELECT ` + releaseColumns + ` FROM release WHERE publication_id = ?`
	if !includeRemoved {
		query += ` AND removed_at IS NULL`
	}
	query += ` ORDER BY COALESCE(published_at, first_seen_at) DESC, id DESC LIMIT ?`
	rows, err := q.q.QueryContext(ctx, query, publicationID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list releases: %w", err)
	}
	defer rows.Close()
	var out []domain.Release
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (q *Queries) UpdateRelease(ctx context.Context, r *domain.Release) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE release SET kind = ?, number = ?, title = ?, url = ?, published_at = ?,
			source_updated_at = ?, content_hash = ?, last_seen_at = ?, removed_at = ?
		 WHERE id = ?`,
		string(r.Kind), stringPtrValue(r.Number), stringPtrValue(r.Title), stringPtrValue(r.URL),
		formatTimePtr(r.PublishedAt), formatTimePtr(r.SourceUpdatedAt), r.ContentHash,
		formatTime(r.LastSeenAt), formatTimePtr(r.RemovedAt), r.ID,
	)
	if err != nil {
		return fmt.Errorf("store: update release: %w", err)
	}
	return expectOne(res)
}

func (q *Queries) MarkReleaseRemoved(ctx context.Context, id string, removedAt time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE release SET removed_at = ? WHERE id = ?`,
		formatTime(removedAt), id,
	)
	if err != nil {
		return fmt.Errorf("store: mark release removed: %w", err)
	}
	return expectOne(res)
}

func (q *Queries) CountActiveReleases(ctx context.Context, publicationID string) (int64, error) {
	var count int64
	err := q.q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM release WHERE publication_id = ? AND removed_at IS NULL`,
		publicationID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count releases: %w", err)
	}
	return count, nil
}

func scanRelease(row rowScanner) (*domain.Release, error) {
	var (
		r                   domain.Release
		kind                string
		number, title, url  sql.NullString
		published, updated  sql.NullString
		removed             sql.NullString
		isBaseline          int
		firstSeen, lastSeen string
	)
	err := row.Scan(
		&r.ID, &r.PublicationID, &r.ExternalID, &kind, &number, &title, &url,
		&published, &updated, &r.ContentHash, &isBaseline, &firstSeen, &lastSeen, &removed,
	)
	if err != nil {
		return nil, err
	}
	r.Kind = domain.ReleaseKind(kind)
	r.Number = scanStringPtr(number)
	r.Title = scanStringPtr(title)
	r.URL = scanStringPtr(url)
	r.IsBaseline = isBaseline != 0
	if r.PublishedAt, err = scanTime(published); err != nil {
		return nil, err
	}
	if r.SourceUpdatedAt, err = scanTime(updated); err != nil {
		return nil, err
	}
	if r.RemovedAt, err = scanTime(removed); err != nil {
		return nil, err
	}
	if r.FirstSeenAt, err = parseTime(firstSeen); err != nil {
		return nil, err
	}
	if r.LastSeenAt, err = parseTime(lastSeen); err != nil {
		return nil, err
	}
	return &r, nil
}
