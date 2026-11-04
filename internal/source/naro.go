package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/source/httpx"
)

const (
	defaultNaroAPIBase     = "https://api.syosetu.com/novelapi/api/"
	defaultNaroCatalogBase = "https://ncode.syosetu.com/"
	naroPageSize           = 100
	naroMaxPages           = 100
)

var (
	naroCodePattern = regexp.MustCompile(`^[Nn][0-9A-Za-z]{3,20}$`)
	naroDatePattern = regexp.MustCompile(`(\d{4}/\d{2}/\d{2} \d{2}:\d{2})`)
	jst             = time.FixedZone("JST", 9*60*60)
)

// NaroAdapter reads the official Narō novel API and the public catalog pages.
type NaroAdapter struct {
	Client      *httpx.Client
	APIBase     string
	CatalogBase string
}

func NewNaro(client *httpx.Client) *NaroAdapter {
	return &NaroAdapter{
		Client:      client,
		APIBase:     defaultNaroAPIBase,
		CatalogBase: defaultNaroCatalogBase,
	}
}

func (a *NaroAdapter) Name() string { return string(domain.SourceNaro) }

// ParseNaroURL returns the canonical uppercase ncode and catalog URL.
func ParseNaroURL(raw string) (ncode, canonical string, ok bool) {
	u, err := httpx.ValidateURL(raw)
	if err != nil {
		return "", "", false
	}
	switch strings.ToLower(u.Hostname()) {
	case "ncode.syosetu.com":
		path := strings.Trim(u.Path, "/")
		segment := strings.Split(path, "/")[0]
		if !naroCodePattern.MatchString(segment) {
			return "", "", false
		}
		ncode = strings.ToUpper(segment)
	case "api.syosetu.com":
		segment := u.Query().Get("ncode")
		if !naroCodePattern.MatchString(segment) {
			return "", "", false
		}
		ncode = strings.ToUpper(segment)
	default:
		return "", "", false
	}
	return ncode, "https://ncode.syosetu.com/" + strings.ToLower(ncode) + "/", true
}

func (a *NaroAdapter) Match(u *url.URL) bool {
	_, _, ok := ParseNaroURL(u.String())
	return ok
}

type naroAPIMeta struct {
	Title         string `json:"title"`
	NCode         string `json:"ncode"`
	Writer        string `json:"writer"`
	GeneralAllNo  *int   `json:"general_all_no"`
	GeneralLastup string `json:"general_lastup"`
	NovelType     *int   `json:"novel_type"`
	End           *int   `json:"end"`
}

func (a *NaroAdapter) Fetch(ctx context.Context, rawURL string) (*Snapshot, error) {
	ncode, canonical, ok := ParseNaroURL(rawURL)
	if !ok {
		return nil, &Error{Code: CodeInvalidURL, Message: "not a Narō novel URL"}
	}
	now := time.Now().UTC()
	apiURL := strings.TrimRight(a.APIBase, "/") + "/?out=json&ncode=" + url.QueryEscape(ncode)
	resp, err := a.Client.Get(ctx, apiURL)
	if err != nil {
		return nil, mapHTTPError(err)
	}
	meta, err := parseNaroAPI(resp.Body)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, &Error{Code: CodeNotFound, Message: "Narō returned no metadata for this ncode"}
	}
	chapterCount := 0
	knownCount := false
	if meta.GeneralAllNo != nil {
		chapterCount = *meta.GeneralAllNo
		knownCount = true
	}

	snapshot := &Snapshot{
		SourceKind:            domain.SourceNaro,
		ExternalID:            ncode,
		ExternalWorkReference: "ncode:" + ncode,
		OriginalURL:           canonical,
		Metadata: Metadata{
			OriginalTitle:   strings.TrimSpace(meta.Title),
			PublicationType: domain.PublicationWN,
			Status:          domain.StatusUnknown,
		},
		Count:      int64(chapterCount),
		Mode:       domain.ModeCatalog,
		ObservedAt: now,
	}
	status, statusKnown := naroStatus(meta.End)
	snapshot.Metadata.Status = status
	if statusKnown {
		snapshot.Metadata.StatusSource = "naro:end"
	}
	if parsed := parseNaroAPIUpTime(meta.GeneralLastup); parsed != nil {
		snapshot.Metadata.LastPublishedAt = parsed
		snapshot.Metadata.ExternalUpdatedAt = parsed
	}

	var warnings []string
	seen := map[string]bool{}
	var releases []Release
	exhausted := false
	page := 1
	for page <= naroMaxPages {
		pageURL := fmt.Sprintf("%s%s/?p=%d", strings.TrimRight(a.CatalogBase, "/")+"/", strings.ToLower(ncode), page)
		pageResp, err := a.Client.Get(ctx, pageURL)
		if err != nil {
			mapped := mapHTTPError(err)
			if page > 1 && mapped.Code == CodeNotFound && knownCount && int64(len(releases)) >= int64(chapterCount) {
				exhausted = true
				warnings = append(warnings, fmt.Sprintf("catalog page %d returned 404 after the known count; treated as exhaustion", page))
				break
			}
			warnings = append(warnings, fmt.Sprintf("catalog page %d unavailable: %s", page, mapped.Message))
			break
		}
		pageReleases, parseErr := parseNaroCatalog(pageResp.Body, ncode)
		if parseErr != nil {
			warnings = append(warnings, fmt.Sprintf("catalog page %d could not be parsed: %v", page, parseErr))
			break
		}
		if len(pageReleases) == 0 {
			if page == 1 {
				exhausted = false
				warnings = append(warnings, "catalog page 1 contained no chapters; completeness is unknown")
				break
			}
			exhausted = true
			break
		}
		base := strings.TrimRight(a.CatalogBase, "/") + "/" + strings.ToLower(ncode) + "/"
		for _, r := range pageReleases {
			if seen[r.ExternalID] {
				continue
			}
			seen[r.ExternalID] = true
			if r.URL == nil {
				u := base + ptrString(r.Number) + "/"
				r.URL = &u
			}
			r.ContentHash = HashRelease(r)
			releases = append(releases, r)
		}
		if len(pageReleases) < naroPageSize {
			exhausted = true
			break
		}
		page++
	}
	if page > naroMaxPages {
		warnings = append(warnings, "catalog page limit reached; completeness is unknown")
	}
	complete := exhausted
	if knownCount && int64(len(releases)) < int64(chapterCount) {
		complete = false
	}
	if len(releases) > 0 {
		snapshot.Releases = releases
		if int64(len(releases)) > snapshot.Count {
			snapshot.Count = int64(len(releases))
		}
		if snapshot.Metadata.LastPublishedAt == nil {
			snapshot.Metadata.LastPublishedAt = latestReleaseTime(releases)
		}
	}
	if len(releases) == 0 && !complete {
		snapshot.Mode = domain.ModeCountOnly
	}
	snapshot.Complete = complete
	snapshot.Warnings = warnings

	fields := []string{"original_url", "title"}
	if statusKnown {
		fields = append(fields, "status")
	}
	if meta.GeneralAllNo != nil {
		fields = append(fields, "release_count")
	}
	if len(releases) > 0 {
		fields = append(fields, "release_catalog", "release_titles")
		if latestReleaseTime(releases) != nil {
			fields = append(fields, "release_times")
		}
	}
	if !snapshot.Complete && len(releases) > 0 {
		fields = append(fields, "release_catalog_partial")
	}
	snapshot.FieldsAvailable = fields
	return snapshot, nil
}

// naroStatus maps the official novel API end field:
// 0 = short story or completed, 1 = ongoing. A missing, null or unsupported
// value is unknown and must not be guessed.
func naroStatus(end *int) (domain.Status, bool) {
	if end == nil {
		return domain.StatusUnknown, false
	}
	switch *end {
	case 0:
		return domain.StatusCompleted, true
	case 1:
		return domain.StatusOngoing, true
	default:
		return domain.StatusUnknown, false
	}
}

func parseNaroAPI(body []byte) (*naroAPIMeta, error) {
	var entries []json.RawMessage
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, &Error{Code: CodeFetchFailed, Message: "Narō API returned invalid JSON", Cause: err}
	}
	if len(entries) < 2 {
		return nil, &Error{Code: CodeNotFound, Message: "Narō API returned no novel data"}
	}
	var head struct {
		AllCount int `json:"allcount"`
	}
	if err := json.Unmarshal(entries[0], &head); err != nil {
		return nil, &Error{Code: CodeFetchFailed, Message: "Narō API header is invalid", Cause: err}
	}
	if head.AllCount == 0 {
		return nil, &Error{Code: CodeNotFound, Message: "Narō API reports no such novel"}
	}
	var meta naroAPIMeta
	if err := json.Unmarshal(entries[1], &meta); err != nil {
		return nil, &Error{Code: CodeFetchFailed, Message: "Narō API metadata is invalid", Cause: err}
	}
	if strings.TrimSpace(meta.NCode) == "" {
		return nil, &Error{Code: CodeNotFound, Message: "Narō API returned no ncode"}
	}
	return &meta, nil
}

func parseNaroAPIUpTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", value, jst)
	if err != nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func parseNaroCatalog(body []byte, ncode string) ([]Release, error) {
	tokenizer := html.NewTokenizer(strings.NewReader(string(body)))
	var out []Release
	var current *Release
	var inTitle, inUpdate bool
	var title, update, revision string
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			if tokenizer.Err() == io.EOF {
				break
			}
			return nil, tokenizer.Err()
		}
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := tokenizer.TagName()
			attrs := map[string]string{}
			if hasAttr {
				for {
					key, value, more := tokenizer.TagAttr()
					attrs[string(key)] = string(value)
					if !more {
						break
					}
				}
			}
			switch string(name) {
			case "a":
				if strings.Contains(attrs["class"], "p-eplist__subtitle") {
					number := naroChapterNumber(attrs["href"], ncode)
					if number == "" {
						continue
					}
					if current != nil {
						out = append(out, *current)
						current = nil
					}
					inTitle = true
					title = ""
					n := number
					current = &Release{ExternalID: strings.ToUpper(ncode) + "/" + number, Kind: domain.ReleaseChapter, Number: &n}
				}
			case "div":
				if strings.Contains(attrs["class"], "p-eplist__update") {
					inUpdate = true
					update = ""
					revision = ""
				}
			case "span":
				if inUpdate && attrs["title"] != "" {
					revision = attrs["title"]
				}
			}
		case html.TextToken:
			if inTitle {
				title += string(tokenizer.Text())
			}
			if inUpdate {
				update += string(tokenizer.Text())
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			if inTitle && string(name) == "a" {
				inTitle = false
				if current != nil {
					if t := strings.TrimSpace(title); t != "" {
						current.Title = &t
					}
				}
			}
			if inUpdate && string(name) == "div" {
				inUpdate = false
				if current != nil {
					if t := parseNaroCatalogTime(update); t != nil {
						current.PublishedAt = t
					}
					if t := parseNaroCatalogTime(revision); t != nil {
						current.SourceUpdatedAt = t
					}
					out = append(out, *current)
					current = nil
				}
			}
		}
	}
	if current != nil {
		out = append(out, *current)
	}
	return out, nil
}

func naroChapterNumber(href, ncode string) string {
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], ncode) {
		return ""
	}
	for _, ch := range parts[1] {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	if parts[1] == "" {
		return ""
	}
	return parts[1]
}

func parseNaroCatalogTime(value string) *time.Time {
	m := naroDatePattern.FindString(value)
	if m == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006/01/02 15:04", m, jst)
	if err != nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func latestReleaseTime(releases []Release) *time.Time {
	var latest *time.Time
	for _, r := range releases {
		if r.PublishedAt == nil {
			continue
		}
		if latest == nil || r.PublishedAt.After(*latest) {
			t := *r.PublishedAt
			latest = &t
		}
	}
	return latest
}
