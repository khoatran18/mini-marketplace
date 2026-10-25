package backfill

import (
	"fmt"
	"time"

	_ "time/tzdata" // Asia/Ho_Chi_Minh must resolve in minimal images

	"analytics-service/internal/store"
)

// Range is a half-open instant range [From, To).
type Range struct{ From, To time.Time }

// ParseDays turns two calendar dates (YYYY-MM-DD, platform timezone Asia/Ho_Chi_Minh, BOTH inclusive) into a
// Range: from 00:00 of `from` up to (excluding) 00:00 of the day after `to`.
func ParseDays(from, to string) (Range, error) {
	loc, err := time.LoadLocation(store.Timezone)
	if err != nil {
		return Range{}, err
	}
	f, err := time.ParseInLocation("2006-01-02", from, loc)
	if err != nil {
		return Range{}, fmt.Errorf("--from %q: want YYYY-MM-DD", from)
	}
	t, err := time.ParseInLocation("2006-01-02", to, loc)
	if err != nil {
		return Range{}, fmt.Errorf("--to %q: want YYYY-MM-DD", to)
	}
	if t.Before(f) {
		return Range{}, fmt.Errorf("--to is before --from")
	}
	return Range{From: f.UTC(), To: t.AddDate(0, 0, 1).UTC()}, nil
}

func mustLoc() *time.Location {
	loc, err := time.LoadLocation(store.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}
