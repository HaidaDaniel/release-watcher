package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/store"
)

type EventQuery struct {
	PublicationID string
	WatchID       string
	Since         *time.Time
	After         string
	Limit         int
}

func (s *Service) Events(ctx context.Context, input EventQuery) (*EventsResult, *Error) {
	events, err := s.store.Queries().ListEvents(ctx, store.EventFilter{
		PublicationID: input.PublicationID,
		WatchID:       input.WatchID,
		Since:         input.Since,
		After:         input.After,
		Limit:         input.Limit,
	})
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		payload, payloadErr := rawEventPayload(event)
		if payloadErr != nil {
			return nil, mapError(payloadErr)
		}
		out = append(out, payload)
	}
	nextCursor := ""
	if len(events) > 0 {
		nextCursor = events[len(events)-1].ID
	}
	return &EventsResult{Events: out, NextCursor: nextCursor}, nil
}
