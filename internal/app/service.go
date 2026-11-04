package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/config"
	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/notifier"
	"github.com/HaidaDaniel/release-watcher/internal/source"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

const defaultIntervalSetting = "default_interval_seconds"

// Service contains the application use cases shared by the CLI and future MCP.
type Service struct {
	cfg      config.Config
	store    *store.Store
	sources  *source.Registry
	now      func() time.Time
	logger   *slog.Logger
	notifier notifier.Notifier
}

func New(cfg config.Config, st *store.Store, sources *source.Registry, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{cfg: cfg, store: st, sources: sources, now: time.Now, logger: logger}
}

// SetNow injects a clock for deterministic tests.
func (s *Service) SetNow(now func() time.Time) { s.now = now }

// SetNotifier installs the optional delivery channel.
func (s *Service) SetNotifier(n notifier.Notifier) { s.notifier = n }

// NotificationsEnabled reports whether a delivery channel is configured.
func (s *Service) NotificationsEnabled() bool { return s.notifier != nil }

// SetLanguage changes human/notification language for tests and future callers.
func (s *Service) SetLanguage(lang string) {
	if lang == "en" {
		s.cfg.Lang = "en"
		return
	}
	s.cfg.Lang = "ru"
}

func (s *Service) DefaultInterval(ctx context.Context) (time.Duration, error) {
	setting, err := s.store.Queries().GetSetting(ctx, defaultIntervalSetting)
	if err == nil {
		seconds, parseErr := strconv.ParseInt(setting.Value, 10, 64)
		if parseErr == nil && seconds >= 60 {
			return time.Duration(seconds) * time.Second, nil
		}
	}
	return s.cfg.DefaultInterval, nil
}

func (s *Service) SetGlobalInterval(ctx context.Context, interval time.Duration) (*GlobalIntervalResult, *Error) {
	if interval < time.Minute {
		return nil, newError("invalid_argument", "interval must be at least 1m", nil)
	}
	oldInterval, err := s.DefaultInterval(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	seconds := int64(interval / time.Second)
	oldSeconds := int64(oldInterval / time.Second)
	now := s.now()
	nextCheck := now.Add(interval)
	txErr := s.store.InTx(ctx, func(q *store.Queries) error {
		if err := q.SetSetting(ctx, defaultIntervalSetting, strconv.FormatInt(seconds, 10), now); err != nil {
			return err
		}
		watches, err := q.ListWatches(ctx)
		if err != nil {
			return err
		}
		for _, watch := range watches {
			if watch.IntervalSeconds != oldSeconds {
				continue
			}
			if err := q.UpdateWatchSchedule(ctx, watch.ID, seconds, nextCheck, now); err != nil {
				return err
			}
			if err := q.UpdatePublicationNextCheck(ctx, watch.PublicationID, nextCheck, now); err != nil {
				return err
			}
		}
		return nil
	})
	if txErr != nil {
		return nil, mapError(txErr)
	}
	return &GlobalIntervalResult{GlobalIntervalSeconds: seconds}, nil
}

type bundle struct {
	watch       *domain.Watch
	publication *domain.Publication
	source      *domain.Source
	work        *domain.Work
}

func (s *Service) loadBundle(ctx context.Context, q *store.Queries, watchID string) (*bundle, error) {
	watch, err := q.GetWatch(ctx, watchID)
	if err != nil {
		return nil, err
	}
	pub, err := q.GetPublication(ctx, watch.PublicationID)
	if err != nil {
		return nil, err
	}
	src, err := q.GetSource(ctx, pub.SourceID)
	if err != nil {
		return nil, err
	}
	work, err := q.GetWork(ctx, pub.WorkID)
	if err != nil {
		return nil, err
	}
	return &bundle{watch: watch, publication: pub, source: src, work: work}, nil
}

func (s *Service) watchView(b *bundle) WatchView {
	v := WatchView{
		WatchID:         b.watch.ID,
		PublicationID:   b.publication.ID,
		WorkID:          b.work.ID,
		OriginalTitle:   b.work.OriginalTitle,
		TitleRU:         b.work.TitleRU,
		TitleEN:         b.work.TitleEN,
		PublicationType: string(b.publication.Type),
		SourceType:      string(b.source.Kind),
		OriginalURL:     b.publication.OriginalURL,
		Status:          string(b.publication.Status),
		StatusKnown:     b.publication.Status != domain.StatusUnknown,
		StatusSource:    b.publication.StatusSource,
		LastPublishedAt: b.publication.LastPublishedAt,
		Enabled:         b.watch.Enabled,
		IntervalSeconds: b.watch.IntervalSeconds,
		LastCheckedAt:   b.watch.LastCheckAt,
		LastSuccessAt:   b.watch.LastSuccessAt,
		NextCheckAt:     b.watch.NextCheckAt,
		ReleaseCount:    b.publication.ReleaseCount,
		Mode:            string(b.watch.Mode),
	}
	if b.watch.LastErrorCategory != "" {
		category := b.watch.LastErrorCategory
		v.LastErrorCategory = &category
	}
	return v
}

func (s *Service) publicationView(p *domain.Publication) PublicationView {
	v := PublicationView{
		ID:              p.ID,
		WorkID:          p.WorkID,
		SourceID:        p.SourceID,
		PublicationType: string(p.Type),
		OriginalURL:     p.OriginalURL,
		Status:          string(p.Status),
		StatusKnown:     p.Status != domain.StatusUnknown,
		StatusSource:    p.StatusSource,
		LastPublishedAt: p.LastPublishedAt,
		LastCheckedAt:   p.LastCheckedAt,
		NextCheckAt:     p.NextCheckAt,
		ReleaseCount:    p.ReleaseCount,
	}
	return v
}

func (s *Service) workView(w *domain.Work) WorkView {
	return WorkView{ID: w.ID, OriginalTitle: w.OriginalTitle, TitleRU: w.TitleRU, TitleEN: w.TitleEN}
}

func (s *Service) sourceView(src *domain.Source) SourceView {
	return SourceView{
		ID:              src.ID,
		Kind:            string(src.Kind),
		URI:             src.URI,
		ExternalID:      src.ExternalID,
		FieldsAvailable: src.FieldsAvailable,
	}
}

func (s *Service) releaseView(r *domain.Release) ReleaseView {
	return ReleaseView{
		ID:              r.ID,
		ExternalID:      r.ExternalID,
		Kind:            string(r.Kind),
		Number:          r.Number,
		Title:           r.Title,
		URL:             r.URL,
		PublishedAt:     r.PublishedAt,
		SourceUpdatedAt: r.SourceUpdatedAt,
		IsBaseline:      r.IsBaseline,
		FirstSeenAt:     r.FirstSeenAt,
		LastSeenAt:      r.LastSeenAt,
		RemovedAt:       r.RemovedAt,
	}
}

func (s *Service) scanRunView(r *domain.ScanRun) ScanRunView {
	return ScanRunView{
		ID:             r.ID,
		StartedAt:      r.StartedAt,
		FinishedAt:     r.FinishedAt,
		Result:         string(r.Result),
		EventsCreated:  r.EventsCreated,
		ErrorCode:      r.ErrorCode,
		ErrorMessage:   r.ErrorMessage,
		DurationMillis: r.DurationMillis,
	}
}

func rawEventPayload(e domain.Event) (json.RawMessage, error) {
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return nil, fmt.Errorf("app: event %s has invalid payload", e.ID)
	}
	return e.Payload, nil
}

func (s *Service) watchTimeout() time.Duration {
	timeout := s.cfg.HTTPTimeout * 4
	if timeout < 2*time.Minute {
		timeout = 2 * time.Minute
	}
	if timeout > 10*time.Minute {
		timeout = 10 * time.Minute
	}
	return timeout
}

func nonEmptyTitle(p *string) *string {
	if p == nil {
		return nil
	}
	value := *p
	if value == "" {
		return nil
	}
	return &value
}
