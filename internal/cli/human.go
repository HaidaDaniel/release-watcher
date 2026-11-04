package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/app"
)

func (r *runner) printAdd(result *app.AddResult) {
	title := displayWatch(result.WatchView, r.lang)
	switch {
	case result.Created:
		fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Добавлено", "Added"), title)
	case result.Reason == "restored":
		fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Наблюдение восстановлено", "Watching restored"), title)
	default:
		fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Уже отслеживается", "Already watched"), title)
	}
	fmt.Fprintf(r.stdout, "watch_id: %s\n", result.WatchID)
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Источник", "Source"), result.SourceType)
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Статус", "Status"), statusText(r.lang, result.Status, result.StatusKnown))
	fmt.Fprintf(r.stdout, "%s: %d\n", r.t("Baseline релизов", "Baseline releases"), result.BaselineReleases)
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Следующая проверка", "Next check"), formatTimeValue(result.NextCheckAt))
	if len(result.Warnings) > 0 {
		fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Предупреждения", "Warnings"), strings.Join(result.Warnings, "; "))
	}
}

func (r *runner) printList(result *app.ListResult) {
	if len(result.Watches) == 0 {
		fmt.Fprintln(r.stdout, r.t("Наблюдений нет.", "No watches."))
		return
	}
	tw := tabwriter.NewWriter(r.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, r.t("ID\tНАЗВАНИЕ\tИСТОЧНИК\tСТАТУС\tСЛЕДУЮЩАЯ\tРЕЛИЗОВ", "ID\tTITLE\tSOURCE\tSTATUS\tNEXT\tRELEASES"))
	for _, watch := range result.Watches {
		releases := "-"
		if watch.ReleaseCount != nil {
			releases = fmt.Sprintf("%d", *watch.ReleaseCount)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			watch.WatchID, displayWatch(watch, r.lang), watch.SourceType,
			statusText(r.lang, watch.Status, watch.StatusKnown),
			formatTimeValue(watch.NextCheckAt), releases)
	}
	_ = tw.Flush()
}

func (r *runner) printShow(result *app.ShowResult) {
	watch := result.Watch
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Наблюдение", "Watch"), watch.WatchID)
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Название", "Title"), displayWatch(watch, r.lang))
	fmt.Fprintf(r.stdout, "%s: %s / %s\n", r.t("Публикация", "Publication"), watch.PublicationType, watch.OriginalURL)
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Источник", "Source"), watch.SourceType)
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Статус", "Status"), statusText(r.lang, watch.Status, watch.StatusKnown))
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Интервал", "Interval"), formatInterval(watch.IntervalSeconds))
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Включено", "Enabled"), yesNo(r.lang, watch.Enabled))
	if watch.RemovedAt != nil {
		fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Наблюдение остановлено", "Watching stopped"), formatTimePtr(watch.RemovedAt))
	}
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Последняя проверка", "Last check"), formatTimePtr(watch.LastCheckedAt))
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Следующая проверка", "Next check"), formatTimeValue(watch.NextCheckAt))
	if watch.LastErrorCategory != nil {
		fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Последняя ошибка", "Last error"), *watch.LastErrorCategory)
	}
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Режим", "Mode"), watch.Mode)
	if watch.ReleaseCount != nil {
		fmt.Fprintf(r.stdout, "%s: %d\n", r.t("Релизов", "Releases"), *watch.ReleaseCount)
	}
	if len(result.Releases) > 0 {
		fmt.Fprintln(r.stdout, r.t("Последние релизы:", "Recent releases:"))
		limit := len(result.Releases)
		if limit > 20 {
			limit = 20
		}
		for _, release := range result.Releases[:limit] {
			title := "-"
			if release.Title != nil {
				title = *release.Title
			}
			number := ""
			if release.Number != nil {
				number = *release.Number + " "
			}
			url := ""
			if release.URL != nil {
				url = " " + *release.URL
			}
			baseline := ""
			if release.IsBaseline {
				baseline = " [baseline]"
			}
			removed := ""
			if release.RemovedAt != nil {
				removed = " [removed]"
			}
			fmt.Fprintf(r.stdout, "  - %s%s (%s)%s%s%s\n", number, title, formatTimePtr(release.PublishedAt), url, baseline, removed)
		}
	}
	if len(result.Events) > 0 {
		fmt.Fprintln(r.stdout, r.t("Последние события:", "Recent events:"))
		for _, raw := range result.Events {
			r.printEventPayload(raw)
		}
	}
	if len(result.Scans) > 0 {
		fmt.Fprintln(r.stdout, r.t("Последние проверки:", "Recent scans:"))
		for _, scan := range result.Scans {
			fmt.Fprintf(r.stdout, "  - %s %s events=%d\n", formatTimeValue(scan.FinishedAt), scan.Result, scan.EventsCreated)
		}
	}
}

type humanEvent struct {
	EventID               string    `json:"event_id"`
	EventType             string    `json:"event_type"`
	DetectedAt            time.Time `json:"detected_at"`
	ExternalWorkReference string    `json:"external_work_reference"`
	ReleaseIDs            []string  `json:"release_ids"`
	CurrentCount          *int64    `json:"current_count"`
	CountDelta            *int64    `json:"count_delta"`
	Work                  struct {
		OriginalTitle string  `json:"original_title"`
		TitleRU       *string `json:"title_ru"`
		TitleEN       *string `json:"title_en"`
	} `json:"work"`
	MetadataChanges *struct {
		OldOriginalTitle *string `json:"old_original_title"`
		NewOriginalTitle *string `json:"new_original_title"`
		OldStatus        *string `json:"old_status"`
		NewStatus        *string `json:"new_status"`
	} `json:"metadata_changes"`
}

func (r *runner) printEvents(result *app.EventsResult) {
	if len(result.Events) == 0 {
		fmt.Fprintln(r.stdout, r.t("Событий нет.", "No events."))
		return
	}
	for _, raw := range result.Events {
		r.printEventPayload(raw)
	}
	if result.NextCursor != "" {
		fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Курсор", "Cursor"), result.NextCursor)
	}
}

func (r *runner) printEventPayload(raw json.RawMessage) {
	var event humanEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		fmt.Fprintf(r.stdout, "  - %s\n", string(raw))
		return
	}
	title := event.Work.OriginalTitle
	if r.lang == "en" {
		if event.Work.TitleEN != nil && *event.Work.TitleEN != "" {
			title = *event.Work.TitleEN
		}
	} else if event.Work.TitleRU != nil && *event.Work.TitleRU != "" {
		title = *event.Work.TitleRU
	}
	detail := ""
	if len(event.ReleaseIDs) > 0 {
		detail = " " + strings.Join(event.ReleaseIDs, ",")
	} else if event.CurrentCount != nil {
		detail = fmt.Sprintf(" count=%d", *event.CurrentCount)
	}
	if event.MetadataChanges != nil {
		if event.MetadataChanges.NewStatus != nil {
			detail += " status=" + *event.MetadataChanges.NewStatus
		}
		if event.MetadataChanges.NewOriginalTitle != nil {
			detail += " title=" + *event.MetadataChanges.NewOriginalTitle
		}
	}
	fmt.Fprintf(r.stdout, "  - %s %s %s%s\n", event.DetectedAt.UTC().Format(time.RFC3339), event.EventType, title, detail)
}

func (r *runner) printScan(result *app.ScanResult) {
	fmt.Fprintf(r.stdout, "%s: %d, %s: %d, %s: %d, %s: %d\n",
		r.t("Проверено", "Scanned"), result.Scanned,
		r.t("успешно", "succeeded"), result.Succeeded,
		r.t("ошибок", "failed"), result.Failed,
		r.t("событий", "events"), result.EventsCreated)
	for _, watch := range result.Results {
		if watch.Error != nil {
			fmt.Fprintf(r.stdout, "  - %s: %s (%s: %s)\n", watch.WatchID, watch.Outcome, watch.Error.Code, watch.Error.Message)
		} else {
			mode := ""
			if watch.Mode != nil {
				mode = *watch.Mode
			}
			fmt.Fprintf(r.stdout, "  - %s: %s, events=%d, mode=%s\n", watch.WatchID, watch.Outcome, watch.EventsCreated, mode)
		}
	}
}

func (r *runner) printWatch(watch *app.WatchView) {
	if watch == nil {
		return
	}
	fmt.Fprintf(r.stdout, "watch_id: %s\n", watch.WatchID)
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Название", "Title"), displayWatch(*watch, r.lang))
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Включено", "Enabled"), yesNo(r.lang, watch.Enabled))
	if watch.RemovedAt != nil {
		fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Наблюдение остановлено", "Watching stopped"), formatTimePtr(watch.RemovedAt))
	}
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Интервал", "Interval"), formatInterval(watch.IntervalSeconds))
	fmt.Fprintf(r.stdout, "%s: %s\n", r.t("Следующая проверка", "Next check"), formatTimeValue(watch.NextCheckAt))
}

func displayWatch(watch app.WatchView, lang string) string {
	if lang == "en" {
		if watch.TitleEN != nil && *watch.TitleEN != "" {
			return *watch.TitleEN
		}
	} else if watch.TitleRU != nil && *watch.TitleRU != "" {
		return *watch.TitleRU
	}
	if watch.OriginalTitle != "" {
		return watch.OriginalTitle
	}
	return watch.OriginalURL
}

func statusText(lang, status string, known bool) string {
	if !known || status == "unknown" {
		if lang == "en" {
			return "unknown"
		}
		return "неизвестно"
	}
	if lang == "en" {
		return status
	}
	switch status {
	case "ongoing":
		return "выходит"
	case "completed":
		return "завершено"
	default:
		return status
	}
}

func yesNo(lang string, value bool) string {
	if lang == "en" {
		if value {
			return "yes"
		}
		return "no"
	}
	if value {
		return "да"
	}
	return "нет"
}

func formatInterval(seconds int64) string {
	return (time.Duration(seconds) * time.Second).String()
}

func formatTimeValue(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04 UTC")
}

func formatTimePtr(value *time.Time) string {
	if value == nil {
		return "—"
	}
	return formatTimeValue(*value)
}
