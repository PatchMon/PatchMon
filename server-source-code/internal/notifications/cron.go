package notifications

import (
	"time"

	"github.com/robfig/cron/v3"
)

// NextCronRun returns the next scheduled instant after `from` for a standard 5-field cron in IANA timezone.
func NextCronRun(expr, tz string, from time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	sched, err := cron.ParseStandard(expr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(from.In(loc)), nil
}
