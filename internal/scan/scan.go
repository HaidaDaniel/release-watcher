// Package scan compares a normalized source snapshot with stored releases.
package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/source"
)

// Change is the pure result of comparing a snapshot with persisted state.
type Change struct {
	Baseline     bool
	Reconcile    bool
	Added        []domain.Release
	Updated      []domain.Release
	Removed      []domain.Release
	Touched      []string
	TitleChanged bool
	OldTitle     string
	NewTitle     string

	StatusChanged   bool
	OldStatus       domain.Status
	NewStatus       domain.Status
	OldStatusSource string
	NewStatusSource string

	CountChanged  bool
	PreviousCount int64
	CurrentCount  int64
	CountDelta    int64

	Mode     domain.WatchMode
	Complete bool
}

// Compare implements the baseline, catalog, count-only and reconciliation rules.
func Compare(pub *domain.Publication, watch *domain.Watch, work *domain.Work, snapshot *source.Snapshot, existing []domain.Release, now time.Time) (Change, error) {
	if pub == nil || watch == nil || work == nil || snapshot == nil {
		return Change{}, fmt.Errorf("scan: missing comparison input")
	}
	if !snapshot.Mode.Valid() {
		return Change{}, fmt.Errorf("scan: invalid snapshot mode %q", snapshot.Mode)
	}
	change := Change{Mode: snapshot.Mode, Complete: snapshot.Complete}

	if watch.BaselineEstablishedAt == nil {
		change.Baseline = true
		for _, r := range snapshot.Releases {
			change.Added = append(change.Added, toDomainRelease(r, pub.ID, now, true))
		}
		return change, nil
	}

	if watch.Mode == domain.ModeCountOnly && snapshot.Mode == domain.ModeCatalog && len(snapshot.Releases) > 0 && len(existing) == 0 {
		change.Reconcile = true
		change.Mode = domain.ModeCatalog
		for _, r := range snapshot.Releases {
			change.Added = append(change.Added, toDomainRelease(r, pub.ID, now, true))
		}
		return change, nil
	}

	if snapshot.Metadata.OriginalTitle != "" && snapshot.Metadata.OriginalTitle != work.OriginalTitle {
		change.TitleChanged = true
		change.OldTitle = work.OriginalTitle
		change.NewTitle = snapshot.Metadata.OriginalTitle
	}
	if pub.Status != snapshot.Metadata.Status {
		change.StatusChanged = true
		change.OldStatus = pub.Status
		change.NewStatus = snapshot.Metadata.Status
		change.OldStatusSource = pub.StatusSource
		change.NewStatusSource = snapshot.Metadata.StatusSource
	}

	if snapshot.Mode == domain.ModeCountOnly || (snapshot.Mode == domain.ModeCatalog && len(snapshot.Releases) == 0 && !snapshot.Complete) {
		previous := pub.ReleaseCount
		if previous == nil {
			previous = watch.BaselineCount
		}
		if previous != nil && *previous != snapshot.Count {
			change.CountChanged = true
			change.PreviousCount = *previous
			change.CurrentCount = snapshot.Count
			change.CountDelta = snapshot.Count - *previous
		}
		return change, nil
	}

	byID := map[string]domain.Release{}
	for _, r := range existing {
		byID[r.ExternalID] = r
	}
	seen := map[string]bool{}
	for _, sr := range snapshot.Releases {
		seen[sr.ExternalID] = true
		current := toDomainRelease(sr, pub.ID, now, false)
		previous, ok := byID[sr.ExternalID]
		if !ok {
			change.Added = append(change.Added, current)
			continue
		}
		current.ID = previous.ID
		current.FirstSeenAt = previous.FirstSeenAt
		if previous.RemovedAt != nil {
			current.IsBaseline = previous.IsBaseline
			change.Added = append(change.Added, current)
			continue
		}
		if previous.ContentHash != current.ContentHash {
			change.Updated = append(change.Updated, current)
		} else {
			change.Touched = append(change.Touched, previous.ID)
		}
	}

	if snapshot.Complete {
		for _, previous := range existing {
			if previous.RemovedAt != nil || seen[previous.ExternalID] {
				continue
			}
			removed := previous
			removed.RemovedAt = &now
			removed.LastSeenAt = now
			change.Removed = append(change.Removed, removed)
		}
	}

	return change, nil
}

func toDomainRelease(r source.Release, publicationID string, now time.Time, baseline bool) domain.Release {
	hash := r.ContentHash
	if hash == "" {
		hash = source.HashRelease(r)
	}
	return domain.Release{
		PublicationID:   publicationID,
		ExternalID:      r.ExternalID,
		Kind:            r.Kind,
		Number:          r.Number,
		Title:           r.Title,
		URL:             r.URL,
		PublishedAt:     r.PublishedAt,
		SourceUpdatedAt: r.SourceUpdatedAt,
		ContentHash:     hash,
		IsBaseline:      baseline,
		FirstSeenAt:     now,
		LastSeenAt:      now,
	}
}

// DedupeKey builds a stable key from the watch, event type and sorted parts.
func DedupeKey(watchID string, eventType domain.EventType, parts ...string) string {
	sort.Strings(parts)
	h := sha256.New()
	write(h, watchID)
	write(h, string(eventType))
	for _, part := range parts {
		write(h, part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func write(h interface{ Write([]byte) (int, error) }, value string) {
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
}

// ReleaseParts returns stable dedupe parts for a release set.
func ReleaseParts(releases []domain.Release) []string {
	parts := make([]string, 0, len(releases))
	for _, r := range releases {
		parts = append(parts, r.ExternalID+"|"+r.ContentHash)
	}
	return parts
}

// LatestPublishedAt returns the latest non-nil release publication time.
func LatestPublishedAt(releases []domain.Release) *time.Time {
	var latest *time.Time
	for _, r := range releases {
		if r.PublishedAt == nil {
			continue
		}
		if latest == nil || r.PublishedAt.After(*latest) {
			t := *r.PublishedAt
			latest = &t
		}
	}
	return latest
}

// RetryDelay returns a bounded retry delay after a failed scan.
func RetryDelay(interval time.Duration, failures int) time.Duration {
	if interval <= 0 {
		return time.Hour
	}
	delay := time.Hour
	if interval < delay {
		delay = interval
	}
	for i := 1; i < failures; i++ {
		if delay >= interval {
			break
		}
		delay *= 2
		if delay > interval {
			delay = interval
		}
	}
	return delay
}
