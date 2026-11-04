// Package domain contains the minimal Release Watcher entities. It has no
// dependency on SQLite, HTTP or any notifier.
package domain

import (
	"encoding/json"
	"time"
)

type PublicationType string

const (
	PublicationWN          PublicationType = "wn"
	PublicationLN          PublicationType = "ln"
	PublicationTranslation PublicationType = "translation"
	PublicationOther       PublicationType = "other"
	PublicationUnknown     PublicationType = "unknown"
)

func (p PublicationType) Valid() bool {
	switch p {
	case PublicationWN, PublicationLN, PublicationTranslation, PublicationOther, PublicationUnknown:
		return true
	default:
		return false
	}
}

type Status string

const (
	StatusOngoing   Status = "ongoing"
	StatusCompleted Status = "completed"
	StatusUnknown   Status = "unknown"
)

func (s Status) Valid() bool {
	switch s {
	case StatusOngoing, StatusCompleted, StatusUnknown:
		return true
	default:
		return false
	}
}

type SourceKind string

const (
	SourceNaro SourceKind = "naro"
	SourceRSS  SourceKind = "rss"
	SourceAtom SourceKind = "atom"
)

func (k SourceKind) Valid() bool {
	switch k {
	case SourceNaro, SourceRSS, SourceAtom:
		return true
	default:
		return false
	}
}

type WatchMode string

const (
	ModeCatalog   WatchMode = "catalog"
	ModeCountOnly WatchMode = "count_only"
)

func (m WatchMode) Valid() bool {
	return m == ModeCatalog || m == ModeCountOnly
}

type ReleaseKind string

const (
	ReleaseChapter ReleaseKind = "chapter"
	ReleaseVolume  ReleaseKind = "volume"
	ReleaseBook    ReleaseKind = "book"
	ReleaseEntry   ReleaseKind = "entry"
	ReleaseOther   ReleaseKind = "other"
)

func (k ReleaseKind) Valid() bool {
	switch k {
	case ReleaseChapter, ReleaseVolume, ReleaseBook, ReleaseEntry, ReleaseOther:
		return true
	default:
		return false
	}
}

type EventType string

const (
	EventReleaseAdded        EventType = "release.added"
	EventReleaseUpdated      EventType = "release.updated"
	EventReleaseRemoved      EventType = "release.removed"
	EventMetadataChanged     EventType = "publication.metadata_changed"
	EventStatusChanged       EventType = "publication.status_changed"
	EventReleaseCountChanged EventType = "release.count_changed"
)

func (e EventType) Valid() bool {
	switch e {
	case EventReleaseAdded, EventReleaseUpdated, EventReleaseRemoved,
		EventMetadataChanged, EventStatusChanged, EventReleaseCountChanged:
		return true
	default:
		return false
	}
}

type DeliveryStatus string

const (
	DeliveryPending  DeliveryStatus = "pending"
	DeliverySent     DeliveryStatus = "sent"
	DeliveryFailed   DeliveryStatus = "failed"
	DeliveryCanceled DeliveryStatus = "canceled"
)

func (d DeliveryStatus) Valid() bool {
	return d == DeliveryPending || d == DeliverySent || d == DeliveryFailed || d == DeliveryCanceled
}

type ScanResult string

const (
	ScanBaseline  ScanResult = "baseline"
	ScanOK        ScanResult = "ok"
	ScanUnchanged ScanResult = "unchanged"
	ScanError     ScanResult = "error"
)

func (r ScanResult) Valid() bool {
	switch r {
	case ScanBaseline, ScanOK, ScanUnchanged, ScanError:
		return true
	default:
		return false
	}
}

// Work is a logical work, for example one web novel.
type Work struct {
	ID            string
	OriginalTitle string
	TitleRU       *string
	TitleEN       *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Source is one external site, API or feed line.
type Source struct {
	ID              string
	Kind            SourceKind
	URI             string
	ExternalID      string
	FieldsAvailable []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Publication is one concrete publication line: WN, LN, translation or mirror.
type Publication struct {
	ID              string
	WorkID          string
	SourceID        string
	Type            PublicationType
	OriginalURL     string
	Status          Status
	StatusSource    string
	LastPublishedAt *time.Time
	LastCheckedAt   *time.Time
	NextCheckAt     *time.Time
	ReleaseCount    *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Watch is one scheduled observation of a publication.
type Watch struct {
	ID                    string
	PublicationID         string
	Enabled               bool
	IntervalSeconds       int64
	BaselineEstablishedAt *time.Time
	BaselineCount         *int64
	Mode                  WatchMode
	NextCheckAt           time.Time
	LastCheckAt           *time.Time
	LastSuccessAt         *time.Time
	LastErrorCategory     string
	ConsecutiveFailures   int
	PausedAt              *time.Time
	RemovedAt             *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Release is one published unit (chapter, volume, book, feed entry).
type Release struct {
	ID              string
	PublicationID   string
	ExternalID      string
	Kind            ReleaseKind
	Number          *string
	Title           *string
	URL             *string
	PublishedAt     *time.Time
	SourceUpdatedAt *time.Time
	ContentHash     string
	IsBaseline      bool
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
	RemovedAt       *time.Time
}

// Event is an immutable detected change.
type Event struct {
	ID                    string
	Type                  EventType
	PublicationID         string
	WatchID               string
	SourceID              string
	ExternalWorkReference string
	ReleaseIDs            []string
	PreviousCount         *int64
	CurrentCount          *int64
	CountDelta            *int64
	Payload               json.RawMessage
	DedupeKey             string
	SchemaVersion         string
	DetectedAt            time.Time
	PublishedAt           *time.Time
	URL                   *string
	CreatedAt             time.Time
}

// NotificationDelivery is one durable attempt to notify about one event.
type NotificationDelivery struct {
	ID            string
	EventID       string
	Channel       string
	Status        DeliveryStatus
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
	SentAt        *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ScanRun records one watch check.
type ScanRun struct {
	ID             string
	WatchID        string
	PublicationID  string
	StartedAt      time.Time
	FinishedAt     time.Time
	Result         ScanResult
	EventsCreated  int
	ErrorCode      string
	ErrorMessage   string
	DurationMillis int64
}

// Setting is a small durable key/value preference.
type Setting struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}
