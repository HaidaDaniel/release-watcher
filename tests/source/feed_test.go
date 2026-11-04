package source_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/source"
)

func TestRSSFeed(t *testing.T) {
	payload := fixture(t, "rss.xml")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	adapter := source.NewFeed(testClient())
	got, err := adapter.Fetch(context.Background(), server.URL+"/feed.xml")
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceKind != domain.SourceRSS {
		t.Fatalf("kind = %q", got.SourceKind)
	}
	if got.Metadata.OriginalTitle != "Example Web Novel" {
		t.Fatalf("title = %q", got.Metadata.OriginalTitle)
	}
	if got.Count != 2 || len(got.Releases) != 2 || got.Complete {
		t.Fatalf("count=%d releases=%d complete=%v", got.Count, len(got.Releases), got.Complete)
	}
	if got.Releases[0].ExternalID != "example-chapter-10" {
		t.Fatalf("guid id = %q", got.Releases[0].ExternalID)
	}
	if got.Releases[1].ExternalID != "https://example.test/novel/11" {
		t.Fatalf("link id = %q", got.Releases[1].ExternalID)
	}
	if got.Releases[0].PublishedAt == nil || got.Releases[1].PublishedAt == nil {
		t.Fatal("published time not parsed")
	}
	if got.Metadata.LastPublishedAt == nil {
		t.Fatal("last published time missing")
	}
}

func TestAtomFeed(t *testing.T) {
	payload := fixture(t, "atom.xml")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	adapter := source.NewFeed(testClient())
	got, err := adapter.Fetch(context.Background(), server.URL+"/atom.xml")
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceKind != domain.SourceAtom {
		t.Fatalf("kind = %q", got.SourceKind)
	}
	if got.Metadata.OriginalTitle != "Example Atom Novel" {
		t.Fatalf("title = %q", got.Metadata.OriginalTitle)
	}
	if len(got.Releases) != 2 || got.Releases[0].ExternalID != "tag:example.test,2026:chapter-20" {
		t.Fatalf("releases = %+v", got.Releases)
	}
	if got.Releases[1].URL == nil || *got.Releases[1].URL != "https://example.test/atom/21" {
		t.Fatalf("url = %v", got.Releases[1].URL)
	}
}

func TestFeedRejectsHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>not a feed</body></html>`))
	}))
	defer server.Close()

	adapter := source.NewFeed(testClient())
	_, err := adapter.Fetch(context.Background(), server.URL+"/page")
	if err == nil {
		t.Fatal("HTML accepted as a feed")
	}
	srcErr, ok := err.(*source.Error)
	if !ok || srcErr.Code != source.CodeUnsupportedSource {
		t.Fatalf("err = %#v", err)
	}
}

func TestRegistryFetchByKindRSS(t *testing.T) {
	payload := fixture(t, "rss.xml")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	reg := source.NewRegistry(testClient())
	got, err := reg.FetchByKind(context.Background(), domain.SourceRSS, server.URL+"/feed.xml")
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceKind != domain.SourceRSS || len(got.Releases) != 2 {
		t.Fatalf("snapshot = %+v", got)
	}
}
