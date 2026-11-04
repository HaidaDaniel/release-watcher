package app

import "time"

const calendarDay = 24 * time.Hour

// nextCheckAfter returns the next due time for a successful check. Intervals
// that are whole local calendar days use the next eligible local midnight so a
// daily timer cannot skip a day because of jitter or DST. Other intervals stay
// duration-based and scheduler-agnostic.
func (s *Service) nextCheckAfter(lastSuccess time.Time, interval time.Duration) time.Time {
	return nextCheckAfter(lastSuccess, interval, s.location())
}

func (s *Service) location() *time.Location {
	if s.cfg.Location != nil {
		return s.cfg.Location
	}
	return time.Local
}

func nextCheckAfter(lastSuccess time.Time, interval time.Duration, location *time.Location) time.Time {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if location == nil {
		location = time.Local
	}
	if interval%calendarDay == 0 {
		days := int(interval / calendarDay)
		local := lastSuccess.In(location)
		year, month, day := local.Date()
		return time.Date(year, month, day+days, 0, 0, 0, 0, location).UTC()
	}
	return lastSuccess.Add(interval).UTC()
}
