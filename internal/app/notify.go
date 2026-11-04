package app

import (
	"context"
	"errors"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/lock"
	"github.com/HaidaDaniel/release-watcher/internal/notifier"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

// Notify flushes deliveries under the same process lock as scans, so a timer
// scan and a manual notify cannot send the same delivery concurrently.
func (s *Service) Notify(ctx context.Context) (NotificationCounts, *Error) {
	release, err := lock.Acquire(ctx, s.cfg.LockPath)
	if err != nil {
		return NotificationCounts{}, mapError(err)
	}
	defer release()
	return s.FlushDeliveries(ctx)
}

// FlushDeliveries sends deliveries that are due, records failures and returns
// current outbox counts. Events remain stored regardless of delivery outcome.
func (s *Service) FlushDeliveries(ctx context.Context) (NotificationCounts, *Error) {
	if s.notifier == nil {
		return s.DeliveryCounts(ctx)
	}
	q := s.store.Queries()
	now := s.now()
	due, err := q.ListDueDeliveries(ctx, now, s.cfg.MaxDeliveryTries, 100)
	if err != nil {
		return NotificationCounts{}, mapError(err)
	}
	for _, delivery := range due {
		event, getErr := q.GetEvent(ctx, delivery.EventID)
		if errors.Is(getErr, store.ErrNotFound) {
			continue
		}
		if getErr != nil {
			return NotificationCounts{}, mapError(getErr)
		}
		text := FormatEvent(s.cfg.Lang, event.Payload)
		sendErr := s.notifier.Send(ctx, text)
		if sendErr == nil {
			if err := q.MarkDeliverySent(ctx, delivery.ID, now); err != nil {
				return NotificationCounts{}, mapError(err)
			}
			continue
		}
		attempts := delivery.Attempts + 1
		retryable := true
		retryAfter := time.Duration(0)
		var notifierErr *notifier.Error
		if errors.As(sendErr, &notifierErr) {
			retryable = notifierErr.Retryable
			retryAfter = notifierErr.RetryAfter
		}
		nextAttempt := now.AddDate(100, 0, 0)
		if retryable && attempts < s.cfg.MaxDeliveryTries {
			nextAttempt = now.Add(deliveryBackoff(attempts, retryAfter))
		}
		if err := q.MarkDeliveryFailed(ctx, delivery.ID, nextAttempt, deliveryErrorMessage(sendErr), now); err != nil {
			return NotificationCounts{}, mapError(err)
		}
	}
	return s.DeliveryCounts(ctx)
}

// DeliveryCounts returns current outbox counts without sending anything.
func (s *Service) DeliveryCounts(ctx context.Context) (NotificationCounts, *Error) {
	counts, err := s.store.Queries().CountDeliveriesByStatus(ctx)
	if err != nil {
		return NotificationCounts{}, mapError(err)
	}
	return NotificationCounts{
		Pending: int(counts["pending"]),
		Sent:    int(counts["sent"]),
		Failed:  int(counts["failed"]),
	}, nil
}

func deliveryBackoff(attempts int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Minute
	for i := 1; i < attempts && delay < 6*time.Hour; i++ {
		delay *= 2
		if delay > 6*time.Hour {
			delay = 6 * time.Hour
		}
	}
	return delay
}

func deliveryErrorMessage(err error) string {
	var notifierErr *notifier.Error
	if errors.As(err, &notifierErr) {
		return boundedMessage(notifierErr.Message)
	}
	return boundedMessage("notification delivery failed")
}
