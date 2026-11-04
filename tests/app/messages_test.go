package app_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/HaidaDaniel/release-watcher/internal/app"
)

func formatEvent(t *testing.T, payload map[string]any, lang string) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return app.FormatEvent(lang, raw)
}

func basePayload(eventType string) map[string]any {
	return map[string]any{
		"schema_version":   "1.0",
		"event_id":         "evt_1",
		"event_type":       eventType,
		"work":             map[string]any{"original_title": "作品", "title_ru": nil, "title_en": nil, "publication_type": "wn"},
		"publication":      map[string]any{"source_type": "naro", "original_url": "https://example.test/work", "status": "ongoing", "status_known": true},
		"release_ids":      []string{},
		"releases":         []any{},
		"previous_count":   nil,
		"current_count":    nil,
		"count_delta":      nil,
		"published_at":     nil,
		"detected_at":      "2026-11-01T12:00:00Z",
		"url":              "https://example.test/work",
		"metadata_changes": nil,
		"dedupe_key":       "key",
	}
}

func TestFormatAddedManyChaptersRussianGrammar(t *testing.T) {
	payload := basePayload("release.added")
	releases := make([]map[string]any, 0, 21)
	ids := make([]string, 0, 21)
	for i := 1; i <= 21; i++ {
		number := json.Number(jsonInt(i))
		ids = append(ids, "N1/"+number.String())
		releases = append(releases, map[string]any{
			"external_id": "N1/" + number.String(), "kind": "chapter", "number": number.String(),
			"title": "Chapter", "url": nil, "published_at": nil,
		})
	}
	payload["releases"] = releases
	payload["release_ids"] = ids
	msg := formatEvent(t, payload, "ru")
	if !strings.Contains(msg, "появилась 21 глава") || !strings.Contains(msg, "1–21") {
		t.Fatalf("message = %q", msg)
	}
}

func TestFormatMetadataAndStatusMessages(t *testing.T) {
	payload := basePayload("publication.metadata_changed")
	payload["metadata_changes"] = map[string]any{
		"old_original_title": "Old", "new_original_title": "New",
		"old_status": "ongoing", "new_status": "completed",
	}
	msg := formatEvent(t, payload, "ru")
	for _, want := range []string{"обновились метаданные публикации.", "Название: New", "Статус: завершено"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q missing %q", msg, want)
		}
	}

	status := basePayload("publication.status_changed")
	status["metadata_changes"] = map[string]any{"old_status": "ongoing", "new_status": "completed"}
	msg = formatEvent(t, status, "ru")
	if !strings.Contains(msg, "статус публикации изменился: выходит → завершено") {
		t.Fatalf("status message = %q", msg)
	}

	status["metadata_changes"] = map[string]any{"old_status": "ongoing", "new_status": "completed"}
	msg = formatEvent(t, status, "en")
	if !strings.Contains(msg, "publication status changed: ongoing → completed") {
		t.Fatalf("status en message = %q", msg)
	}
}

func TestFormatUpdatedRemovedAndCountOnly(t *testing.T) {
	updated := basePayload("release.updated")
	updated["releases"] = []any{map[string]any{"external_id": "N1/5", "kind": "chapter", "number": "5", "title": "Five"}}
	updated["release_ids"] = []string{"N1/5"}
	if msg := formatEvent(t, updated, "ru"); !strings.Contains(msg, "обновилась глава: 5") {
		t.Fatalf("updated = %q", msg)
	}

	removed := basePayload("release.removed")
	removed["releases"] = []any{map[string]any{"external_id": "N1/5", "kind": "chapter", "number": "5", "title": "Five"}}
	removed["release_ids"] = []string{"N1/5"}
	if msg := formatEvent(t, removed, "ru"); !strings.Contains(msg, "удалена глава: 5") {
		t.Fatalf("removed = %q", msg)
	}

	countOnly := basePayload("release.added")
	countOnly["previous_count"] = 3
	countOnly["current_count"] = 5
	countOnly["count_delta"] = 2
	if msg := formatEvent(t, countOnly, "ru"); !strings.Contains(msg, "появились новые публикации (+2)") {
		t.Fatalf("count-only = %q", msg)
	}

	countChanged := basePayload("release.count_changed")
	countChanged["previous_count"] = 5
	countChanged["current_count"] = 3
	countChanged["count_delta"] = -2
	if msg := formatEvent(t, countChanged, "ru"); !strings.Contains(msg, "число публикаций изменилось: 5 → 3") {
		t.Fatalf("count-changed = %q", msg)
	}
}

func TestFormatEnglishSingleChapterAndTruncation(t *testing.T) {
	payload := basePayload("release.added")
	payload["releases"] = []any{map[string]any{"external_id": "N1/1", "kind": "chapter", "number": "1", "title": "One"}}
	payload["release_ids"] = []string{"N1/1"}
	msg := formatEvent(t, payload, "en")
	if !strings.Contains(msg, "1 new chapter") || !strings.Contains(msg, "Open original") {
		t.Fatalf("en message = %q", msg)
	}

	long := basePayload("publication.metadata_changed")
	long["metadata_changes"] = map[string]any{"new_original_title": strings.Repeat("x", 5000)}
	msg = formatEvent(t, long, "ru")
	if len([]rune(msg)) > 4096 {
		t.Fatalf("message too long: %d", len([]rune(msg)))
	}
}

func jsonInt(value int) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
