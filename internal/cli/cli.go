package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HaidaDaniel/release-watcher/internal/app"
	"github.com/HaidaDaniel/release-watcher/internal/buildinfo"
	"github.com/HaidaDaniel/release-watcher/internal/config"
	"github.com/HaidaDaniel/release-watcher/internal/i18n"
	"github.com/HaidaDaniel/release-watcher/internal/logging"
	"github.com/HaidaDaniel/release-watcher/internal/source"
	"github.com/HaidaDaniel/release-watcher/internal/source/httpx"
	"github.com/HaidaDaniel/release-watcher/internal/store"
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
	stdout  io.Writer
	stderr  io.Writer
	getenv  func(string) string
	json    bool
	logJSON bool
	lang    string
	help    bool
	version bool
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
	rest, err := r.parseGlobals(args)
	if err != nil {
		return r.fail(err)
	}
	if r.version && len(rest) == 0 {
		rest = []string{"version"}
	}
	if r.help {
		if len(rest) > 0 {
			r.printCommandHelp(rest[0])
		} else {
			r.printHelp()
		}
		return ExitOK
	}
	if len(rest) == 0 {
		r.printHelp()
		return ExitOK
	}

	command, commandArgs := rest[0], rest[1:]
	switch command {
	case "help":
		r.printHelp()
		return ExitOK
	case "version":
		return r.versionCmd(commandArgs)
	case "doctor":
		return r.doctor(ctx, commandArgs)
	case "add":
		return r.add(ctx, commandArgs)
	case "list":
		return r.list(ctx, commandArgs)
	case "show":
		return r.show(ctx, commandArgs)
	case "scan":
		return r.scan(ctx, commandArgs)
	case "events":
		return r.events(ctx, commandArgs)
	case "pause":
		return r.pause(ctx, commandArgs)
	case "resume":
		return r.resume(ctx, commandArgs)
	case "interval":
		return r.interval(ctx, commandArgs)
	case "remove":
		return r.remove(ctx, commandArgs)
	default:
		return r.fail(&commandError{
			code:    "usage",
			message: fmt.Sprintf("unknown command %q", command),
			exit:    ExitUsage,
		})
	}
}

func (r *runner) parseGlobals(args []string) ([]string, *commandError) {
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			r.json = true
		case arg == "--log-json":
			r.logJSON = true
		case arg == "--lang":
			if i+1 >= len(args) {
				return nil, &commandError{code: "usage", message: "--lang requires ru or en", exit: ExitUsage}
			}
			i++
			r.lang = args[i]
		case strings.HasPrefix(arg, "--lang="):
			r.lang = strings.TrimPrefix(arg, "--lang=")
		case arg == "--help" || arg == "-h":
			r.help = true
		case arg == "--version":
			r.version = true
		default:
			rest = append(rest, arg)
		}
	}
	if r.lang == "" {
		r.lang = strings.ToLower(strings.TrimSpace(r.getenv("RELEASE_WATCHER_LANG")))
	}
	if r.lang == "" {
		r.lang = "ru"
	}
	if r.lang != "ru" && r.lang != "en" {
		return nil, &commandError{code: "usage", message: "--lang must be ru or en", exit: ExitUsage}
	}
	return rest, nil
}

type flagKind int

const (
	boolFlag flagKind = iota
	valueFlag
)

func parseCommandArgs(args []string, specs map[string]flagKind) (map[string]string, []string, *commandError) {
	values := map[string]string{}
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "--") || arg == "--" {
			positionals = append(positionals, arg)
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		value := ""
		if idx := strings.IndexByte(name, '='); idx >= 0 {
			value = name[idx+1:]
			name = name[:idx]
		}
		kind, ok := specs[name]
		if !ok {
			return nil, nil, &commandError{code: "usage", message: fmt.Sprintf("unknown flag --%s", name), exit: ExitUsage}
		}
		switch kind {
		case boolFlag:
			if value == "" {
				values[name] = "true"
			} else if _, err := strconv.ParseBool(value); err != nil {
				return nil, nil, &commandError{code: "usage", message: fmt.Sprintf("--%s expects true or false", name), exit: ExitUsage}
			} else {
				values[name] = value
			}
		case valueFlag:
			if value == "" {
				if i+1 >= len(args) {
					return nil, nil, &commandError{code: "usage", message: fmt.Sprintf("--%s requires a value", name), exit: ExitUsage}
				}
				i++
				value = args[i]
			}
			values[name] = value
		}
	}
	return values, positionals, nil
}

func flagBool(values map[string]string, name string) bool {
	value, ok := values[name]
	if !ok {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func (r *runner) openService(ctx context.Context) (*app.Service, func(), *commandError) {
	cfg, err := config.Load(r.getenv)
	if err != nil {
		return nil, nil, &commandError{code: "config_error", message: err.Error(), exit: ExitError}
	}
	if r.logJSON {
		cfg.LogFormat = "json"
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, nil, &commandError{code: "storage_error", message: err.Error(), exit: ExitError}
	}
	logger := logging.New(cfg.LogFormat, cfg.LogLevel, r.stderr)
	client := httpx.New(httpx.Config{
		UserAgent:         cfg.UserAgent,
		Timeout:           cfg.HTTPTimeout,
		MaxRedirects:      cfg.MaxRedirects,
		MaxRetries:        cfg.MaxRetries,
		RateInterval:      cfg.RateInterval,
		MaxBodyBytes:      cfg.MaxBodyBytes,
		AllowPrivateHosts: cfg.AllowPrivateHosts,
	})
	reg := source.NewRegistry(client)
	svc := app.New(cfg, st, reg, logger)
	return svc, func() { _ = st.Close() }, nil
}

func (r *runner) versionCmd(args []string) int {
	if len(args) != 0 {
		return r.fail(&commandError{code: "usage", message: "version takes no arguments", exit: ExitUsage})
	}
	info := buildinfo.Get()
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: info})
		return ExitOK
	}
	fmt.Fprintf(r.stdout, "%s %s\ncommit: %s\nbuilt: %s\nmode: %s\n%s\n",
		info.Name, info.Version, info.Commit, info.BuiltAt, info.BuildMode, info.GoVersion)
	return ExitOK
}

func (r *runner) doctor(ctx context.Context, args []string) int {
	if len(args) != 0 {
		return r.fail(&commandError{code: "usage", message: "doctor takes no arguments", exit: ExitUsage})
	}
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
		add("storage", "ok", "database file exists")
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

	root := gitOutput(ctx, "", "rev-parse", "--show-toplevel")
	if root == "" {
		root, _ = os.Getwd()
	}
	hookPath := gitOutput(ctx, root, "rev-parse", "--path-format=absolute", "--git-path", "hooks/pre-commit")
	if hookPath == "" {
		hookPath = filepath.Join(root, ".git", "hooks", "pre-commit")
	}
	if _, statErr := os.Stat(hookPath); statErr == nil {
		add("hooks", "ok", "pre-commit hook is installed")
	} else {
		add("hooks", "warn", "pre-commit hook is not installed; run pnpm exec lefthook install")
	}

	if _, statErr := os.Stat(filepath.Join(root, ".github", "workflows")); statErr == nil {
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

func gitOutput(ctx context.Context, dir string, args ...string) string {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (r *runner) writeJSON(v any) {
	enc := json.NewEncoder(r.stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func (r *runner) fail(err *commandError) int {
	if err == nil {
		err = &commandError{code: "internal_error", message: "internal error", exit: ExitError}
	}
	if r.json {
		r.writeJSON(envelope{
			SchemaVersion: SchemaVersion,
			OK:            false,
			Error: &errorShape{
				Code:    err.code,
				Message: err.message,
				Details: err.details,
			},
		})
	} else {
		fmt.Fprintf(r.stderr, "%s %s\n", r.t("Ошибка:", "Error:"), err.message)
	}
	if err.exit == 0 {
		return ExitError
	}
	return err.exit
}

func (r *runner) failApp(err *app.Error) int {
	if err == nil {
		return r.fail(&commandError{code: "internal_error", message: "internal error", exit: ExitError})
	}
	exit := ExitError
	switch err.Code {
	case "usage", "invalid_argument", "invalid_url", "unsupported_source":
		exit = ExitUsage
	case "locked":
		exit = ExitLocked
	}
	return r.fail(&commandError{code: err.Code, message: err.Message, details: err.Details, exit: exit})
}

func (r *runner) t(ru, en string) string {
	return i18n.Text(r.lang, ru, en)
}

func (r *runner) printHelp() {
	if r.lang == "en" {
		fmt.Fprint(r.stdout, helpEN)
		return
	}
	fmt.Fprint(r.stdout, helpRU)
}

func (r *runner) printCommandHelp(command string) {
	if r.lang == "en" {
		fmt.Fprintf(r.stdout, "release-watcher %s\n\nSee docs/integration-contract.md for JSON contracts and flags.\n", command)
		return
	}
	fmt.Fprintf(r.stdout, "release-watcher %s\n\nФлаги и JSON-контракт: docs/integration-contract.md\n", command)
}

const helpRU = `Release Watcher — отслеживание новых публикаций.

Использование:
  release-watcher <команда> [флаги]

Команды:
  add <url>                 добавить источник и сохранить baseline
  list                      показать наблюдения
  show <id>                 показать наблюдение, релизы и события
  scan [--due|--all|--id]   проверить источники
  events                    прочитать события
  pause <id>                приостановить наблюдение
  resume <id>               возобновить наблюдение
  interval <id> <dur>       изменить интервал наблюдения
  interval --global <dur>   изменить интервал по умолчанию
  remove <id> --yes         удалить наблюдение и его данные
  doctor                    проверить конфигурацию и окружение
  version                   показать версию и build identity
  help                      показать эту справку

Глобальные флаги:
  --json                    машиночитаемый JSON (schema_version 1.0)
  --lang ru|en              язык человекочитаемого вывода
  --log-json                JSON-логи в stderr
  --help                    справка

Документация: docs/integration-contract.md
`

const helpEN = `Release Watcher — watch for new publications.

Usage:
  release-watcher <command> [flags]

Commands:
  add <url>                 add a source and store a baseline
  list                      list watches
  show <id>                 show a watch, releases and events
  scan [--due|--all|--id]   check sources
  events                    read events
  pause <id>                pause a watch
  resume <id>               resume a watch
  interval <id> <dur>       change a watch interval
  interval --global <dur>   change the default interval
  remove <id> --yes         remove a watch and its data
  doctor                    inspect configuration and environment
  version                   show version and build identity
  help                      show this help

Global flags:
  --json                    machine-readable JSON (schema_version 1.0)
  --lang ru|en              human output language
  --log-json                JSON logs on stderr
  --help                    help

Documentation: docs/integration-contract.md
`
