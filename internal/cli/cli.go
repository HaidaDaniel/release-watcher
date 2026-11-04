package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/HaidaDaniel/release-watcher/internal/buildinfo"
	"github.com/HaidaDaniel/release-watcher/internal/config"
)

const SchemaVersion = "1.0"

// Exit codes are part of the CLI contract.
const (
	ExitOK      = 0
	ExitError   = 1
	ExitUsage   = 2
	ExitPartial = 3
	ExitLocked  = 4
)

type runner struct {
	stdout io.Writer
	stderr io.Writer
	getenv func(string) string
	json   bool
	lang   string
	help   bool
}

type envelope struct {
	SchemaVersion string      `json:"schema_version"`
	OK            bool        `json:"ok"`
	Data          any         `json:"data,omitempty"`
	Error         *errorShape `json:"error,omitempty"`
}

type errorShape struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type commandError struct {
	code    string
	message string
	details map[string]any
	exit    int
}

func (e *commandError) Error() string { return e.message }

// Run executes the CLI and returns a process exit code. It is safe to call
// from tests with synthetic streams and environment.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if getenv == nil {
		getenv = os.Getenv
	}
	r := &runner{stdout: stdout, stderr: stderr, getenv: getenv}
	command, err := r.parse(args)
	if err != nil {
		return r.fail(err)
	}
	if command == "" {
		r.printHelp()
		return ExitOK
	}

	switch command {
	case "help":
		r.printHelp()
		return ExitOK
	case "version":
		return r.version()
	case "doctor":
		return r.doctor(ctx)
	default:
		return r.fail(&commandError{
			code:    "usage",
			message: fmt.Sprintf("unknown command %q", command),
			exit:    ExitUsage,
		})
	}
}

func (r *runner) parse(args []string) (string, error) {
	var command string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			r.json = true
		case arg == "--log-json":
			// Accepted for forward compatibility; logging format is read from
			// the environment by the logger in later tasks.
		case arg == "--lang":
			if i+1 >= len(args) {
				return "", &commandError{code: "usage", message: "--lang requires ru or en", exit: ExitUsage}
			}
			i++
			r.lang = args[i]
		case strings.HasPrefix(arg, "--lang="):
			r.lang = strings.TrimPrefix(arg, "--lang=")
		case arg == "--help" || arg == "-h":
			r.help = true
		case arg == "--version":
			command = "version"
		case strings.HasPrefix(arg, "-"):
			return "", &commandError{
				code:    "usage",
				message: fmt.Sprintf("unknown flag %q", arg),
				exit:    ExitUsage,
			}
		default:
			if command == "" {
				command = arg
			} else {
				return "", &commandError{
					code:    "usage",
					message: fmt.Sprintf("unexpected argument %q", arg),
					exit:    ExitUsage,
				}
			}
		}
	}
	if r.lang == "" {
		r.lang = strings.ToLower(strings.TrimSpace(r.getenv("RELEASE_WATCHER_LANG")))
	}
	if r.lang == "" {
		r.lang = "ru"
	}
	if r.lang != "ru" && r.lang != "en" {
		return "", &commandError{code: "usage", message: "--lang must be ru or en", exit: ExitUsage}
	}
	if r.help {
		return "help", nil
	}
	return command, nil
}

func (r *runner) version() int {
	info := buildinfo.Get()
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: info})
		return ExitOK
	}
	fmt.Fprintf(r.stdout, "%s %s\ncommit: %s\nbuilt: %s\nmode: %s\n%s\n",
		info.Name, info.Version, info.Commit, info.BuiltAt, info.BuildMode, info.GoVersion)
	return ExitOK
}

func (r *runner) doctor(ctx context.Context) int {
	_ = ctx
	cfg, err := config.Load(r.getenv)
	if err != nil {
		return r.fail(&commandError{code: "config_error", message: err.Error(), exit: ExitError})
	}

	type check struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	var checks []check
	add := func(id, status, message string) {
		checks = append(checks, check{ID: id, Status: status, Message: message})
	}

	add("config", "ok", "configuration parsed")
	if st, statErr := os.Stat(cfg.StateDir); statErr == nil {
		if st.IsDir() {
			add("state_dir", "ok", "state directory exists")
			if st.Mode().Perm()&0o077 != 0 {
				add("state_dir_permissions", "warn", "state directory is accessible by group/others; prefer 0700")
			} else {
				add("state_dir_permissions", "ok", "state directory is private")
			}
		} else {
			add("state_dir", "error", "state path exists but is not a directory")
		}
	} else if os.IsNotExist(statErr) {
		add("state_dir", "warn", "state directory does not exist yet; it will be created on first write")
	} else {
		add("state_dir", "error", "cannot inspect state directory")
	}

	if _, statErr := os.Stat(cfg.DBPath); statErr == nil {
		add("storage", "ok", "database file exists; schema checks arrive with the storage task")
	} else if os.IsNotExist(statErr) {
		add("storage", "warn", "database has not been initialized yet")
	} else {
		add("storage", "error", "cannot inspect database path")
	}

	if cfg.TelegramEnabled() {
		add("telegram", "ok", "Telegram is configured (no network request was made)")
	} else {
		add("telegram", "warn", "Telegram is not configured; events remain available through the CLI")
	}

	info := buildinfo.Get()
	if info.BuildMode == "release" && !buildinfo.ReleaseReady(info) {
		add("build_identity", "error", "release build is missing a valid commit or build time")
	} else if info.BuildMode == "development" {
		add("build_identity", "warn", "development build; not a release candidate")
	} else {
		add("build_identity", "ok", "build identity is valid")
	}

	if _, statErr := os.Stat(filepath.Join(".git", "hooks", "pre-commit")); statErr == nil {
		add("hooks", "ok", "pre-commit hook is installed")
	} else {
		add("hooks", "warn", "pre-commit hook is not installed; run pnpm exec lefthook install")
	}

	if _, statErr := os.Stat(filepath.Join(".github", "workflows")); statErr == nil {
		add("ci", "warn", "GitHub Actions is present before MVP acceptance; local hooks are the intended path")
	} else {
		add("ci", "ok", "no GitHub Actions workflow (intentional before MVP)")
	}

	healthy := true
	for _, c := range checks {
		if c.Status == "error" {
			healthy = false
		}
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: map[string]any{
			"healthy": healthy,
			"checks":  checks,
		}})
	} else {
		for _, c := range checks {
			fmt.Fprintf(r.stdout, "[%s] %s: %s\n", c.Status, c.ID, c.Message)
		}
		if healthy {
			fmt.Fprintln(r.stdout, "healthy: yes")
		} else {
			fmt.Fprintln(r.stdout, "healthy: no")
		}
	}
	if !healthy {
		return ExitError
	}
	return ExitOK
}

func (r *runner) writeJSON(v any) {
	enc := json.NewEncoder(r.stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func (r *runner) fail(err error) int {
	ce, ok := err.(*commandError)
	if !ok {
		ce = &commandError{code: "internal_error", message: "internal error", exit: ExitError}
	}
	if r.json {
		r.writeJSON(envelope{
			SchemaVersion: SchemaVersion,
			OK:            false,
			Error: &errorShape{
				Code:    ce.code,
				Message: ce.message,
				Details: ce.details,
			},
		})
	} else {
		fmt.Fprintf(r.stderr, "error: %s\n", ce.message)
	}
	if ce.exit == 0 {
		return ExitError
	}
	return ce.exit
}

func (r *runner) printHelp() {
	if r.lang == "en" {
		fmt.Fprint(r.stdout, helpEN)
		return
	}
	fmt.Fprint(r.stdout, helpRU)
}

const helpRU = `Release Watcher — отслеживание новых публикаций.

Использование:
  release-watcher <команда> [флаги]

Команды:
  version           показать версию и build identity
  doctor            проверить конфигурацию и окружение (read-only)
  help              показать эту справку

Глобальные флаги:
  --json            машиночитаемый JSON (schema_version 1.0)
  --lang ru|en      язык человекочитаемого вывода
  --help            справка

Документация: docs/integration-contract.md
`

const helpEN = `Release Watcher — watch for new publications.

Usage:
  release-watcher <command> [flags]

Commands:
  version           show version and build identity
  doctor            inspect configuration and environment (read-only)
  help              show this help

Global flags:
  --json            machine-readable JSON (schema_version 1.0)
  --lang ru|en      human output language
  --help            help

Documentation: docs/integration-contract.md
`
