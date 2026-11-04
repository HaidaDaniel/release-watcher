package app

import (
	"context"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/store"
)

func (s *Service) Pause(ctx context.Context, watchID string) (*WatchView, *Error) {
	q := s.store.Queries()
	watch, err := q.GetWatch(ctx, watchID)
	if err != nil {
		return nil, mapError(err)
	}
	if watch.RemovedAt != nil {
		return nil, newError("invalid_argument", "watch is removed; re-add the URL to restore it", nil)
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
	now := s.now()
	txErr := s.store.InTx(ctx, func(q *store.Queries) error {
		watch, err := q.GetWatch(ctx, watchID)
		if err != nil {
			return err
		}
		if watch.RemovedAt != nil {
			return newError("invalid_argument", "watch is removed; re-add the URL to restore it", nil)
		}
		if watch.Enabled && watch.PausedAt == nil {
			return nil
		}
		if err := q.UpdateWatchSchedule(ctx, watchID, watch.IntervalSeconds, now, now); err != nil {
			return err
		}
		if err := q.UpdatePublicationNextCheck(ctx, watch.PublicationID, now, now); err != nil {
			return err
		}
		return q.SetWatchEnabled(ctx, watchID, true, nil, now)
	})
	if txErr != nil {
		return nil, mapError(txErr)
	}
	b, loadErr := s.loadBundle(ctx, s.store.Queries(), watchID)
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
	now := s.now()
	nextCheck := s.nextCheckAfter(now, interval)
	txErr := s.store.InTx(ctx, func(q *store.Queries) error {
		watch, err := q.GetWatch(ctx, watchID)
		if err != nil {
			return err
		}
		if watch.RemovedAt != nil {
			return newError("invalid_argument", "watch is removed; re-add the URL to restore it", nil)
		}
		if err := q.UpdateWatchSchedule(ctx, watchID, int64(interval/time.Second), nextCheck, now); err != nil {
			return err
		}
		return q.UpdatePublicationNextCheck(ctx, watch.PublicationID, nextCheck, now)
	})
	if txErr != nil {
		return nil, mapError(txErr)
	}
	b, loadErr := s.loadBundle(ctx, s.store.Queries(), watchID)
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
	if watch.RemovedAt == nil {
		if err := q.MarkWatchRemoved(ctx, watchID, s.now()); err != nil {
			return nil, mapError(err)
		}
	}
	return &RemoveResult{Removed: true, HistoryPreserved: true, WatchID: watchID, PublicationID: watch.PublicationID}, nil
}

func (s *Service) Purge(ctx context.Context, watchID string) (*PurgeResult, *Error) {
	q := s.store.Queries()
	watch, err := q.GetWatch(ctx, watchID)
	if err != nil {
		return nil, mapError(err)
	}
	if err := s.store.DeleteObservation(ctx, watchID); err != nil {
		return nil, mapError(err)
	}
	return &PurgeResult{Purged: true, WatchID: watchID, PublicationID: watch.PublicationID}, nil
}
