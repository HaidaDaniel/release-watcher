package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultInterval = 24 * time.Hour
	DefaultTimeout  = 30 * time.Second
)

// Config contains process configuration. Secrets are read from the environment
// and are never written to disk by this package.
type Config struct {
	StateDir          string
	DBPath            string
	LockPath          string
	DefaultInterval   time.Duration
	HTTPTimeout       time.Duration
	UserAgent         string
	Lang              string
	LogFormat         string
	LogLevel          string
	TelegramToken     string
	TelegramChatID    string
	TelegramAPIBase   string
	AllowPrivateHosts bool
	MaxDeliveryTries  int
	MaxRedirects      int
}

// Load resolves configuration from environment variables using getenv.
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	stateDir, err := resolveStateDir(getenv)
	if err != nil {
		return Config{}, err
	}

	dbPath := strings.TrimSpace(getenv("RELEASE_WATCHER_DB"))
	if dbPath == "" {
		dbPath = filepath.Join(stateDir, "release-watcher.db")
	}

	interval, err := durationEnv(getenv, "RELEASE_WATCHER_DEFAULT_INTERVAL", DefaultInterval)
	if err != nil {
		return Config{}, err
	}
	if interval < time.Minute {
		return Config{}, fmt.Errorf("RELEASE_WATCHER_DEFAULT_INTERVAL must be at least 1m")
	}

	timeout, err := durationEnv(getenv, "RELEASE_WATCHER_HTTP_TIMEOUT", DefaultTimeout)
	if err != nil {
		return Config{}, err
	}
	if timeout < time.Second {
		return Config{}, fmt.Errorf("RELEASE_WATCHER_HTTP_TIMEOUT must be at least 1s")
	}

	lang := strings.ToLower(strings.TrimSpace(getenv("RELEASE_WATCHER_LANG")))
	if lang == "" {
		lang = "ru"
	}
	if lang != "ru" && lang != "en" {
		return Config{}, fmt.Errorf("RELEASE_WATCHER_LANG must be ru or en")
	}

	logFormat := strings.ToLower(strings.TrimSpace(getenv("RELEASE_WATCHER_LOG_FORMAT")))
	if logFormat == "" {
		logFormat = "text"
	}
	if logFormat != "text" && logFormat != "json" {
		return Config{}, fmt.Errorf("RELEASE_WATCHER_LOG_FORMAT must be text or json")
	}

	logLevel := strings.ToLower(strings.TrimSpace(getenv("RELEASE_WATCHER_LOG_LEVEL")))
	if logLevel == "" {
		logLevel = "info"
	}
	switch logLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("RELEASE_WATCHER_LOG_LEVEL must be debug, info, warn or error")
	}

	allowPrivate, err := boolEnv(getenv, "RELEASE_WATCHER_ALLOW_PRIVATE_HOSTS", false)
	if err != nil {
		return Config{}, err
	}

	maxTries := 5
	if raw := strings.TrimSpace(getenv("RELEASE_WATCHER_MAX_DELIVERY_ATTEMPTS")); raw != "" {
		maxTries, err = strconv.Atoi(raw)
		if err != nil || maxTries < 1 || maxTries > 50 {
			return Config{}, fmt.Errorf("RELEASE_WATCHER_MAX_DELIVERY_ATTEMPTS must be between 1 and 50")
		}
	}

	maxRedirects := 5
	if raw := strings.TrimSpace(getenv("RELEASE_WATCHER_MAX_REDIRECTS")); raw != "" {
		maxRedirects, err = strconv.Atoi(raw)
		if err != nil || maxRedirects < 0 || maxRedirects > 20 {
			return Config{}, fmt.Errorf("RELEASE_WATCHER_MAX_REDIRECTS must be between 0 and 20")
		}
	}

	telegramAPIBase := strings.TrimSpace(getenv("RELEASE_WATCHER_TELEGRAM_API_BASE"))
	if telegramAPIBase == "" {
		telegramAPIBase = "https://api.telegram.org"
	}
	if !strings.HasPrefix(telegramAPIBase, "http://") && !strings.HasPrefix(telegramAPIBase, "https://") {
		return Config{}, fmt.Errorf("RELEASE_WATCHER_TELEGRAM_API_BASE must be an http(s) URL")
	}

	userAgent := strings.TrimSpace(getenv("RELEASE_WATCHER_USER_AGENT"))
	if userAgent == "" {
		userAgent = "release-watcher/0.1 (+https://github.com/HaidaDaniel/release-watcher)"
	}

	return Config{
		StateDir:          stateDir,
		DBPath:            dbPath,
		LockPath:          filepath.Join(stateDir, "scan.lock"),
		DefaultInterval:   interval,
		HTTPTimeout:       timeout,
		UserAgent:         userAgent,
		Lang:              lang,
		LogFormat:         logFormat,
		LogLevel:          logLevel,
		TelegramToken:     strings.TrimSpace(getenv("RELEASE_WATCHER_TELEGRAM_BOT_TOKEN")),
		TelegramChatID:    strings.TrimSpace(getenv("RELEASE_WATCHER_TELEGRAM_CHAT_ID")),
		TelegramAPIBase:   strings.TrimRight(telegramAPIBase, "/"),
		AllowPrivateHosts: allowPrivate,
		MaxDeliveryTries:  maxTries,
		MaxRedirects:      maxRedirects,
	}, nil
}

// TelegramEnabled reports whether both required Telegram settings exist.
func (c Config) TelegramEnabled() bool {
	return c.TelegramToken != "" && c.TelegramChatID != ""
}

func resolveStateDir(getenv func(string) string) (string, error) {
	if raw := strings.TrimSpace(getenv("RELEASE_WATCHER_STATE_DIR")); raw != "" {
		return filepath.Clean(raw), nil
	}
	if raw := strings.TrimSpace(getenv("XDG_DATA_HOME")); raw != "" {
		return filepath.Join(raw, "release-watcher"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("cannot resolve home directory; set RELEASE_WATCHER_STATE_DIR")
	}
	return filepath.Join(home, ".local", "share", "release-watcher"), nil
}

func durationEnv(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 24h: %w", key, err)
	}
	return d, nil
}

func boolEnv(getenv func(string) string, key string, def bool) (bool, error) {
	raw := strings.ToLower(strings.TrimSpace(getenv(key)))
	if raw == "" {
		return def, nil
	}
	switch raw {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", key)
	}
}
