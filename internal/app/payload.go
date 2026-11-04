package app

import (
	"encoding/json"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/id"
	"github.com/HaidaDaniel/release-watcher/internal/scan"
)

type payloadWork struct {
	OriginalTitle   string  `json:"original_title"`
	TitleRU         *string `json:"title_ru"`
	TitleEN         *string `json:"title_en"`
	PublicationType string  `json:"publication_type"`
}

type payloadPublication struct {
	SourceType  string `json:"source_type"`
	OriginalURL string `json:"original_url"`
	Status      string `json:"status"`
	StatusKnown bool   `json:"status_known"`
}

type payloadRelease struct {
	ExternalID  string     `json:"external_id"`
	Kind        string     `json:"kind"`
	Number      *string    `json:"number"`
	Title       *string    `json:"title"`
	URL         *string    `json:"url"`
	PublishedAt *time.Time `json:"published_at"`
	RemovedAt   *time.Time `json:"removed_at,omitempty"`
}

type payloadMetadata struct {
	OldOriginalTitle *string `json:"old_original_title,omitempty"`
	NewOriginalTitle *string `json:"new_original_title,omitempty"`
	OldStatus        *string `json:"old_status,omitempty"`
	NewStatus        *string `json:"new_status,omitempty"`
}

type eventPayload struct {
	SchemaVersion         string             `json:"schema_version"`
	EventID               string             `json:"event_id"`
	EventType             string             `json:"event_type"`
	SourceID              string             `json:"source_id"`
	PublicationID         string             `json:"publication_id"`
	WatchID               string             `json:"watch_id"`
	ExternalWorkReference string             `json:"external_work_reference"`
	Work                  payloadWork        `json:"work"`
	Publication           payloadPublication `json:"publication"`
	ReleaseIDs            []string           `json:"release_ids"`
	Releases              []payloadRelease   `json:"releases"`
	PreviousCount         *int64             `json:"previous_count"`
	CurrentCount          *int64             `json:"current_count"`
	CountDelta            *int64             `json:"count_delta"`
	PublishedAt           *time.Time         `json:"published_at"`
	DetectedAt            time.Time          `json:"detected_at"`
	URL                   *string            `json:"url"`
	MetadataChanges       *payloadMetadata   `json:"metadata_changes"`
	DedupeKey             string             `json:"dedupe_key"`
}

func (s *Service) buildEvent(b *bundle, eventType domain.EventType, releases []domain.Release, previous, current, delta *int64, metadata *payloadMetadata, parts []string, now time.Time) (*domain.Event, error) {
	eventID := id.New("evt")
	dedupeParts := append(append([]string{}, parts...), now.UTC().Format(time.RFC3339Nano))
	dedupeKey := scan.DedupeKey(b.watch.ID, eventType, dedupeParts...)
	releaseIDs := make([]string, 0, len(releases))
	payloadReleases := make([]payloadRelease, 0, len(releases))
	for _, r := range releases {
		releaseIDs = append(releaseIDs, r.ExternalID)
		payloadReleases = append(payloadReleases, payloadRelease{
			ExternalID:  r.ExternalID,
			Kind:        string(r.Kind),
			Number:      r.Number,
			Title:       r.Title,
			URL:         r.URL,
			PublishedAt: r.PublishedAt,
			RemovedAt:   r.RemovedAt,
		})
	}
	url := b.publication.OriginalURL
	payload := eventPayload{
		SchemaVersion:         "1.0",
		EventID:               eventID,
		EventType:             string(eventType),
		SourceID:              b.source.ID,
		PublicationID:         b.publication.ID,
		WatchID:               b.watch.ID,
		ExternalWorkReference: externalWorkReference(b.source),
		Work: payloadWork{
			OriginalTitle:   b.work.OriginalTitle,
			TitleRU:         b.work.TitleRU,
			TitleEN:         b.work.TitleEN,
			PublicationType: string(b.publication.Type),
		},
		Publication: payloadPublication{
			SourceType:  string(b.source.Kind),
			OriginalURL: b.publication.OriginalURL,
			Status:      string(b.publication.Status),
			StatusKnown: b.publication.Status != domain.StatusUnknown,
		},
		ReleaseIDs:      releaseIDs,
		Releases:        payloadReleases,
		PreviousCount:   previous,
		CurrentCount:    current,
		CountDelta:      delta,
		PublishedAt:     scan.LatestPublishedAt(releases),
		DetectedAt:      now,
		URL:             &url,
		MetadataChanges: metadata,
		DedupeKey:       dedupeKey,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &domain.Event{
		ID:                    eventID,
		Type:                  eventType,
		PublicationID:         b.publication.ID,
		WatchID:               b.watch.ID,
		SourceID:              b.source.ID,
		ExternalWorkReference: payload.ExternalWorkReference,
		ReleaseIDs:            releaseIDs,
		PreviousCount:         previous,
		CurrentCount:          current,
		CountDelta:            delta,
		Payload:               data,
		DedupeKey:             dedupeKey,
		SchemaVersion:         "1.0",
		DetectedAt:            now,
		PublishedAt:           payload.PublishedAt,
		URL:                   &url,
		CreatedAt:             now,
	}, nil
}

func externalWorkReference(src *domain.Source) string {
	switch src.Kind {
	case domain.SourceNaro:
		return "ncode:" + src.ExternalID
	default:
		return "feed:" + src.ExternalID
	}
}
