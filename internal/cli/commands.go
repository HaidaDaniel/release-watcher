package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/app"
	"github.com/HaidaDaniel/release-watcher/internal/config"
	"github.com/HaidaDaniel/release-watcher/internal/domain"
)

func (r *runner) add(ctx context.Context, args []string) int {
	values, positionals, err := parseCommandArgs(args, map[string]flagKind{
		"title-ru": valueFlag,
		"title-en": valueFlag,
		"type":     valueFlag,
		"interval": valueFlag,
	})
	if err != nil {
		return r.fail(err)
	}
	if len(positionals) != 1 {
		return r.fail(&commandError{code: "usage", message: "usage: release-watcher add <url> [--title-ru ...] [--title-en ...] [--type ...] [--interval 24h]", exit: ExitUsage})
	}
	var pubType *domain.PublicationType
	if raw := values["type"]; raw != "" {
		value := domain.PublicationType(strings.ToLower(raw))
		if !value.Valid() {
			return r.fail(&commandError{code: "invalid_argument", message: "invalid publication type", exit: ExitUsage})
		}
		pubType = &value
	}
	var interval *time.Duration
	if raw := values["interval"]; raw != "" {
		duration, parseErr := time.ParseDuration(raw)
		if parseErr != nil {
			return r.fail(&commandError{code: "invalid_argument", message: "invalid interval: " + parseErr.Error(), exit: ExitUsage})
		}
		interval = &duration
	}
	svc, closeStore, openErr := r.openService(ctx)
	if openErr != nil {
		return r.fail(openErr)
	}
	defer closeStore()
	result, appErr := svc.Add(ctx, app.AddInput{
		URL:      positionals[0],
		TitleRU:  optionalString(values["title-ru"]),
		TitleEN:  optionalString(values["title-en"]),
		Type:     pubType,
		Interval: interval,
	})
	if appErr != nil {
		return r.failApp(appErr)
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: result})
		return ExitOK
	}
	r.printAdd(result)
	return ExitOK
}

func (r *runner) list(ctx context.Context, args []string) int {
	_, positionals, err := parseCommandArgs(args, map[string]flagKind{})
	if err != nil {
		return r.fail(err)
	}
	if len(positionals) != 0 {
		return r.fail(&commandError{code: "usage", message: "list takes no arguments", exit: ExitUsage})
	}
	svc, closeStore, openErr := r.openService(ctx)
	if openErr != nil {
		return r.fail(openErr)
	}
	defer closeStore()
	result, appErr := svc.List(ctx)
	if appErr != nil {
		return r.failApp(appErr)
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: result})
		return ExitOK
	}
	r.printList(result)
	return ExitOK
}

func (r *runner) show(ctx context.Context, args []string) int {
	_, positionals, err := parseCommandArgs(args, map[string]flagKind{})
	if err != nil {
		return r.fail(err)
	}
	if len(positionals) != 1 {
		return r.fail(&commandError{code: "usage", message: "usage: release-watcher show <watch-id>", exit: ExitUsage})
	}
	svc, closeStore, openErr := r.openService(ctx)
	if openErr != nil {
		return r.fail(openErr)
	}
	defer closeStore()
	result, appErr := svc.Show(ctx, positionals[0])
	if appErr != nil {
		return r.failApp(appErr)
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: result})
		return ExitOK
	}
	r.printShow(result)
	return ExitOK
}

func (r *runner) scan(ctx context.Context, args []string) int {
	values, positionals, err := parseCommandArgs(args, map[string]flagKind{
		"due":       boolFlag,
		"all":       boolFlag,
		"id":        valueFlag,
		"no-notify": boolFlag,
	})
	if err != nil {
		return r.fail(err)
	}
	if len(positionals) != 0 {
		return r.fail(&commandError{code: "usage", message: "scan takes no positional arguments", exit: ExitUsage})
	}
	mode := "due"
	if flagBool(values, "all") {
		mode = "all"
	}
	if values["id"] != "" {
		mode = "id"
	}
	svc, closeStore, openErr := r.openService(ctx)
	if openErr != nil {
		return r.fail(openErr)
	}
	defer closeStore()
	result, appErr := svc.Scan(ctx, app.ScanOptions{Mode: mode, ID: values["id"], NoNotify: flagBool(values, "no-notify")})
	if appErr != nil {
		return r.failApp(appErr)
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: result})
	} else {
		r.printScan(result)
	}
	if result.Failed > 0 {
		return ExitPartial
	}
	return ExitOK
}

func (r *runner) events(ctx context.Context, args []string) int {
	values, positionals, err := parseCommandArgs(args, map[string]flagKind{
		"limit":       valueFlag,
		"since":       valueFlag,
		"publication": valueFlag,
		"watch":       valueFlag,
		"after":       valueFlag,
	})
	if err != nil {
		return r.fail(err)
	}
	if len(positionals) != 0 {
		return r.fail(&commandError{code: "usage", message: "events takes no positional arguments", exit: ExitUsage})
	}
	limit := 100
	if raw := values["limit"]; raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 1 || parsed > 1000 {
			return r.fail(&commandError{code: "invalid_argument", message: "--limit must be between 1 and 1000", exit: ExitUsage})
		}
		limit = parsed
	}
	var since *time.Time
	if raw := values["since"]; raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return r.fail(&commandError{code: "invalid_argument", message: "--since must be RFC3339", exit: ExitUsage})
		}
		since = &parsed
	}
	svc, closeStore, openErr := r.openService(ctx)
	if openErr != nil {
		return r.fail(openErr)
	}
	defer closeStore()
	result, appErr := svc.Events(ctx, app.EventQuery{
		PublicationID: values["publication"],
		WatchID:       values["watch"],
		Since:         since,
		After:         values["after"],
		Limit:         limit,
	})
	if appErr != nil {
		return r.failApp(appErr)
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: result})
		return ExitOK
	}
	r.printEvents(result)
	return ExitOK
}

func (r *runner) notify(ctx context.Context, args []string) int {
	_, positionals, err := parseCommandArgs(args, map[string]flagKind{})
	if err != nil {
		return r.fail(err)
	}
	if len(positionals) != 0 {
		return r.fail(&commandError{code: "usage", message: "notify takes no arguments", exit: ExitUsage})
	}
	svc, closeStore, openErr := r.openService(ctx)
	if openErr != nil {
		return r.fail(openErr)
	}
	defer closeStore()
	if !svc.NotificationsEnabled() {
		return r.fail(&commandError{code: "config_error", message: r.t("Telegram не настроен", "Telegram is not configured"), exit: ExitError})
	}
	counts, appErr := svc.Notify(ctx)
	if appErr != nil {
		return r.failApp(appErr)
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: counts})
		return ExitOK
	}
	fmt.Fprintf(r.stdout, "%s: %d, %s: %d, %s: %d\n",
		r.t("Ожидают", "Pending"), counts.Pending,
		r.t("Отправлено", "Sent"), counts.Sent,
		r.t("Ошибок", "Failed"), counts.Failed)
	return ExitOK
}

func (r *runner) pause(ctx context.Context, args []string) int {
	return r.watchMutation(ctx, args, "pause")
}

func (r *runner) resume(ctx context.Context, args []string) int {
	return r.watchMutation(ctx, args, "resume")
}

func (r *runner) watchMutation(ctx context.Context, args []string, action string) int {
	_, positionals, err := parseCommandArgs(args, map[string]flagKind{})
	if err != nil {
		return r.fail(err)
	}
	if len(positionals) != 1 {
		return r.fail(&commandError{code: "usage", message: fmt.Sprintf("usage: release-watcher %s <watch-id>", action), exit: ExitUsage})
	}
	svc, closeStore, openErr := r.openService(ctx)
	if openErr != nil {
		return r.fail(openErr)
	}
	defer closeStore()
	var view *app.WatchView
	var appErr *app.Error
	if action == "pause" {
		view, appErr = svc.Pause(ctx, positionals[0])
	} else {
		view, appErr = svc.Resume(ctx, positionals[0])
	}
	if appErr != nil {
		return r.failApp(appErr)
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: view})
		return ExitOK
	}
	r.printWatch(view)
	return ExitOK
}

func (r *runner) interval(ctx context.Context, args []string) int {
	values, positionals, err := parseCommandArgs(args, map[string]flagKind{"global": boolFlag})
	if err != nil {
		return r.fail(err)
	}
	global := flagBool(values, "global")
	var watchID string
	var rawDuration string
	if global {
		if len(positionals) != 1 {
			return r.fail(&commandError{code: "usage", message: "usage: release-watcher interval --global 24h", exit: ExitUsage})
		}
		rawDuration = positionals[0]
	} else {
		if len(positionals) != 2 {
			return r.fail(&commandError{code: "usage", message: "usage: release-watcher interval <watch-id> 24h", exit: ExitUsage})
		}
		watchID, rawDuration = positionals[0], positionals[1]
	}
	duration, parseErr := time.ParseDuration(rawDuration)
	if parseErr != nil {
		return r.fail(&commandError{code: "invalid_argument", message: "invalid interval: " + parseErr.Error(), exit: ExitUsage})
	}
	svc, closeStore, openErr := r.openService(ctx)
	if openErr != nil {
		return r.fail(openErr)
	}
	defer closeStore()
	if global {
		result, appErr := svc.SetGlobalInterval(ctx, duration)
		if appErr != nil {
			return r.failApp(appErr)
		}
		if r.json {
			r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: result})
			return ExitOK
		}
		fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Интервал по умолчанию", "Default interval"), duration)
		return ExitOK
	}
	view, appErr := svc.SetInterval(ctx, watchID, duration)
	if appErr != nil {
		return r.failApp(appErr)
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: view})
		return ExitOK
	}
	r.printWatch(view)
	return ExitOK
}

func (r *runner) remove(ctx context.Context, args []string) int {
	values, positionals, err := parseCommandArgs(args, map[string]flagKind{"yes": boolFlag})
	if err != nil {
		return r.fail(err)
	}
	if len(positionals) != 1 {
		return r.fail(&commandError{code: "usage", message: "usage: release-watcher remove <watch-id> --yes", exit: ExitUsage})
	}
	if !flagBool(values, "yes") {
		return r.fail(&commandError{code: "usage", message: "remove requires --yes", exit: ExitUsage})
	}
	svc, closeStore, openErr := r.openService(ctx)
	if openErr != nil {
		return r.fail(openErr)
	}
	defer closeStore()
	result, appErr := svc.Remove(ctx, positionals[0])
	if appErr != nil {
		return r.failApp(appErr)
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: result})
		return ExitOK
	}
	fmt.Fprintf(r.stdout, "%s %s\n", r.t("Удалено наблюдение", "Removed watch"), result.WatchID)
	return ExitOK
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (r *runner) backup(ctx context.Context, args []string) int {
	values, positionals, err := parseCommandArgs(args, map[string]flagKind{
		"out":   valueFlag,
		"force": boolFlag,
	})
	if err != nil {
		return r.fail(err)
	}
	if len(positionals) != 0 || values["out"] == "" {
		return r.fail(&commandError{code: "usage", message: "usage: release-watcher backup --out <file> [--force]", exit: ExitUsage})
	}
	svc, closeStore, openErr := r.openService(ctx)
	if openErr != nil {
		return r.fail(openErr)
	}
	defer closeStore()
	result, appErr := svc.Backup(ctx, values["out"], flagBool(values, "force"))
	if appErr != nil {
		return r.failApp(appErr)
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: result})
		return ExitOK
	}
	fmt.Fprintf(r.stdout, "%s: %s\n%s: %d\n%s: %d\n",
		r.t("Резервная копия", "Backup"), result.Path,
		r.t("Версия схемы", "Schema version"), result.SchemaVersion,
		r.t("Байт", "Bytes"), result.Bytes)
	return ExitOK
}

func (r *runner) restore(ctx context.Context, args []string) int {
	values, positionals, err := parseCommandArgs(args, map[string]flagKind{
		"in":    valueFlag,
		"force": boolFlag,
	})
	if err != nil {
		return r.fail(err)
	}
	if len(positionals) != 0 || values["in"] == "" {
		return r.fail(&commandError{code: "usage", message: "usage: release-watcher restore --in <file> --force", exit: ExitUsage})
	}
	cfg, cfgErr := config.Load(r.getenv)
	if cfgErr != nil {
		return r.fail(&commandError{code: "config_error", message: cfgErr.Error(), exit: ExitError})
	}
	result, appErr := app.Restore(ctx, cfg, values["in"], flagBool(values, "force"))
	if appErr != nil {
		return r.failApp(appErr)
	}
	if r.json {
		r.writeJSON(envelope{SchemaVersion: SchemaVersion, OK: true, Data: result})
		return ExitOK
	}
	fmt.Fprintf(r.stdout, "%s: %s\n%s: %d\n%s: %d\n",
		r.t("Восстановлено", "Restored"), result.Path,
		r.t("Версия схемы", "Schema version"), result.SchemaVersion,
		r.t("Байт", "Bytes"), result.Bytes)
	return ExitOK
}
