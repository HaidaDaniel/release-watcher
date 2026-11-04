package app

import (
	"context"
	"fmt"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/id"
	"github.com/HaidaDaniel/release-watcher/internal/lock"
	"github.com/HaidaDaniel/release-watcher/internal/scan"
	"github.com/HaidaDaniel/release-watcher/internal/source"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

type ScanOptions struct {
	Mode     string
	ID       string
	NoNotify bool
}

func (s *Service) Scan(ctx context.Context, opts ScanOptions) (*ScanResult, *Error) {
	if opts.Mode == "" {
		opts.Mode = "due"
	}
	if opts.Mode != "due" && opts.Mode != "all" && opts.Mode != "id" {
		return nil, newError("invalid_argument", "scan mode must be due, all or id", nil)
	}
	if opts.Mode == "id" && opts.ID == "" {
		return nil, newError("invalid_argument", "scan --id requires a watch id", nil)
	}
	release, err := lock.Acquire(ctx, s.cfg.LockPath)
	if err != nil {
		return nil, mapError(err)
	}
	defer release()

	start := s.now()
	q := s.store.Queries()
	var watches []domain.Watch
	switch opts.Mode {
	case "id":
		watch, getErr := q.GetWatch(ctx, opts.ID)
		if getErr != nil {
			return nil, mapError(getErr)
		}
		watches = []domain.Watch{*watch}
	case "all":
		all, listErr := q.ListWatches(ctx)
		if listErr != nil {
			return nil, mapError(listErr)
		}
		for _, watch := range all {
			if watch.Enabled {
				watches = append(watches, watch)
			}
		}
	case "due":
		due, listErr := q.ListDueWatches(ctx, start, 1000)
		if listErr != nil {
			return nil, mapError(listErr)
		}
		watches = due
	}

	result := &ScanResult{StartedAt: start, Results: make([]ScanWatchResult, 0, len(watches))}
	for _, watch := range watches {
		if ctx.Err() != nil {
			break
		}
		watchResult := s.scanWatch(ctx, watch)
		result.Results = append(result.Results, watchResult)
		result.Scanned++
		if watchResult.Outcome == "error" {
			result.Failed++
		} else {
			result.Succeeded++
		}
		result.EventsCreated += watchResult.EventsCreated
	}
	result.FinishedAt = s.now()
	if !opts.NoNotify && s.notifier != nil {
		counts, flushErr := s.FlushDeliveries(ctx)
		if flushErr != nil {
			s.logger.Error("notification flush failed", "error", flushErr.Message)
		} else {
			result.Notifications = counts
		}
	}
	return result, nil
}

func (s *Service) scanWatch(parent context.Context, watch domain.Watch) ScanWatchResult {
	start := s.now()
	ctx, cancel := context.WithTimeout(parent, s.watchTimeout())
	defer cancel()
	result := ScanWatchResult{WatchID: watch.ID, PublicationID: watch.PublicationID}
	events, outcome, mode, err := s.scanOne(ctx, watch)
	result.DurationMS = s.now().Sub(start).Milliseconds()
	if err != nil {
		appErr := mapError(err)
		result.Outcome = "error"
		result.Error = &ErrorView{Code: appErr.Code, Message: appErr.Message, Details: appErr.Details}
		return result
	}
	result.Outcome = outcome
	result.EventsCreated = events
	result.Mode = &mode
	return result
}

func (s *Service) scanOne(ctx context.Context, watch domain.Watch) (int, string, string, error) {
	start := s.now()
	b, err := s.loadBundle(ctx, s.store.Queries(), watch.ID)
	if err != nil {
		return 0, "", "", err
	}
	snapshot, fetchErr := s.sources.FetchByKind(ctx, b.source.Kind, b.publication.OriginalURL)
	if fetchErr != nil {
		s.recordFailure(b, start, fetchErr)
		return 0, "", "", fetchErr
	}

	var (
		events  int
		outcome string
		mode    = string(snapshot.Mode)
	)
	txErr := s.store.InTx(ctx, func(q *store.Queries) error {
		fresh, loadErr := s.loadBundle(ctx, q, watch.ID)
		if loadErr != nil {
			return loadErr
		}
		releases, listErr := q.ListReleasesByPublication(ctx, fresh.publication.ID, true, 20000)
		if listErr != nil {
			return listErr
		}
		now := s.now()
		change, compareErr := scan.Compare(fresh.publication, fresh.watch, fresh.work, snapshot, releases, now)
		if compareErr != nil {
			return compareErr
		}
		if change.Baseline {
			return s.applyBaseline(ctx, q, fresh, snapshot, change, now, start)
		}
		if change.Reconcile {
			if err := s.applyBaseline(ctx, q, fresh, snapshot, change, now, start); err != nil {
				return err
			}
			outcome = "baseline"
			return nil
		}
		var count int
		count, outcome, err = s.applyChange(ctx, q, fresh, snapshot, change, now, start)
		if err != nil {
			return err
		}
		events = count
		return nil
	})
	if txErr != nil {
		s.recordFailure(b, start, txErr)
		return 0, "", "", txErr
	}
	if outcome == "" {
		outcome = "baseline"
	}
	if outcome == "baseline" {
		events = 0
	}
	return events, outcome, mode, nil
}

func (s *Service) applyBaseline(ctx context.Context, q *store.Queries, b *bundle, snapshot *source.Snapshot, change scan.Change, now, started time.Time) error {
	if snapshot.Metadata.OriginalTitle != "" && snapshot.Metadata.OriginalTitle != b.work.OriginalTitle {
		if err := q.UpdateWorkOriginalTitle(ctx, b.work.ID, snapshot.Metadata.OriginalTitle, now); err != nil {
			return err
		}
	}
	for i := range change.Added {
		release := change.Added[i]
		if release.ID == "" {
			release.ID = id.New("rel")
			release.IsBaseline = true
			if err := q.InsertRelease(ctx, &release); err != nil {
				return err
			}
		} else {
			release.IsBaseline = true
			if err := q.UpdateRelease(ctx, &release); err != nil {
				return err
			}
		}
	}
	for _, release := range change.Removed {
		if err := q.MarkReleaseRemoved(ctx, release.ID, now); err != nil {
			return err
		}
	}
	count := int64(len(change.Added))
	if snapshot.Mode == domain.ModeCountOnly || len(change.Added) == 0 {
		count = snapshot.Count
	}
	nextCheck := now.Add(time.Duration(b.watch.IntervalSeconds) * time.Second)
	if err := q.UpdateWatchBaseline(ctx, b.watch.ID, now, &count, change.Mode, now); err != nil {
		return err
	}
	if err := q.UpdateWatchAfterSuccess(ctx, b.watch.ID, now, now, nextCheck, change.Mode, now); err != nil {
		return err
	}
	b.publication.Status = snapshot.Metadata.Status
	b.publication.StatusSource = snapshot.Metadata.StatusSource
	b.publication.LastPublishedAt = snapshot.Metadata.LastPublishedAt
	b.publication.LastCheckedAt = &now
	b.publication.NextCheckAt = &nextCheck
	countValue := snapshot.Count
	if int64(len(change.Added)) > countValue {
		countValue = int64(len(change.Added))
	}
	b.publication.ReleaseCount = &countValue
	if err := q.UpdatePublicationAfterScan(ctx, b.publication, now); err != nil {
		return err
	}
	return q.InsertScanRun(ctx, &domain.ScanRun{
		ID: id.New("scan"), WatchID: b.watch.ID, PublicationID: b.publication.ID,
		StartedAt: started, FinishedAt: s.now(), Result: domain.ScanBaseline,
		DurationMillis: s.now().Sub(started).Milliseconds(),
	})
}

func (s *Service) applyChange(ctx context.Context, q *store.Queries, b *bundle, snapshot *source.Snapshot, change scan.Change, now, started time.Time) (int, string, error) {
	for i := range change.Added {
		release := change.Added[i]
		if release.ID == "" {
			release.ID = id.New("rel")
			release.IsBaseline = false
			if err := q.InsertRelease(ctx, &release); err != nil {
				return 0, "", err
			}
		} else {
			// Reappeared release: clear removed_at and update fields.
			if err := q.UpdateRelease(ctx, &release); err != nil {
				return 0, "", err
			}
		}
		change.Added[i] = release
	}
	for i := range change.Updated {
		release := change.Updated[i]
		if err := q.UpdateRelease(ctx, &release); err != nil {
			return 0, "", err
		}
	}
	if len(change.Touched) > 0 {
		if err := q.TouchReleases(ctx, change.Touched, now); err != nil {
			return 0, "", err
		}
	}
	for _, release := range change.Removed {
		if err := q.MarkReleaseRemoved(ctx, release.ID, now); err != nil {
			return 0, "", err
		}
	}
	if change.TitleChanged {
		if err := q.UpdateWorkOriginalTitle(ctx, b.work.ID, change.NewTitle, now); err != nil {
			return 0, "", err
		}
		b.work.OriginalTitle = change.NewTitle
	}

	interval := time.Duration(b.watch.IntervalSeconds) * time.Second
	nextCheck := now.Add(interval)
	if err := q.UpdateWatchAfterSuccess(ctx, b.watch.ID, now, now, nextCheck, change.Mode, now); err != nil {
		return 0, "", err
	}

	b.publication.Status = snapshot.Metadata.Status
	b.publication.StatusSource = snapshot.Metadata.StatusSource
	if snapshot.Metadata.LastPublishedAt != nil {
		if b.publication.LastPublishedAt == nil || snapshot.Metadata.LastPublishedAt.After(*b.publication.LastPublishedAt) {
			b.publication.LastPublishedAt = snapshot.Metadata.LastPublishedAt
		}
	}
	if latest := scan.LatestPublishedAt(append(append([]domain.Release{}, change.Added...), change.Updated...)); latest != nil {
		if b.publication.LastPublishedAt == nil || latest.After(*b.publication.LastPublishedAt) {
			b.publication.LastPublishedAt = latest
		}
	}
	b.publication.LastCheckedAt = &now
	b.publication.NextCheckAt = &nextCheck
	countValue := snapshot.Count
	if int64(len(change.Added)) > countValue {
		countValue = int64(len(change.Added))
	}
	b.publication.ReleaseCount = &countValue
	if err := q.UpdatePublicationAfterScan(ctx, b.publication, now); err != nil {
		return 0, "", err
	}

	events, buildErr := s.buildChangeEvents(b, change, now)
	if buildErr != nil {
		return 0, "", buildErr
	}
	created := 0
	var insertedEvents []*domain.Event
	for _, event := range events {
		inserted, insertErr := q.InsertEvent(ctx, event)
		if insertErr != nil {
			return 0, "", insertErr
		}
		if inserted {
			created++
			insertedEvents = append(insertedEvents, event)
		}
	}
	if s.notifier != nil {
		for _, event := range insertedEvents {
			_, deliveryErr := q.InsertDelivery(ctx, &domain.NotificationDelivery{
				ID:            id.New("del"),
				EventID:       event.ID,
				Channel:       s.notifier.Name(),
				Status:        domain.DeliveryPending,
				NextAttemptAt: now,
				CreatedAt:     now,
				UpdatedAt:     now,
			})
			if deliveryErr != nil {
				return 0, "", deliveryErr
			}
		}
	}
	result := domain.ScanUnchanged
	outcome := "unchanged"
	if created > 0 {
		result = domain.ScanOK
		outcome = "ok"
	}
	if err := q.InsertScanRun(ctx, &domain.ScanRun{
		ID: id.New("scan"), WatchID: b.watch.ID, PublicationID: b.publication.ID,
		StartedAt: started, FinishedAt: s.now(), Result: result,
		EventsCreated: created, DurationMillis: s.now().Sub(started).Milliseconds(),
	}); err != nil {
		return 0, "", err
	}
	return created, outcome, nil
}

func (s *Service) buildChangeEvents(b *bundle, change scan.Change, now time.Time) ([]*domain.Event, error) {
	var events []*domain.Event
	if change.CountChanged {
		eventType := domain.EventReleaseAdded
		if change.CountDelta < 0 {
			eventType = domain.EventReleaseCountChanged
		}
		previous, current, delta := change.PreviousCount, change.CurrentCount, change.CountDelta
		event, err := s.buildEvent(b, eventType, nil, &previous, &current, &delta, nil,
			[]string{fmt.Sprintf("%d", previous), fmt.Sprintf("%d", current)}, now)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if len(change.Added) > 0 {
		event, err := s.buildEvent(b, domain.EventReleaseAdded, change.Added, nil, nil, nil, nil,
			scan.ReleaseParts(change.Added), now)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if len(change.Updated) > 0 {
		event, err := s.buildEvent(b, domain.EventReleaseUpdated, change.Updated, nil, nil, nil, nil,
			scan.ReleaseParts(change.Updated), now)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if len(change.Removed) > 0 {
		event, err := s.buildEvent(b, domain.EventReleaseRemoved, change.Removed, nil, nil, nil, nil,
			scan.ReleaseParts(change.Removed), now)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if change.TitleChanged {
		oldTitle, newTitle := change.OldTitle, change.NewTitle
		event, err := s.buildEvent(b, domain.EventMetadataChanged, nil, nil, nil, nil,
			&payloadMetadata{OldOriginalTitle: &oldTitle, NewOriginalTitle: &newTitle},
			[]string{oldTitle, newTitle}, now)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if change.StatusChanged {
		oldStatus, newStatus := string(change.OldStatus), string(change.NewStatus)
		event, err := s.buildEvent(b, domain.EventStatusChanged, nil, nil, nil, nil,
			&payloadMetadata{OldStatus: &oldStatus, NewStatus: &newStatus},
			[]string{oldStatus, newStatus, change.OldStatusSource, change.NewStatusSource}, now)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Service) recordFailure(b *bundle, started time.Time, cause error) {
	appErr := mapError(cause)
	now := s.now()
	interval := time.Duration(b.watch.IntervalSeconds) * time.Second
	failures := b.watch.ConsecutiveFailures + 1
	nextCheck := now.Add(scan.RetryDelay(interval, failures))
	recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.store.InTx(recordCtx, func(q *store.Queries) error {
		if err := q.UpdateWatchAfterError(recordCtx, b.watch.ID, now, nextCheck, appErr.Code, failures, now); err != nil {
			return err
		}
		if err := q.UpdatePublicationCheckState(recordCtx, b.publication.ID, now, nextCheck, now); err != nil {
			return err
		}
		return q.InsertScanRun(recordCtx, &domain.ScanRun{
			ID: id.New("scan"), WatchID: b.watch.ID, PublicationID: b.publication.ID,
			StartedAt: started, FinishedAt: s.now(), Result: domain.ScanError,
			ErrorCode: appErr.Code, ErrorMessage: boundedMessage(appErr.Message),
			DurationMillis: s.now().Sub(started).Milliseconds(),
		})
	})
	if err != nil {
		s.logger.Error("failed to record scan failure", "watch_id", b.watch.ID, "error", err)
	}
}

func boundedMessage(value string) string {
	const max = 500
	if len(value) <= max {
		return value
	}
	return value[:max]
}
