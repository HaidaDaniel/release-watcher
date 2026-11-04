package app

import (
	"context"
	"time"
)

func (s *Service) Pause(ctx context.Context, watchID string) (*WatchView, *Error) {
	q := s.store.Queries()
	watch, err := q.GetWatch(ctx, watchID)
	if err != nil {
		return nil, mapError(err)
	}
	now := s.now()
	if !watch.Enabled && watch.PausedAt != nil {
		b, loadErr := s.loadBundle(ctx, q, watchID)
		if loadErr != nil {
			return nil, mapError(loadErr)
		}
		view := s.watchView(b)
		return &view, nil
	}
	if err := q.SetWatchEnabled(ctx, watchID, false, &now, now); err != nil {
		return nil, mapError(err)
	}
	b, loadErr := s.loadBundle(ctx, q, watchID)
	if loadErr != nil {
		return nil, mapError(loadErr)
	}
	view := s.watchView(b)
	return &view, nil
}

func (s *Service) Resume(ctx context.Context, watchID string) (*WatchView, *Error) {
	q := s.store.Queries()
	watch, err := q.GetWatch(ctx, watchID)
	if err != nil {
		return nil, mapError(err)
	}
	now := s.now()
	if watch.Enabled && watch.PausedAt == nil {
		b, loadErr := s.loadBundle(ctx, q, watchID)
		if loadErr != nil {
			return nil, mapError(loadErr)
		}
		view := s.watchView(b)
		return &view, nil
	}
	if err := q.UpdateWatchSchedule(ctx, watchID, watch.IntervalSeconds, now, now); err != nil {
		return nil, mapError(err)
	}
	if err := q.UpdatePublicationNextCheck(ctx, watch.PublicationID, now, now); err != nil {
		return nil, mapError(err)
	}
	if err := q.SetWatchEnabled(ctx, watchID, true, nil, now); err != nil {
		return nil, mapError(err)
	}
	b, loadErr := s.loadBundle(ctx, q, watchID)
	if loadErr != nil {
		return nil, mapError(loadErr)
	}
	view := s.watchView(b)
	return &view, nil
}

func (s *Service) SetInterval(ctx context.Context, watchID string, interval time.Duration) (*WatchView, *Error) {
	if interval < time.Minute {
		return nil, newError("invalid_argument", "interval must be at least 1m", nil)
	}
	q := s.store.Queries()
	watch, err := q.GetWatch(ctx, watchID)
	if err != nil {
		return nil, mapError(err)
	}
	now := s.now()
	nextCheck := now.Add(interval)
	if err := q.UpdateWatchSchedule(ctx, watchID, int64(interval/time.Second), nextCheck, now); err != nil {
		return nil, mapError(err)
	}
	if err := q.UpdatePublicationNextCheck(ctx, watch.PublicationID, nextCheck, now); err != nil {
		return nil, mapError(err)
	}
	b, loadErr := s.loadBundle(ctx, q, watchID)
	if loadErr != nil {
		return nil, mapError(loadErr)
	}
	view := s.watchView(b)
	return &view, nil
}

func (s *Service) Remove(ctx context.Context, watchID string) (*RemoveResult, *Error) {
	q := s.store.Queries()
	watch, err := q.GetWatch(ctx, watchID)
	if err != nil {
		return nil, mapError(err)
	}
	if err := s.store.DeleteObservation(ctx, watchID); err != nil {
		return nil, mapError(err)
	}
	return &RemoveResult{Removed: true, WatchID: watchID, PublicationID: watch.PublicationID}, nil
}
