package source_test

import (
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/config"
)

func TestConfigHTTPDefaultsAndOverrides(t *testing.T) {
	env := map[string]string{"RELEASE_WATCHER_STATE_DIR": t.TempDir()}
	cfg, err := config.Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RateInterval != time.Second || cfg.MaxRetries != 3 || cfg.MaxBodyBytes != 4<<20 {
		t.Fatalf("defaults: rate=%v retries=%d body=%d", cfg.RateInterval, cfg.MaxRetries, cfg.MaxBodyBytes)
	}
	env["RELEASE_WATCHER_RATE_INTERVAL"] = "250ms"
	env["RELEASE_WATCHER_MAX_RETRIES"] = "5"
	env["RELEASE_WATCHER_MAX_BODY_BYTES"] = "1048576"
	cfg, err = config.Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RateInterval != 250*time.Millisecond || cfg.MaxRetries != 5 || cfg.MaxBodyBytes != 1<<20 {
		t.Fatalf("overrides: rate=%v retries=%d body=%d", cfg.RateInterval, cfg.MaxRetries, cfg.MaxBodyBytes)
	}
}

func TestConfigRejectsInvalidHTTPValues(t *testing.T) {
	cases := map[string]string{
		"RELEASE_WATCHER_RATE_INTERVAL":  "not-a-duration",
		"RELEASE_WATCHER_MAX_RETRIES":    "99",
		"RELEASE_WATCHER_MAX_BODY_BYTES": "10",
	}
	for key, value := range cases {
		env := map[string]string{"RELEASE_WATCHER_STATE_DIR": t.TempDir(), key: value}
		if _, err := config.Load(func(k string) string { return env[k] }); err == nil {
			t.Errorf("%s=%q accepted", key, value)
		}
	}
}
