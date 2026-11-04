package e2e_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/app"
	"github.com/HaidaDaniel/release-watcher/internal/config"
	"github.com/HaidaDaniel/release-watcher/internal/source"
	"github.com/HaidaDaniel/release-watcher/internal/source/httpx"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

// TestLiveNaroBaselineSmoke is a polite, opt-in live check. Run it with
// RELEASE_WATCHER_LIVE_NARO=1. A network failure is reported as skipped
// (blocked), never as passed.
func TestLiveNaroBaselineSmoke(t *testing.T) {
	if os.Getenv("RELEASE_WATCHER_LIVE_NARO") != "1" {
		t.Skip("set RELEASE_WATCHER_LIVE_NARO=1 to run the live Narō smoke")
	}
	dir := t.TempDir()
	cfg := config.Config{
		StateDir:          dir,
		DBPath:            filepath.Join(dir, "release-watcher.db"),
		LockPath:          filepath.Join(dir, "scan.lock"),
		DefaultInterval:   24 * time.Hour,
		HTTPTimeout:       30 * time.Second,
		UserAgent:         "release-watcher-live-smoke/0.1 (+https://github.com/HaidaDaniel/release-watcher)",
		Lang:              "ru",
		LogFormat:         "text",
		LogLevel:          "error",
		MaxDeliveryTries:  3,
		MaxRedirects:      5,
		RateInterval:      time.Second,
		MaxRetries:        2,
		MaxBodyBytes:      4 << 20,
		AllowPrivateHosts: false,
	}
	client := httpx.New(httpx.Config{
		UserAgent: cfg.UserAgent, Timeout: cfg.HTTPTimeout, MaxRedirects: cfg.MaxRedirects,
		MaxRetries: cfg.MaxRetries, RateInterval: cfg.RateInterval, MaxBodyBytes: cfg.MaxBodyBytes,
		AllowPrivateHosts: cfg.AllowPrivateHosts,
	})
	st, err := store.Open(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	logger := slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := app.New(cfg, st, source.NewRegistry(client), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	added, appErr := svc.Add(ctx, app.AddInput{URL: naroURL})
	if appErr != nil {
		t.Skipf("live Narō unavailable (blocked, not passed): %v", appErr)
	}
	if !added.Created || added.ExternalWorkReference != "ncode:N2166FE" {
		t.Fatalf("add = %+v", added)
	}
	if added.OriginalTitle == "" || added.BaselineReleases < 1 {
		t.Fatalf("live metadata = title=%q baseline=%d", added.OriginalTitle, added.BaselineReleases)
	}
	if !added.StatusKnown || added.StatusSource != "naro:end" {
		t.Fatalf("live status = %q known=%v source=%q", added.Status, added.StatusKnown, added.StatusSource)
	}
	if added.Status != "ongoing" && added.Status != "completed" {
		t.Fatalf("live status = %q, want ongoing or completed", added.Status)
	}
	if events, eventErr := svc.Events(ctx, app.EventQuery{}); eventErr != nil || len(events.Events) != 0 {
		t.Fatalf("baseline emitted events: %+v err=%v", events, eventErr)
	}
	show, showErr := svc.Show(ctx, added.WatchID)
	if showErr != nil {
		t.Fatal(showErr)
	}
	if len(show.Releases) == 0 {
		t.Fatal("live catalog returned no releases")
	}
	if added.BaselineReleases > 100 && len(show.Releases) <= 100 {
		t.Fatalf("live pagination did not return more than one page: baseline=%d shown=%d", added.BaselineReleases, len(show.Releases))
	}
	var dated, urlOK int
	for _, release := range show.Releases {
		if release.PublishedAt != nil {
			dated++
		}
		if release.URL != nil && strings.Contains(*release.URL, "/n2166fe/") {
			urlOK++
		}
	}
	if dated == 0 || urlOK == 0 {
		t.Fatalf("live release dates=%d urls=%d", dated, urlOK)
	}
	t.Logf("live Narō: title=%q ncode=%s status=%s baseline=%d shown=%d paginated=%v",
		added.OriginalTitle, added.ExternalWorkReference, added.Status, added.BaselineReleases, len(show.Releases), added.BaselineReleases > 100)
}
