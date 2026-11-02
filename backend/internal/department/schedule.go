package department

import (
	"fmt"
	"time"
)

// IsOpen reports whether the weekly schedule is open at the given instant, evaluated in the
// department's timezone. An empty schedule means the department is always closed (D23).
func IsOpen(timezone string, hours []HoursRange, now time.Time) (bool, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return false, fmt.Errorf("department timezone %q: %w", timezone, err)
	}
	local := now.In(loc)
	weekday := int(local.Weekday())
	minute := local.Hour()*60 + local.Minute()
	for _, h := range hours {
		if h.Weekday != weekday {
			continue
		}
		open, okOpen := parseClock(h.Opens, false)
		closing, okClose := parseClock(h.Closes, true)
		if !okOpen || !okClose {
			return false, fmt.Errorf("department hours: malformed range %+v", h)
		}
		if minute >= open && minute < closing {
			return true, nil
		}
	}
	return false, nil
}
