package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/app"
)

func openScheduleEnv(t *testing.T, location *time.Location) (*testEnv, *naroFixture) {
	t.Helper()
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	if err := env.store.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := env.cfg
	cfg.Location = location
	return openServiceForConfig(t, cfg, n), n
}

func addWithInterval(t *testing.T, env *testEnv, interval time.Duration) *app.AddResult {
	t.Helper()
	added, err := env.svc.Add(context.Background(), app.AddInput{URL: naroURL, Interval: &interval})
	if err != nil {
		t.Fatal(err)
	}
	return added
}

func TestDailyScanSurvivesTimerJitter(t *testing.T) {
	env, _ := openScheduleEnv(t, time.UTC)
	clock := time.Date(2026, 11, 1, 4, 29, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addWithInterval(t, env, 24*time.Hour)
	wantNext := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	if !added.NextCheckAt.Equal(wantNext) {
		t.Fatalf("next check = %s, want %s", added.NextCheckAt, wantNext)
	}

	clock = time.Date(2026, 11, 2, 4, 18, 0, 0, time.UTC)
	result, err := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "due"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 1 {
		t.Fatalf("day 2 scan was skipped: %+v", result)
	}
	show, _ := env.svc.Show(context.Background(), added.WatchID)
	if !show.Watch.NextCheckAt.Equal(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("next check after day 2 = %s", show.Watch.NextCheckAt)
	}
}

func TestEveningAddIsDueAtFirstMorningRun(t *testing.T) {
	env, _ := openScheduleEnv(t, time.UTC)
	clock := time.Date(2026, 11, 1, 23, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addWithInterval(t, env, 24*time.Hour)
	if !added.NextCheckAt.Equal(time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("next check = %s", added.NextCheckAt)
	}

	clock = time.Date(2026, 11, 2, 4, 15, 0, 0, time.UTC)
	result, err := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "due"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 1 {
		t.Fatalf("first morning run did not scan: %+v", result)
	}
}

func TestMissedDaysCatchUpOnce(t *testing.T) {
	env, _ := openScheduleEnv(t, time.UTC)
	clock := time.Date(2026, 11, 1, 4, 29, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addWithInterval(t, env, 24*time.Hour)

	clock = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	result, err := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "due"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 1 {
		t.Fatalf("missed days not caught up: %+v", result)
	}
	show, _ := env.svc.Show(context.Background(), added.WatchID)
	if !show.Watch.NextCheckAt.Equal(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("next check = %s", show.Watch.NextCheckAt)
	}
	again, _ := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "due"})
	if again.Scanned != 0 {
		t.Fatalf("catch-up duplicated scan: %+v", again)
	}
}

func TestIntervalsUseCalendarOrDurationScheduling(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		want     time.Time
	}{
		{name: "6h", interval: 6 * time.Hour, want: time.Date(2026, 11, 1, 16, 0, 0, 0, time.UTC)},
		{name: "12h", interval: 12 * time.Hour, want: time.Date(2026, 11, 1, 22, 0, 0, 0, time.UTC)},
		{name: "24h", interval: 24 * time.Hour, want: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)},
		{name: "48h", interval: 48 * time.Hour, want: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := openScheduleEnv(t, time.UTC)
			clock := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
			env.svc.SetNow(func() time.Time { return clock })
			added := addWithInterval(t, env, tc.interval)
			if !added.NextCheckAt.Equal(tc.want) {
				t.Fatalf("next check = %s, want %s", added.NextCheckAt, tc.want)
			}
		})
	}
}

func TestManualScanAllKeepsCalendarWindow(t *testing.T) {
	env, _ := openScheduleEnv(t, time.UTC)
	clock := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addWithInterval(t, env, 24*time.Hour)

	if _, err := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "all"}); err != nil {
		t.Fatal(err)
	}
	show, _ := env.svc.Show(context.Background(), added.WatchID)
	if !show.Watch.NextCheckAt.Equal(time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("manual scan shifted window: %s", show.Watch.NextCheckAt)
	}

	clock = time.Date(2026, 11, 2, 4, 15, 0, 0, time.UTC)
	due, _ := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "due"})
	if due.Scanned != 1 {
		t.Fatalf("morning run after manual scan was skipped: %+v", due)
	}
}

func TestNoRepeatWithinPlannedWindow(t *testing.T) {
	env, _ := openScheduleEnv(t, time.UTC)
	clock := time.Date(2026, 11, 1, 4, 29, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	addWithInterval(t, env, 24*time.Hour)

	clock = time.Date(2026, 11, 2, 4, 18, 0, 0, time.UTC)
	first, _ := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "due"})
	if first.Scanned != 1 {
		t.Fatalf("first scan = %+v", first)
	}
	clock = time.Date(2026, 11, 2, 4, 20, 0, 0, time.UTC)
	second, _ := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "due"})
	if second.Scanned != 0 {
		t.Fatalf("repeat scan in same window = %+v", second)
	}
}

func TestDailyScheduleHandlesDSTTransitions(t *testing.T) {
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("Europe/Berlin timezone unavailable")
	}
	tests := []struct {
		name    string
		start   time.Time
		morning time.Time
		next    time.Time
	}{
		{
			name:    "spring forward",
			start:   time.Date(2026, 3, 28, 4, 29, 0, 0, location),
			morning: time.Date(2026, 3, 29, 4, 15, 0, 0, location),
			next:    time.Date(2026, 3, 30, 0, 0, 0, 0, location),
		},
		{
			name:    "fall back",
			start:   time.Date(2026, 10, 24, 4, 29, 0, 0, location),
			morning: time.Date(2026, 10, 25, 4, 15, 0, 0, location),
			next:    time.Date(2026, 10, 26, 0, 0, 0, 0, location),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := openScheduleEnv(t, location)
			clock := tc.start
			env.svc.SetNow(func() time.Time { return clock })
			added := addWithInterval(t, env, 24*time.Hour)
			wantNext := time.Date(tc.start.Year(), tc.start.Month(), tc.start.Day()+1, 0, 0, 0, 0, location)
			if !added.NextCheckAt.Equal(wantNext.UTC()) {
				t.Fatalf("next check = %s, want %s", added.NextCheckAt, wantNext.UTC())
			}
			clock = tc.morning
			result, err := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "due"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Scanned != 1 {
				t.Fatalf("DST morning scan skipped: %+v", result)
			}
			show, _ := env.svc.Show(context.Background(), added.WatchID)
			if !show.Watch.NextCheckAt.Equal(tc.next.UTC()) {
				t.Fatalf("next check after DST = %s, want %s", show.Watch.NextCheckAt, tc.next.UTC())
			}
		})
	}
}
