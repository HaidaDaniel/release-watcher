package scan_test

import (
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/scan"
	"github.com/HaidaDaniel/release-watcher/internal/source"
)

func baseInputs() (*domain.Publication, *domain.Watch, *domain.Work) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	baseline := now
	pub := &domain.Publication{
		ID: "pub", Type: domain.PublicationWN, Status: domain.StatusOngoing,
		StatusSource: "naro:end", ReleaseCount: int64Ptr(2),
	}
	watch := &domain.Watch{
		ID: "watch", PublicationID: "pub", Mode: domain.ModeCatalog,
		IntervalSeconds: 3600, BaselineEstablishedAt: &baseline, BaselineCount: int64Ptr(2),
	}
	work := &domain.Work{ID: "work", OriginalTitle: "Title"}
	return pub, watch, work
}

func int64Ptr(v int64) *int64        { return &v }
func strPtr(v string) *string        { return &v }
func timePtr(t time.Time) *time.Time { return &t }

func release(id, hash string) domain.Release {
	return domain.Release{ID: "rel-" + id, PublicationID: "pub", ExternalID: id, Kind: domain.ReleaseChapter, ContentHash: hash}
}

func snapshot(mode domain.WatchMode, complete bool, releases ...source.Release) *source.Snapshot {
	return &source.Snapshot{SourceKind: domain.SourceNaro, Mode: mode, Complete: complete, Releases: releases}
}

func srcRelease(id, hash string) source.Release {
	return source.Release{ExternalID: id, Kind: domain.ReleaseChapter, ContentHash: hash}
}

func TestBaselineProducesNoChanges(t *testing.T) {
	pub, watch, work := baseInputs()
	watch.BaselineEstablishedAt = nil
	now := time.Now().UTC()
	change, err := scan.Compare(pub, watch, work, snapshot(domain.ModeCatalog, true, srcRelease("1", "h1"), srcRelease("2", "h2")), nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if !change.Baseline || len(change.Added) != 2 || change.CountChanged || len(change.Removed) != 0 {
		t.Fatalf("change = %+v", change)
	}
	for _, r := range change.Added {
		if !r.IsBaseline {
			t.Fatal("baseline release not marked")
		}
	}
}

func TestCatalogAddedUpdatedRemoved(t *testing.T) {
	pub, watch, work := baseInputs()
	existing := []domain.Release{release("1", "h1"), release("2", "h2")}
	snap := snapshot(domain.ModeCatalog, true, srcRelease("1", "h1"), srcRelease("2", "changed"), srcRelease("3", "h3"))
	change, err := scan.Compare(pub, watch, work, snap, existing, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(change.Added) != 1 || change.Added[0].ExternalID != "3" {
		t.Fatalf("added = %+v", change.Added)
	}
	if len(change.Updated) != 1 || change.Updated[0].ExternalID != "2" {
		t.Fatalf("updated = %+v", change.Updated)
	}
	if len(change.Removed) != 0 {
		t.Fatalf("removed = %+v", change.Removed)
	}
}

func TestIncompleteCatalogDoesNotRemove(t *testing.T) {
	pub, watch, work := baseInputs()
	existing := []domain.Release{release("1", "h1"), release("2", "h2")}
	change, err := scan.Compare(pub, watch, work, snapshot(domain.ModeCatalog, false, srcRelease("1", "h1")), existing, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(change.Removed) != 0 {
		t.Fatalf("incomplete catalog removed releases: %+v", change.Removed)
	}
}

func TestCompleteCatalogRemoves(t *testing.T) {
	pub, watch, work := baseInputs()
	existing := []domain.Release{release("1", "h1"), release("2", "h2")}
	change, err := scan.Compare(pub, watch, work, snapshot(domain.ModeCatalog, true, srcRelease("1", "h1")), existing, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(change.Removed) != 1 || change.Removed[0].ExternalID != "2" {
		t.Fatalf("removed = %+v", change.Removed)
	}
}

func TestCountOnlyChange(t *testing.T) {
	pub, watch, work := baseInputs()
	watch.Mode = domain.ModeCountOnly
	pub.ReleaseCount = int64Ptr(2)
	change, err := scan.Compare(pub, watch, work, snapshot(domain.ModeCountOnly, false), nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !change.CountChanged || change.PreviousCount != 2 || change.CurrentCount != 0 || change.CountDelta != -2 {
		t.Fatalf("change = %+v", change)
	}
}

func TestCountOnlyReconcile(t *testing.T) {
	pub, watch, work := baseInputs()
	watch.Mode = domain.ModeCountOnly
	pub.ReleaseCount = int64Ptr(2)
	change, err := scan.Compare(pub, watch, work, snapshot(domain.ModeCatalog, true, srcRelease("1", "h1"), srcRelease("2", "h2")), nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !change.Reconcile || change.CountChanged || len(change.Added) != 2 {
		t.Fatalf("change = %+v", change)
	}
	for _, r := range change.Added {
		if !r.IsBaseline {
			t.Fatal("reconciled release not baseline")
		}
	}
}

func TestMetadataAndStatusChanges(t *testing.T) {
	pub, watch, work := baseInputs()
	snap := snapshot(domain.ModeCatalog, true)
	snap.Metadata.OriginalTitle = "New"
	snap.Metadata.Status = domain.StatusCompleted
	snap.Metadata.StatusSource = "naro:end"
	change, err := scan.Compare(pub, watch, work, snap, nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !change.TitleChanged || change.OldTitle != "Title" || change.NewTitle != "New" {
		t.Fatalf("title = %+v", change)
	}
	if !change.StatusChanged || change.OldStatus != domain.StatusOngoing || change.NewStatus != domain.StatusCompleted {
		t.Fatalf("status = %+v", change)
	}
}

func TestDedupeKeyIsStable(t *testing.T) {
	a := scan.DedupeKey("w", domain.EventReleaseAdded, "b", "a")
	b := scan.DedupeKey("w", domain.EventReleaseAdded, "a", "b")
	if a != b {
		t.Fatalf("dedupe keys differ: %s / %s", a, b)
	}
	if a == scan.DedupeKey("w", domain.EventReleaseUpdated, "a", "b") {
		t.Fatal("event type does not affect dedupe key")
	}
}

func TestRetryDelayBounded(t *testing.T) {
	if got := scan.RetryDelay(time.Hour, 1); got != time.Hour {
		t.Fatalf("delay = %v", got)
	}
	if got := scan.RetryDelay(24*time.Hour, 1); got != time.Hour {
		t.Fatalf("delay = %v", got)
	}
	if got := scan.RetryDelay(2*time.Hour, 5); got != 2*time.Hour {
		t.Fatalf("delay = %v", got)
	}
}
