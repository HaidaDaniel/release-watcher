package source_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/source"
)

func TestNaroFetchCatalog(t *testing.T) {
	api := fixture(t, "naro_api.json")
	catalog := fixture(t, "naro_catalog.html")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/novelapi/api/":
			if got := r.URL.Query().Get("ncode"); got != "N2166FE" {
				t.Errorf("ncode = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(api)
		case strings.HasPrefix(r.URL.Path, "/n2166fe/"):
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write(catalog)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter := source.NewNaro(testClient())
	adapter.APIBase = server.URL + "/novelapi/api/"
	adapter.CatalogBase = server.URL + "/"
	got, err := adapter.Fetch(context.Background(), "https://ncode.syosetu.com/n2166fe/")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got.ExternalID != "N2166FE" || got.ExternalWorkReference != "ncode:N2166FE" {
		t.Fatalf("identity = %q / %q", got.ExternalID, got.ExternalWorkReference)
	}
	if got.Metadata.OriginalTitle != "夢見る男子は現実主義者" {
		t.Fatalf("title = %q", got.Metadata.OriginalTitle)
	}
	if got.Metadata.Status != domain.StatusCompleted || got.Metadata.StatusSource != "naro:end" {
		t.Fatalf("status = %q source=%q", got.Metadata.Status, got.Metadata.StatusSource)
	}
	if got.Metadata.LastPublishedAt == nil || !got.Metadata.LastPublishedAt.Equal(time.Date(2023, 8, 1, 13, 25, 9, 0, time.UTC)) {
		t.Fatalf("last published = %v", got.Metadata.LastPublishedAt)
	}
	if got.Count != 3 || len(got.Releases) != 3 || !got.Complete || got.Mode != domain.ModeCatalog {
		t.Fatalf("count=%d releases=%d complete=%v mode=%q", got.Count, len(got.Releases), got.Complete, got.Mode)
	}
	if got.Releases[0].ExternalID != "N2166FE/1" || got.Releases[2].ExternalID != "N2166FE/3" {
		t.Fatalf("ids = %q ... %q", got.Releases[0].ExternalID, got.Releases[2].ExternalID)
	}
	if got.Releases[0].Title == nil || *got.Releases[0].Title != "星屑の後に" {
		t.Fatalf("title = %v", got.Releases[0].Title)
	}
	if got.Releases[0].SourceUpdatedAt == nil {
		t.Fatal("revision time not parsed")
	}
	if got.Releases[2].URL == nil || !strings.HasPrefix(*got.Releases[2].URL, server.URL) {
		t.Fatalf("url = %v", got.Releases[2].URL)
	}
	fields := strings.Join(got.FieldsAvailable, ",")
	for _, want := range []string{"status", "release_count", "release_catalog", "release_titles", "release_times"} {
		if !strings.Contains(fields, want) {
			t.Fatalf("fields %q missing %q", fields, want)
		}
	}
}

func TestNaroStatusUnknownWhenEndMissing(t *testing.T) {
	api := strings.Replace(string(fixture(t, "naro_api.json")), `"end":1`, `"end":null`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/novelapi/api/" {
			_, _ = w.Write([]byte(api))
			return
		}
		_, _ = w.Write(fixture(t, "naro_catalog.html"))
	}))
	defer server.Close()

	adapter := source.NewNaro(testClient())
	adapter.APIBase = server.URL + "/novelapi/api/"
	adapter.CatalogBase = server.URL + "/"
	got, err := adapter.Fetch(context.Background(), "https://ncode.syosetu.com/n2166fe/")
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.Status != domain.StatusUnknown || got.Metadata.StatusSource != "" {
		t.Fatalf("status = %q source=%q", got.Metadata.Status, got.Metadata.StatusSource)
	}
	if strings.Contains(strings.Join(got.FieldsAvailable, ","), "status") {
		t.Fatal("status claimed available without end field")
	}
}

func TestNaroPagination(t *testing.T) {
	api := strings.Replace(string(fixture(t, "naro_api.json")), `"general_all_no":3`, `"general_all_no":101`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/novelapi/api/" {
			_, _ = w.Write([]byte(api))
			return
		}
		switch r.URL.Query().Get("p") {
		case "1":
			_, _ = w.Write([]byte(catalogHTML(1, 100)))
		case "2":
			_, _ = w.Write([]byte(catalogHTML(101, 1)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter := source.NewNaro(testClient())
	adapter.APIBase = server.URL + "/novelapi/api/"
	adapter.CatalogBase = server.URL + "/"
	got, err := adapter.Fetch(context.Background(), "https://ncode.syosetu.com/n2166fe/")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Releases) != 101 || !got.Complete {
		t.Fatalf("releases=%d complete=%v", len(got.Releases), got.Complete)
	}
	if got.Releases[100].ExternalID != "N2166FE/101" {
		t.Fatalf("last id = %q", got.Releases[100].ExternalID)
	}
}

func TestNaroPartialCatalogIsNotComplete(t *testing.T) {
	api := strings.Replace(string(fixture(t, "naro_api.json")), `"general_all_no":3`, `"general_all_no":101`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/novelapi/api/" {
			_, _ = w.Write([]byte(api))
			return
		}
		if r.URL.Query().Get("p") == "1" {
			_, _ = w.Write([]byte(catalogHTML(1, 100)))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	adapter := source.NewNaro(testClient())
	adapter.APIBase = server.URL + "/novelapi/api/"
	adapter.CatalogBase = server.URL + "/"
	got, err := adapter.Fetch(context.Background(), "https://ncode.syosetu.com/n2166fe/")
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete {
		t.Fatal("partial catalog marked complete")
	}
	if got.Mode != domain.ModeCatalog || len(got.Releases) != 100 {
		t.Fatalf("mode=%q releases=%d", got.Mode, len(got.Releases))
	}
	if len(got.Warnings) == 0 {
		t.Fatal("partial catalog has no warning")
	}
}

func TestNaroCatalogFailureFallsBackToCountOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/novelapi/api/" {
			_, _ = w.Write(fixture(t, "naro_api.json"))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	adapter := source.NewNaro(testClient())
	adapter.APIBase = server.URL + "/novelapi/api/"
	adapter.CatalogBase = server.URL + "/"
	got, err := adapter.Fetch(context.Background(), "https://ncode.syosetu.com/n2166fe/")
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != domain.ModeCountOnly || got.Count != 3 || len(got.Releases) != 0 {
		t.Fatalf("mode=%q count=%d releases=%d", got.Mode, got.Count, len(got.Releases))
	}
	if got.Complete {
		t.Fatal("count-only fallback marked complete")
	}
}

func TestNaroInvalidURL(t *testing.T) {
	adapter := source.NewNaro(testClient())
	if _, err := adapter.Fetch(context.Background(), "https://example.test/not-naro"); err == nil {
		t.Fatal("accepted a non-Naro URL")
	}
}

func TestNaroChapterWithoutUpdateBlockIsKept(t *testing.T) {
	api := fixture(t, "naro_api.json")
	htmlBody := `<div class="p-eplist">
<div class="p-eplist__sublist"><a href="/n2166fe/1/" class="p-eplist__subtitle">One</a><div class="p-eplist__update">2023/08/01 22:25</div></div>
<div class="p-eplist__sublist"><a href="/n2166fe/2/" class="p-eplist__subtitle">Two</a></div>
<div class="p-eplist__sublist"><a href="/n2166fe/3/" class="p-eplist__subtitle">Three</a><div class="p-eplist__update">2023/08/01 22:26</div></div>
</div>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/novelapi/api/" {
			_, _ = w.Write(api)
			return
		}
		_, _ = w.Write([]byte(htmlBody))
	}))
	defer server.Close()

	adapter := source.NewNaro(testClient())
	adapter.APIBase = server.URL + "/novelapi/api/"
	adapter.CatalogBase = server.URL + "/"
	got, err := adapter.Fetch(context.Background(), "https://ncode.syosetu.com/n2166fe/")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Releases) != 3 || !got.Complete {
		t.Fatalf("releases=%d complete=%v", len(got.Releases), got.Complete)
	}
	if got.Releases[1].ExternalID != "N2166FE/2" || got.Releases[1].PublishedAt != nil {
		t.Fatalf("middle release = %+v", got.Releases[1])
	}
}

func TestNaroCatalogExhaustionDetectsUndercount(t *testing.T) {
	api := strings.Replace(string(fixture(t, "naro_api.json")), `"general_all_no":3`, `"general_all_no":100`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/novelapi/api/" {
			_, _ = w.Write([]byte(api))
			return
		}
		switch r.URL.Query().Get("p") {
		case "1":
			_, _ = w.Write([]byte(catalogHTML(1, 100)))
		case "2":
			_, _ = w.Write([]byte(catalogHTML(101, 1)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter := source.NewNaro(testClient())
	adapter.APIBase = server.URL + "/novelapi/api/"
	adapter.CatalogBase = server.URL + "/"
	got, err := adapter.Fetch(context.Background(), "https://ncode.syosetu.com/n2166fe/")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || len(got.Releases) != 101 || got.Count != 101 {
		t.Fatalf("complete=%v releases=%d count=%d", got.Complete, len(got.Releases), got.Count)
	}
}

func TestNaroMissingCountDoesNotUseAllCount(t *testing.T) {
	api := strings.Replace(string(fixture(t, "naro_api.json")), `"general_all_no":3,`, ``, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/novelapi/api/" {
			_, _ = w.Write([]byte(api))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	adapter := source.NewNaro(testClient())
	adapter.APIBase = server.URL + "/novelapi/api/"
	adapter.CatalogBase = server.URL + "/"
	got, err := adapter.Fetch(context.Background(), "https://ncode.syosetu.com/n2166fe/")
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != domain.ModeCountOnly || got.Count != 0 {
		t.Fatalf("mode=%q count=%d, want count_only/0", got.Mode, got.Count)
	}
}
