package app

import (
	"context"
	"encoding/json"

	"github.com/HaidaDaniel/release-watcher/internal/store"
)

func (s *Service) Show(ctx context.Context, watchID string) (*ShowResult, *Error) {
	q := s.store.Queries()
	b, err := s.loadBundle(ctx, q, watchID)
	if err != nil {
		return nil, mapError(err)
	}
	releases, err := q.ListReleasesByPublication(ctx, b.publication.ID, true, 200)
	if err != nil {
		return nil, mapError(err)
	}
	events, err := q.ListEvents(ctx, store.EventFilter{WatchID: watchID, Limit: 20, Descending: true})
	if err != nil {
		return nil, mapError(err)
	}
	scans, err := q.ListScanRunsByWatch(ctx, watchID, 10)
	if err != nil {
		return nil, mapError(err)
	}

	releaseViews := make([]ReleaseView, 0, len(releases))
	for i := range releases {
		releaseViews = append(releaseViews, s.releaseView(&releases[i]))
	}
	eventViews := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		payload, payloadErr := rawEventPayload(event)
		if payloadErr != nil {
			return nil, mapError(payloadErr)
		}
		eventViews = append(eventViews, payload)
	}
	scanViews := make([]ScanRunView, 0, len(scans))
	for i := range scans {
		scanViews = append(scanViews, s.scanRunView(&scans[i]))
	}

	return &ShowResult{
		Watch:       s.watchView(b),
		Work:        s.workView(b.work),
		Publication: s.publicationView(b.publication),
		Source:      s.sourceView(b.source),
		Releases:    releaseViews,
		Events:      eventViews,
		Scans:       scanViews,
	}, nil
}
