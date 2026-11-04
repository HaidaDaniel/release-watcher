package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
)

func (q *Queries) CreatePublication(ctx context.Context, p *domain.Publication) error {
	_, err := q.q.ExecContext(ctx,
		`INSERT INTO publication (
			id, work_id, source_id, publication_type, original_url, status, status_source,
			last_published_at, last_checked_at, next_check_at, release_count, mode, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.WorkID, p.SourceID, string(p.Type), p.OriginalURL, string(p.Status), nullIfEmpty(p.StatusSource),
		formatTimePtr(p.LastPublishedAt), formatTimePtr(p.LastCheckedAt), formatTimePtr(p.NextCheckAt),
		int64PtrValue(p.ReleaseCount), string(p.Mode), formatTime(p.CreatedAt), formatTime(p.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create publication: %w", err)
	}
	return nil
}

func (q *Queries) GetPublication(ctx context.Context, id string) (*domain.Publication, error) {
	var (
		p                             domain.Publication
		ptype, status, mode           string
		statusSource                  sql.NullString
		lastPub, lastCheck, nextCheck sql.NullString
		releaseCount                  sql.NullInt64
		created, updated              string
	)
	err := q.q.QueryRowContext(ctx,
		`SELECT id, work_id, source_id, publication_type, original_url, status, status_source,
		        last_published_at, last_checked_at, next_check_at, release_count, mode, created_at, updated_at
		 FROM publication WHERE id = ?`, id,
	).Scan(&p.ID, &p.WorkID, &p.SourceID, &ptype, &p.OriginalURL, &status, &statusSource,
		&lastPub, &lastCheck, &nextCheck, &releaseCount, &mode, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get publication: %w", err)
	}
	p.Type = domain.PublicationType(ptype)
	p.Status = domain.Status(status)
	p.StatusSource = statusSource.String
	p.Mode = domain.WatchMode(mode)
	if p.LastPublishedAt, err = scanTime(lastPub); err != nil {
		return nil, err
	}
	if p.LastCheckedAt, err = scanTime(lastCheck); err != nil {
		return nil, err
	}
	if p.NextCheckAt, err = scanTime(nextCheck); err != nil {
		return nil, err
	}
	p.ReleaseCount = scanInt64Ptr(releaseCount)
	if p.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if p.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &p, nil
}

func (q *Queries) GetPublicationBySourceURL(ctx context.Context, sourceID, originalURL string) (*domain.Publication, error) {
	var id string
	err := q.q.QueryRowContext(ctx,
		`SELECT id FROM publication WHERE source_id = ? AND original_url = ?`,
		sourceID, originalURL,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: find publication: %w", err)
	}
	return q.GetPublication(ctx, id)
}

func (q *Queries) ListPublications(ctx context.Context) ([]domain.Publication, error) {
	rows, err := q.q.QueryContext(ctx,
		`SELECT id FROM publication ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list publications: %w", err)
	}
	defer rows.Close()
	var out []domain.Publication
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		p, err := q.GetPublication(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// UpdatePublicationAfterScan records observation facts and schedule state.
func (q *Queries) UpdatePublicationAfterScan(ctx context.Context, p *domain.Publication, now time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE publication SET
			status = ?, status_source = ?, last_published_at = ?, last_checked_at = ?,
			next_check_at = ?, release_count = ?, mode = ?, updated_at = ?
		 WHERE id = ?`,
		string(p.Status), nullIfEmpty(p.StatusSource), formatTimePtr(p.LastPublishedAt),
		formatTimePtr(p.LastCheckedAt), formatTimePtr(p.NextCheckAt),
		int64PtrValue(p.ReleaseCount), string(p.Mode), formatTime(now), p.ID,
	)
	if err != nil {
		return fmt.Errorf("store: update publication: %w", err)
	}
	return expectOne(res)
}

func (q *Queries) SetPublicationStatus(ctx context.Context, id string, status domain.Status, source string, now time.Time) error {
	res, err := q.q.ExecContext(ctx,
		`UPDATE publication SET status = ?, status_source = ?, updated_at = ? WHERE id = ?`,
		string(status), nullIfEmpty(source), formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("store: set publication status: %w", err)
	}
	return expectOne(res)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
