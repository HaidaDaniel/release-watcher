package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FormatEvent renders one event payload as a Telegram message in RU or EN.
func FormatEvent(lang string, raw json.RawMessage) string {
	var event eventPayload
	if err := json.Unmarshal(raw, &event); err != nil {
		return "Release Watcher: event notification"
	}
	title := messageTitle(lang, event.Work)
	labels := releaseLabels(event.Releases)
	var text string
	switch event.EventType {
	case "release.added":
		if len(event.Releases) > 0 {
			n := len(event.Releases)
			suffix := ""
			if labels != "" {
				suffix = ", " + labels
			}
			if lang == "en" {
				if n == 1 {
					text = fmt.Sprintf("%q: 1 new chapter%s.", title, suffix)
				} else {
					text = fmt.Sprintf("%q: %d new chapters%s.", title, n, suffix)
				}
			} else {
				if n == 1 {
					text = fmt.Sprintf("«%s»: появилась новая глава%s.", title, suffix)
				} else {
					text = fmt.Sprintf("«%s»: появились %d %s%s.", title, n, ruChapterWord(n), suffix)
				}
			}
		} else if event.CurrentCount != nil {
			delta := int64(0)
			if event.CountDelta != nil {
				delta = *event.CountDelta
			}
			if lang == "en" {
				text = fmt.Sprintf("%q: %d new releases.", title, delta)
			} else {
				text = fmt.Sprintf("«%s»: появились новые публикации (+%d).", title, delta)
			}
		} else {
			text = eventMessageFallback(lang, title)
		}
	case "release.updated":
		if lang == "en" {
			text = fmt.Sprintf("%q: updated chapter(s): %s.", title, labels)
		} else if len(event.Releases) == 1 {
			text = fmt.Sprintf("«%s»: обновилась глава: %s.", title, labels)
		} else {
			text = fmt.Sprintf("«%s»: обновились главы: %s.", title, labels)
		}
	case "release.removed":
		if lang == "en" {
			text = fmt.Sprintf("%q: removed chapter(s): %s.", title, labels)
		} else if len(event.Releases) == 1 {
			text = fmt.Sprintf("«%s»: удалена глава: %s.", title, labels)
		} else {
			text = fmt.Sprintf("«%s»: удалены главы: %s.", title, labels)
		}
	case "publication.metadata_changed":
		if lang == "en" {
			text = fmt.Sprintf("%q: publication metadata changed.", title)
		} else {
			text = fmt.Sprintf("«%s»: обновились метаданные публикации.", title)
		}
		if event.MetadataChanges != nil {
			titleLabel, statusLabel := "название", "статус"
			if lang == "en" {
				titleLabel, statusLabel = "title", "status"
			}
			if event.MetadataChanges.NewOriginalTitle != nil {
				text = strings.TrimSuffix(text, ".") + fmt.Sprintf(" %s: %s.", titleLabel, *event.MetadataChanges.NewOriginalTitle)
			}
			if event.MetadataChanges.NewStatus != nil {
				text = strings.TrimSuffix(text, ".") + fmt.Sprintf(" %s: %s.", statusLabel, statusMessage(lang, *event.MetadataChanges.NewStatus))
			}
		}
	case "publication.status_changed":
		oldStatus, newStatus := "unknown", "unknown"
		if event.MetadataChanges != nil {
			if event.MetadataChanges.OldStatus != nil {
				oldStatus = *event.MetadataChanges.OldStatus
			}
			if event.MetadataChanges.NewStatus != nil {
				newStatus = *event.MetadataChanges.NewStatus
			}
		}
		if lang == "en" {
			text = fmt.Sprintf("%q: publication status changed: %s → %s.", title, statusMessage(lang, oldStatus), statusMessage(lang, newStatus))
		} else {
			text = fmt.Sprintf("«%s»: статус публикации изменился: %s → %s.", title, statusMessage(lang, oldStatus), statusMessage(lang, newStatus))
		}
	case "release.count_changed":
		previous, current := int64(0), int64(0)
		if event.PreviousCount != nil {
			previous = *event.PreviousCount
		}
		if event.CurrentCount != nil {
			current = *event.CurrentCount
		}
		if lang == "en" {
			text = fmt.Sprintf("%q: release count changed: %d → %d.", title, previous, current)
		} else {
			text = fmt.Sprintf("«%s»: число публикаций изменилось: %d → %d.", title, previous, current)
		}
	default:
		text = eventMessageFallback(lang, title)
	}

	if event.URL != nil && *event.URL != "" {
		if lang == "en" {
			text += "\nOpen original: " + *event.URL
		} else {
			text += "\nОткрыть оригинал: " + *event.URL
		}
	}
	return truncateMessage(text)
}

func eventMessageFallback(lang, title string) string {
	if lang == "en" {
		return fmt.Sprintf("%q: publication changed.", title)
	}
	return fmt.Sprintf("«%s»: публикация изменилась.", title)
}

func messageTitle(lang string, work payloadWork) string {
	if lang == "en" {
		if work.TitleEN != nil && *work.TitleEN != "" {
			return *work.TitleEN
		}
	} else if work.TitleRU != nil && *work.TitleRU != "" {
		return *work.TitleRU
	}
	if work.OriginalTitle != "" {
		return work.OriginalTitle
	}
	return "Release Watcher"
}

func statusMessage(lang, status string) string {
	if lang == "en" {
		return status
	}
	switch status {
	case "ongoing":
		return "выходит"
	case "completed":
		return "завершено"
	case "unknown":
		return "неизвестно"
	default:
		return status
	}
}

func ruChapterWord(n int) string {
	if n%100 >= 11 && n%100 <= 14 {
		return "глав"
	}
	switch n % 10 {
	case 1:
		return "глава"
	case 2, 3, 4:
		return "главы"
	default:
		return "глав"
	}
}

func releaseLabels(releases []payloadRelease) string {
	if len(releases) == 0 {
		return ""
	}
	var numbers []string
	for _, release := range releases {
		if release.Number != nil && *release.Number != "" {
			numbers = append(numbers, *release.Number)
		}
	}
	if len(numbers) == len(releases) && len(numbers) > 0 {
		if len(numbers) > 1 && contiguous(numbers) {
			return numbers[0] + "–" + numbers[len(numbers)-1]
		}
		return strings.Join(numbers, ", ")
	}
	var labels []string
	for _, release := range releases {
		if release.Title != nil && *release.Title != "" {
			labels = append(labels, *release.Title)
		} else {
			labels = append(labels, release.ExternalID)
		}
	}
	if len(labels) > 4 {
		labels = append(labels[:4], "…")
	}
	return strings.Join(labels, ", ")
}

func contiguous(values []string) bool {
	first, err := strconv.Atoi(values[0])
	if err != nil {
		return false
	}
	for i, value := range values {
		number, err := strconv.Atoi(value)
		if err != nil || number != first+i {
			return false
		}
	}
	return true
}

func truncateMessage(text string) string {
	runes := []rune(text)
	if len(runes) <= 4096 {
		return text
	}
	return string(runes[:4093]) + "…"
}
