package department

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/validate"
)

type Input struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Timezone    string `json:"timezone"`
	IsActive    *bool  `json:"is_active"`
}

func (in *Input) Validate() error {
	v := validate.Errors{}
	in.Name = v.RequiredString("name", in.Name, 80)
	in.Description = v.OptionalString("description", in.Description, 500)
	in.Timezone = v.RequiredString("timezone", in.Timezone, 64)
	if in.Timezone != "" {
		// time/tzdata is embedded in the binary, so this works on hosts without a zoneinfo database.
		if _, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "Local" {
			v.Add("timezone", "Choose a valid timezone.")
		}
	}
	if in.IsActive == nil {
		v.Add("is_active", "This field is required.")
	}
	return v.Err()
}

const maxLinkedUsers = 500

type UsersInput struct {
	UserIDs []uuid.UUID `json:"user_ids"`
}

func (in *UsersInput) Validate() error {
	v := validate.Errors{}
	if in.UserIDs == nil {
		in.UserIDs = []uuid.UUID{}
	}
	slices.SortFunc(in.UserIDs, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	in.UserIDs = slices.Compact(in.UserIDs)
	if len(in.UserIDs) > maxLinkedUsers {
		v.Add("user_ids", fmt.Sprintf("At most %d users can be assigned.", maxLinkedUsers))
	}
	return v.Err()
}

const maxRangesPerDay = 6

type HoursInput struct {
	Hours []HoursRange `json:"business_hours"`
}

var clockPattern = regexp.MustCompile(`^([01]\d|2[0-4]):([0-5]\d)$`)

// parseClock converts "HH:MM" into minutes since midnight. "24:00" is allowed only as a closing time.
func parseClock(s string, allowEndOfDay bool) (int, bool) {
	m := clockPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	if h == 24 && (min != 0 || !allowEndOfDay) {
		return 0, false
	}
	return h*60 + min, true
}

func formatClock(minutes int) string { return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60) }

// Validate checks each range and rejects overlapping ranges on the same weekday.
// Field keys use the index into business_hours so the UI can highlight the right row.
func (in *HoursInput) Validate() error {
	v := validate.Errors{}
	if in.Hours == nil {
		in.Hours = []HoursRange{}
	}
	type span struct{ idx, open, close int }
	byDay := map[int][]span{}
	for i, h := range in.Hours {
		key := fmt.Sprintf("business_hours.%d", i)
		if h.Weekday < 0 || h.Weekday > 6 {
			v.Add(key, "Choose a valid day.")
			continue
		}
		open, okOpen := parseClock(h.Opens, false)
		closing, okClose := parseClock(h.Closes, true)
		if !okOpen || !okClose {
			v.Add(key, "Use 24-hour HH:MM times.")
			continue
		}
		if open >= closing {
			v.Add(key, "Opening time must be before closing time.")
			continue
		}
		in.Hours[i] = HoursRange{Weekday: h.Weekday, Opens: formatClock(open), Closes: formatClock(closing)}
		byDay[h.Weekday] = append(byDay[h.Weekday], span{i, open, closing})
	}
	for _, spans := range byDay {
		if len(spans) > maxRangesPerDay {
			v.Add(fmt.Sprintf("business_hours.%d", spans[0].idx), fmt.Sprintf("At most %d ranges per day.", maxRangesPerDay))
			continue
		}
		slices.SortFunc(spans, func(a, b span) int { return a.open - b.open })
		for j := 1; j < len(spans); j++ {
			if spans[j].open < spans[j-1].close {
				v.Add(fmt.Sprintf("business_hours.%d", spans[j].idx), "Overlaps another range on the same day.")
			}
		}
	}
	return v.Err()
}

// FilterFromQuery validates list filters; invalid values are ignored rather than rejected.
func FilterFromQuery(query, active string) Filter {
	f := Filter{Query: strings.TrimSpace(query)}
	if len([]rune(f.Query)) > 100 {
		f.Query = string([]rune(f.Query)[:100])
	}
	if b, err := strconv.ParseBool(active); err == nil {
		f.Active = &b
	}
	return f
}
