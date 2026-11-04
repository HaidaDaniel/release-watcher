package app

import (
	"context"
)

func (s *Service) List(ctx context.Context) (*ListResult, *Error) {
	watches, err := s.store.Queries().ListWatches(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]WatchView, 0, len(watches))
	for _, watch := range watches {
		b, loadErr := s.loadBundle(ctx, s.store.Queries(), watch.ID)
		if loadErr != nil {
			return nil, mapError(loadErr)
		}
		out = append(out, s.watchView(b))
	}
	return &ListResult{Watches: out}, nil
}
