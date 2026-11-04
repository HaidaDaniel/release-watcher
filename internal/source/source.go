// Package source contains normalized content source adapters.
package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/source/httpx"
)

const (
	CodeInvalidURL        = "invalid_url"
	CodeUnsupportedSource = "unsupported_source"
	CodeSSRFBlocked       = "ssrf_blocked"
	CodeSourceTimeout     = "source_timeout"
	CodeRateLimited       = "rate_limited"
	CodeNotFound          = "source_not_found"
	CodeForbidden         = "source_forbidden"
	CodeFetchFailed       = "fetch_failed"
)

// Error is a typed source error suitable for CLI and diagnostics.
type Error struct {
	Code      string
	Message   string
	Temporary bool
	Cause     error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// Metadata is normalized publication metadata. Unknown values stay zero.
type Metadata struct {
	OriginalTitle     string
	PublicationType   domain.PublicationType
	Status            domain.Status
	StatusSource      string
	LastPublishedAt   *time.Time
	ExternalUpdatedAt *time.Time
}

// Release is one normalized published unit.
type Release struct {
	ExternalID      string
	Kind            domain.ReleaseKind
	Number          *string
	Title           *string
	URL             *string
	PublishedAt     *time.Time
	SourceUpdatedAt *time.Time
	ContentHash     string
}

// Snapshot is one normalized fetch of a publication line.
type Snapshot struct {
	SourceKind            domain.SourceKind
	ExternalID            string
	ExternalWorkReference string
	OriginalURL           string
	Metadata              Metadata
	Releases              []Release
	Count                 int64
	Mode                  domain.WatchMode
	Complete              bool
	FieldsAvailable       []string
	Warnings              []string
	ObservedAt            time.Time
}

// Adapter turns a source URL into a normalized snapshot.
type Adapter interface {
	Name() string
	Fetch(ctx context.Context, rawURL string) (*Snapshot, error)
}

// Registry selects and runs adapters.
type Registry struct {
	naro *NaroAdapter
	feed *FeedAdapter
}

func NewRegistry(client *httpx.Client) *Registry {
	return &Registry{
		naro: NewNaro(client),
		feed: NewFeed(client),
	}
}

// Fetch selects by URL. Narō URLs are matched by host; everything else is
// attempted as RSS/Atom and rejected with unsupported_source if it is not a feed.
func (r *Registry) Fetch(ctx context.Context, rawURL string) (*Snapshot, error) {
	u, err := httpx.ValidateURL(rawURL)
	if err != nil {
		return nil, mapHTTPError(err)
	}
	if r.naro.Match(u) {
		return r.naro.Fetch(ctx, rawURL)
	}
	return r.feed.Fetch(ctx, rawURL)
}

// FetchByKind uses the stored source kind for scans.
func (r *Registry) FetchByKind(ctx context.Context, kind domain.SourceKind, rawURL string) (*Snapshot, error) {
	switch kind {
	case domain.SourceNaro:
		return r.naro.Fetch(ctx, rawURL)
	case domain.SourceRSS, domain.SourceAtom:
		return r.feed.Fetch(ctx, rawURL)
	default:
		return nil, &Error{Code: CodeUnsupportedSource, Message: fmt.Sprintf("unsupported source kind %q", kind)}
	}
}

func mapHTTPError(err error) *Error {
	var hErr *httpx.Error
	if errors.As(err, &hErr) {
		return &Error{Code: hErr.Code, Message: hErr.Message, Temporary: hErr.Temporary, Cause: hErr.Cause}
	}
	return &Error{Code: CodeFetchFailed, Message: "source request failed", Temporary: true, Cause: err}
}

// HashRelease returns a bounded fingerprint of identity-relevant release fields.
func HashRelease(r Release) string {
	h := sha256.New()
	writeHashField(h, r.ExternalID)
	writeHashField(h, string(r.Kind))
	writeHashField(h, ptrString(r.Number))
	writeHashField(h, ptrString(r.Title))
	writeHashField(h, ptrString(r.URL))
	writeHashField(h, timeString(r.PublishedAt))
	writeHashField(h, timeString(r.SourceUpdatedAt))
	return hex.EncodeToString(h.Sum(nil))
}

func writeHashField(w io.Writer, value string) {
	_, _ = io.WriteString(w, value)
	_, _ = io.WriteString(w, "\x00")
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func timeString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	v := value
	return &v
}
