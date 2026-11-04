package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/source/httpx"
)

// FeedAdapter parses RSS 2.0, RSS 1.0 (RDF) and Atom feeds.
type FeedAdapter struct {
	Client *httpx.Client
}

func NewFeed(client *httpx.Client) *FeedAdapter {
	return &FeedAdapter{Client: client}
}

func (a *FeedAdapter) Name() string { return "rss" }

type feedRoot struct {
	XMLName xml.Name
	Channel *rssChannel `xml:"channel"`
	Title   string      `xml:"title"`
	Items   []rssItem   `xml:"item"`
	Entries []atomEntry `xml:"entry"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
	Date    string `xml:"date"`
	Updated string `xml:"updated"`
}

type atomEntry struct {
	ID        string     `xml:"id"`
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func (a *FeedAdapter) Fetch(ctx context.Context, rawURL string) (*Snapshot, error) {
	u, err := httpx.ValidateURL(rawURL)
	if err != nil {
		return nil, mapHTTPError(err)
	}
	u.Fragment = ""
	canonical := u.String()
	resp, err := a.Client.Get(ctx, canonical)
	if err != nil {
		return nil, mapHTTPError(err)
	}
	var root feedRoot
	if err := xml.Unmarshal(resp.Body, &root); err != nil {
		return nil, &Error{Code: CodeUnsupportedSource, Message: "URL is not a valid RSS/Atom feed", Cause: err}
	}
	now := time.Now().UTC()
	snapshot := &Snapshot{
		ExternalID:            canonical,
		ExternalWorkReference: "feed:" + canonical,
		OriginalURL:           canonical,
		Metadata: Metadata{
			PublicationType: domain.PublicationUnknown,
			Status:          domain.StatusUnknown,
		},
		Mode:       domain.ModeCatalog,
		Complete:   false,
		ObservedAt: now,
	}
	var releases []Release
	switch {
	case root.Channel != nil:
		snapshot.SourceKind = domain.SourceRSS
		snapshot.Metadata.OriginalTitle = strings.TrimSpace(root.Channel.Title)
		items := append(append([]rssItem{}, root.Channel.Items...), root.Items...)
		for i, item := range items {
			releases = append(releases, rssRelease(item, i))
		}
	case root.XMLName.Local == "feed" || len(root.Entries) > 0:
		snapshot.SourceKind = domain.SourceAtom
		snapshot.Metadata.OriginalTitle = strings.TrimSpace(root.Title)
		for i, entry := range root.Entries {
			releases = append(releases, atomRelease(entry, i))
		}
	default:
		return nil, &Error{Code: CodeUnsupportedSource, Message: "URL is not an RSS or Atom feed"}
	}
	for i := range releases {
		releases[i].URL = resolveFeedURL(u, releases[i].URL)
		releases[i].ContentHash = HashRelease(releases[i])
	}
	snapshot.Releases = releases
	snapshot.Count = int64(len(releases))
	snapshot.Metadata.LastPublishedAt = latestReleaseTime(releases)
	if len(releases) == 0 {
		snapshot.Warnings = append(snapshot.Warnings, "feed contains no entries")
	}
	fields := []string{"original_url", "title"}
	if len(releases) > 0 {
		fields = append(fields, "release_catalog", "release_titles")
		if snapshot.Metadata.LastPublishedAt != nil {
			fields = append(fields, "release_times")
		}
	}
	snapshot.FieldsAvailable = fields
	return snapshot, nil
}

func rssRelease(item rssItem, index int) Release {
	id := firstNonEmpty(item.GUID, item.Link)
	published := parseFeedTime(firstNonEmpty(item.PubDate, item.Date, item.Updated))
	if id == "" {
		id = fallbackEntryID(item.Title, published, index)
	}
	var link *string
	if item.Link != "" {
		link = stringPtr(strings.TrimSpace(item.Link))
	} else if strings.HasPrefix(item.GUID, "http") {
		link = stringPtr(strings.TrimSpace(item.GUID))
	}
	return Release{
		ExternalID:  strings.TrimSpace(id),
		Kind:        domain.ReleaseEntry,
		Title:       stringPtr(strings.TrimSpace(item.Title)),
		URL:         link,
		PublishedAt: published,
	}
}

func atomRelease(entry atomEntry, index int) Release {
	link := alternateLink(entry.Links)
	id := firstNonEmpty(entry.ID, link)
	published := parseFeedTime(firstNonEmpty(entry.Published, entry.Updated))
	if id == "" {
		id = fallbackEntryID(entry.Title, published, index)
	}
	return Release{
		ExternalID:  strings.TrimSpace(id),
		Kind:        domain.ReleaseEntry,
		Title:       stringPtr(strings.TrimSpace(entry.Title)),
		URL:         stringPtr(strings.TrimSpace(link)),
		PublishedAt: published,
	}
}

func alternateLink(links []atomLink) string {
	for _, link := range links {
		if link.Rel == "" || link.Rel == "alternate" {
			return link.Href
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func fallbackEntryID(title string, published *time.Time, index int) string {
	base := strings.TrimSpace(title)
	if published != nil {
		base += "|" + published.UTC().Format(time.RFC3339)
	}
	base += "|" + fmt.Sprintf("%d", index)
	sum := sha256.Sum256([]byte(base))
	return "entry:" + hex.EncodeToString(sum[:])
}

func resolveFeedURL(base *url.URL, raw *string) *string {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	ref, err := url.Parse(strings.TrimSpace(*raw))
	if err != nil {
		return nil
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return nil
	}
	value := resolved.String()
	return &value
}

func parseFeedTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if t, err := http.ParseTime(value); err == nil {
		utc := t.UTC()
		return &utc
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC850,
		time.ANSIC,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			utc := t.UTC()
			return &utc
		}
	}
	return nil
}
