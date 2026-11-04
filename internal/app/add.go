package app

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/id"
	"github.com/HaidaDaniel/release-watcher/internal/source"
	"github.com/HaidaDaniel/release-watcher/internal/source/httpx"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

type AddInput struct {
	URL      string
	TitleRU  *string
	TitleEN  *string
	Type     *domain.PublicationType
	Interval *time.Duration
}

func (s *Service) Add(ctx context.Context, in AddInput) (*AddResult, *Error) {
	if strings.TrimSpace(in.URL) == "" {
		return nil, newError("invalid_url", "a source URL is required", nil)
	}
	parsedURL, urlErr := httpx.ValidateURL(in.URL)
	if urlErr != nil {
		return nil, newError("invalid_url", "invalid source URL", urlErr)
	}
	start := s.now()
	interval, err := s.DefaultInterval(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	if in.Interval != nil {
		interval = *in.Interval
	}
	if interval < time.Minute {
		return nil, newError("invalid_argument", "interval must be at least 1m", nil)
	}

	if existing, found, findErr := s.findExistingAdd(ctx, in.URL, parsedURL); findErr != nil {
		return nil, findErr
	} else if found {
		if existing.RemovedAt != nil {
			return s.restoreAdd(ctx, existing)
		}
		return existing, nil
	}

	snapshot, fetchErr := s.sources.Fetch(ctx, strings.TrimSpace(in.URL))
	if fetchErr != nil {
		return nil, mapError(fetchErr)
	}
	if !snapshot.SourceKind.Valid() || !snapshot.Mode.Valid() {
		return nil, newError("unsupported_source", "adapter returned an unsupported source kind or mode", nil)
	}

	pubType := snapshot.Metadata.PublicationType
	if pubType == "" {
		pubType = domain.PublicationUnknown
	}
	if in.Type != nil {
		if !in.Type.Valid() {
			return nil, newError("invalid_argument", "invalid publication type", nil)
		}
		pubType = *in.Type
	}
	if !pubType.Valid() {
		pubType = domain.PublicationUnknown
	}

	originalTitle := strings.TrimSpace(snapshot.Metadata.OriginalTitle)
	if originalTitle == "" {
		originalTitle = snapshot.OriginalURL
	}
	now := s.now()
	nextCheck := now.Add(interval)

	var (
		createdWatchID string
		existingID     string
		baselineCount  int64
	)

	txErr := s.store.InTx(ctx, func(q *store.Queries) error {
		src, srcErr := q.GetSourceByKindExternal(ctx, snapshot.SourceKind, snapshot.ExternalID)
		switch {
		case srcErr == nil:
			if err := q.UpdateSourceFields(ctx, src.ID, snapshot.FieldsAvailable, now); err != nil {
				return err
			}
		case errors.Is(srcErr, store.ErrNotFound):
			src = &domain.Source{
				ID:              id.New("src"),
				Kind:            snapshot.SourceKind,
				URI:             snapshot.OriginalURL,
				ExternalID:      snapshot.ExternalID,
				FieldsAvailable: snapshot.FieldsAvailable,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if err := q.CreateSource(ctx, src); err != nil {
				return err
			}
		default:
			return srcErr
		}

		pub, pubErr := q.GetPublicationBySourceURL(ctx, src.ID, snapshot.OriginalURL)
		switch {
		case pubErr == nil:
			existingWatch, watchErr := q.GetWatchByPublication(ctx, pub.ID)
			if watchErr == nil {
				existingID = existingWatch.ID
				return nil
			}
			if !errors.Is(watchErr, store.ErrNotFound) {
				return watchErr
			}
			// Repair path: a publication exists without a watch. Establish a
			// silent baseline from already stored releases.
			active, countErr := q.CountActiveReleases(ctx, pub.ID)
			if countErr != nil {
				return countErr
			}
			count := active
			watchID := id.New("watch")
			watch := &domain.Watch{
				ID:                    watchID,
				PublicationID:         pub.ID,
				Enabled:               true,
				IntervalSeconds:       int64(interval / time.Second),
				BaselineEstablishedAt: &now,
				BaselineCount:         &count,
				Mode:                  snapshot.Mode,
				NextCheckAt:           nextCheck,
				LastCheckAt:           &now,
				LastSuccessAt:         &now,
				CreatedAt:             now,
				UpdatedAt:             now,
			}
			if err := q.CreateWatch(ctx, watch); err != nil {
				return err
			}
			pub.LastCheckedAt = &now
			pub.NextCheckAt = &nextCheck
			pub.Status = snapshot.Metadata.Status
			pub.StatusSource = snapshot.Metadata.StatusSource
			pub.LastPublishedAt = snapshot.Metadata.LastPublishedAt
			pub.ReleaseCount = &snapshot.Count
			pub.UpdatedAt = now
			if err := q.UpdatePublicationAfterScan(ctx, pub, now); err != nil {
				return err
			}
			createdWatchID = watchID
			baselineCount = count
			return q.InsertScanRun(ctx, &domain.ScanRun{
				ID: id.New("scan"), WatchID: watchID, PublicationID: pub.ID,
				StartedAt: start, FinishedAt: now, Result: domain.ScanBaseline,
				DurationMillis: now.Sub(start).Milliseconds(),
			})
		case errors.Is(pubErr, store.ErrNotFound):
			// create a new work/publication below
		default:
			return pubErr
		}

		work := &domain.Work{
			ID:            id.New("work"),
			OriginalTitle: originalTitle,
			TitleRU:       nonEmptyTitle(in.TitleRU),
			TitleEN:       nonEmptyTitle(in.TitleEN),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := q.CreateWork(ctx, work); err != nil {
			return err
		}
		baselineReleases := dedupeSourceReleases(snapshot.Releases)
		baselineCount = int64(len(baselineReleases))
		if snapshot.Mode == domain.ModeCountOnly || len(baselineReleases) == 0 {
			baselineCount = snapshot.Count
		}
		pubCount := snapshot.Count
		if baselineCount > pubCount {
			pubCount = baselineCount
		}
		pub = &domain.Publication{
			ID:              id.New("pub"),
			WorkID:          work.ID,
			SourceID:        src.ID,
			Type:            pubType,
			OriginalURL:     snapshot.OriginalURL,
			Status:          snapshot.Metadata.Status,
			StatusSource:    snapshot.Metadata.StatusSource,
			LastPublishedAt: snapshot.Metadata.LastPublishedAt,
			LastCheckedAt:   &now,
			NextCheckAt:     &nextCheck,
			ReleaseCount:    &pubCount,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := q.CreatePublication(ctx, pub); err != nil {
			return err
		}

		watchID := id.New("watch")
		watch := &domain.Watch{
			ID:                    watchID,
			PublicationID:         pub.ID,
			Enabled:               true,
			IntervalSeconds:       int64(interval / time.Second),
			BaselineEstablishedAt: &now,
			BaselineCount:         &baselineCount,
			Mode:                  snapshot.Mode,
			NextCheckAt:           nextCheck,
			LastCheckAt:           &now,
			LastSuccessAt:         &now,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		if err := q.CreateWatch(ctx, watch); err != nil {
			return err
		}
		for _, sr := range baselineReleases {
			release := baselineRelease(sr, pub.ID, now)
			if err := q.InsertRelease(ctx, &release); err != nil {
				return err
			}
		}
		if err := q.InsertScanRun(ctx, &domain.ScanRun{
			ID: id.New("scan"), WatchID: watchID, PublicationID: pub.ID,
			StartedAt: start, FinishedAt: now, Result: domain.ScanBaseline,
			DurationMillis: now.Sub(start).Milliseconds(),
		}); err != nil {
			return err
		}
		createdWatchID = watchID
		return nil
	})
	if txErr != nil {
		return nil, mapError(txErr)
	}

	watchID := createdWatchID
	created := true
	reason := ""
	if existingID != "" {
		watchID = existingID
		created = false
		reason = "already_exists"
	}
	b, loadErr := s.loadBundle(ctx, s.store.Queries(), watchID)
	if loadErr != nil {
		return nil, mapError(loadErr)
	}
	result := &AddResult{
		WatchView:             s.watchView(b),
		Created:               created,
		Reason:                reason,
		BaselineReleases:      baselineCount,
		FieldsAvailable:       snapshot.FieldsAvailable,
		Warnings:              snapshot.Warnings,
		ExternalWorkReference: snapshot.ExternalWorkReference,
	}
	if !created {
		if b.watch.BaselineCount != nil {
			result.BaselineReleases = *b.watch.BaselineCount
		} else if b.publication.ReleaseCount != nil {
			result.BaselineReleases = *b.publication.ReleaseCount
		}
	}
	if result.FieldsAvailable == nil {
		result.FieldsAvailable = []string{}
	}
	if result.Warnings == nil {
		result.Warnings = []string{}
	}
	return result, nil
}

func (s *Service) restoreAdd(ctx context.Context, existing *AddResult) (*AddResult, *Error) {
	now := s.now()
	txErr := s.store.InTx(ctx, func(q *store.Queries) error {
		watch, err := q.GetWatch(ctx, existing.WatchID)
		if err != nil {
			return err
		}
		if watch.RemovedAt == nil {
			return nil
		}
		nextCheck := now.Add(time.Duration(watch.IntervalSeconds) * time.Second)
		if err := q.RestoreWatch(ctx, watch.ID, nextCheck, now); err != nil {
			return err
		}
		return q.UpdatePublicationNextCheck(ctx, watch.PublicationID, nextCheck, now)
	})
	if txErr != nil {
		return nil, mapError(txErr)
	}
	b, loadErr := s.loadBundle(ctx, s.store.Queries(), existing.WatchID)
	if loadErr != nil {
		return nil, mapError(loadErr)
	}
	baseline := int64(0)
	if b.watch.BaselineCount != nil {
		baseline = *b.watch.BaselineCount
	} else if b.publication.ReleaseCount != nil {
		baseline = *b.publication.ReleaseCount
	}
	fields := existing.FieldsAvailable
	if fields == nil {
		fields = []string{}
	}
	return &AddResult{
		WatchView:             s.watchView(b),
		Created:               false,
		Reason:                "restored",
		BaselineReleases:      baseline,
		FieldsAvailable:       fields,
		Warnings:              []string{},
		ExternalWorkReference: existing.ExternalWorkReference,
	}, nil
}

func baselineRelease(r source.Release, publicationID string, now time.Time) domain.Release {
	hash := r.ContentHash
	if hash == "" {
		hash = source.HashRelease(r)
	}
	return domain.Release{
		ID:              id.New("rel"),
		PublicationID:   publicationID,
		ExternalID:      r.ExternalID,
		Kind:            r.Kind,
		Number:          r.Number,
		Title:           r.Title,
		URL:             r.URL,
		PublishedAt:     r.PublishedAt,
		SourceUpdatedAt: r.SourceUpdatedAt,
		ContentHash:     hash,
		IsBaseline:      true,
		FirstSeenAt:     now,
		LastSeenAt:      now,
	}
}

func (s *Service) findExistingAdd(ctx context.Context, rawURL string, u *url.URL) (*AddResult, bool, *Error) {
	u.Fragment = ""
	canonical := u.String()
	if _, naroCanonical, ok := source.ParseNaroURL(rawURL); ok {
		canonical = naroCanonical
	}
	q := s.store.Queries()
	pub, err := q.GetPublicationByURL(ctx, canonical)
	if errors.Is(err, store.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, mapError(err)
	}
	watch, err := q.GetWatchByPublication(ctx, pub.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, mapError(err)
	}
	b, err := s.loadBundle(ctx, q, watch.ID)
	if err != nil {
		return nil, false, mapError(err)
	}
	baseline := int64(0)
	if b.watch.BaselineCount != nil {
		baseline = *b.watch.BaselineCount
	} else if b.publication.ReleaseCount != nil {
		baseline = *b.publication.ReleaseCount
	}
	fields := b.source.FieldsAvailable
	if fields == nil {
		fields = []string{}
	}
	return &AddResult{
		WatchView:             s.watchView(b),
		Created:               false,
		Reason:                "already_exists",
		BaselineReleases:      baseline,
		FieldsAvailable:       fields,
		Warnings:              []string{},
		ExternalWorkReference: externalWorkReference(b.source),
	}, true, nil
}

func dedupeSourceReleases(releases []source.Release) []source.Release {
	seen := map[string]bool{}
	out := make([]source.Release, 0, len(releases))
	for _, r := range releases {
		if seen[r.ExternalID] {
			continue
		}
		seen[r.ExternalID] = true
		out = append(out, r)
	}
	return out
}
