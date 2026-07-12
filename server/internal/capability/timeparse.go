package capability

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseTime attempts to parse a natural language time reference into a time.Time.
//
// The date and the time-of-day are parsed independently, then combined. Parsing
// them separately is what stops a date component ("2026-08-05", "July 18", a
// weekday) from being swallowed by the time parser — and vice versa. A bare
// number inside a date (the "18" in "July 18") is never mistaken for an hour,
// because the time parser only accepts a time introduced by "at", carrying
// am/pm, or written with a colon.
func ParseTime(text string, loc *time.Location) (time.Time, error) {
	now := time.Now().In(loc)
	text = strings.TrimSpace(strings.ToLower(text))

	// Relative: "in 5 minutes", "in 2 hours"
	if rel := parseRelative(text, now); !rel.IsZero() {
		return rel, nil
	}

	date, dateOK := parseDate(text, now, loc)
	hour, minute, timeOK := parseTimeOfDay(text)

	switch {
	case dateOK:
		h, m := 9, 0 // default to 9am when only a date is given
		if timeOK {
			h, m = hour, minute
		}
		return time.Date(date.Year(), date.Month(), date.Day(), h, m, 0, 0, loc), nil
	case timeOK:
		// Time only, no date: today, rolling to tomorrow if already past.
		t := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
		if t.Before(now) {
			t = t.AddDate(0, 0, 1)
		}
		return t, nil
	}

	return time.Time{}, fmt.Errorf("could not parse time from %q", text)
}

var relativeRe = regexp.MustCompile(`in\s+(\d+)\s*(min(?:ute)?s?|hours?|hrs?|days?|weeks?)`)

func parseRelative(text string, now time.Time) time.Time {
	m := relativeRe.FindStringSubmatch(text)
	if m == nil {
		return time.Time{}
	}

	n, _ := strconv.Atoi(m[1])
	unit := m[2]

	switch {
	case strings.HasPrefix(unit, "min"):
		return now.Add(time.Duration(n) * time.Minute)
	case strings.HasPrefix(unit, "h"):
		return now.Add(time.Duration(n) * time.Hour)
	case strings.HasPrefix(unit, "d"):
		return now.AddDate(0, 0, n)
	case strings.HasPrefix(unit, "w"):
		return now.AddDate(0, 0, n*7)
	}
	return time.Time{}
}

var (
	// "at 3pm", "at 15:00", "at 9:30 am"
	atTimeRe = regexp.MustCompile(`\bat\s+(\d{1,2})(?::(\d{2}))?\s*(am|pm)?`)
	// "3pm", "9:30am" — a bare time that carries am/pm
	meridiemRe = regexp.MustCompile(`\b(\d{1,2})(?::(\d{2}))?\s*(am|pm)\b`)
	// "15:00", "09:30" — a bare 24-hour clock time
	clockRe = regexp.MustCompile(`\b(\d{1,2}):(\d{2})\b`)
)

// parseTimeOfDay extracts an hour/minute from the text. It only recognises an
// explicit time — one introduced by "at", carrying am/pm, or written with a
// colon — so a bare date number is never read as an hour.
func parseTimeOfDay(text string) (hour, minute int, ok bool) {
	if m := atTimeRe.FindStringSubmatch(text); m != nil {
		return normalizeClock(m[1], m[2], m[3])
	}
	if m := meridiemRe.FindStringSubmatch(text); m != nil {
		return normalizeClock(m[1], m[2], m[3])
	}
	if m := clockRe.FindStringSubmatch(text); m != nil {
		return normalizeClock(m[1], m[2], "")
	}
	return 0, 0, false
}

func normalizeClock(hourStr, minStr, meridiem string) (int, int, bool) {
	hour, err := strconv.Atoi(hourStr)
	if err != nil {
		return 0, 0, false
	}
	minute := 0
	if minStr != "" {
		minute, _ = strconv.Atoi(minStr)
	}
	switch meridiem {
	case "pm":
		if hour < 12 {
			hour += 12
		}
	case "am":
		if hour == 12 {
			hour = 0
		}
	}
	if hour > 23 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}

var weekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "sun": time.Sunday,
	"monday": time.Monday, "mon": time.Monday,
	"tuesday": time.Tuesday, "tue": time.Tuesday,
	"wednesday": time.Wednesday, "wed": time.Wednesday,
	"thursday": time.Thursday, "thu": time.Thursday,
	"friday": time.Friday, "fri": time.Friday,
	"saturday": time.Saturday, "sat": time.Saturday,
}

var months = map[string]time.Month{
	"january": time.January, "jan": time.January,
	"february": time.February, "feb": time.February,
	"march": time.March, "mar": time.March,
	"april": time.April, "apr": time.April,
	"may": time.May,
	"june": time.June, "jun": time.June,
	"july": time.July, "jul": time.July,
	"august": time.August, "aug": time.August,
	"september": time.September, "sept": time.September, "sep": time.September,
	"october": time.October, "oct": time.October,
	"november": time.November, "nov": time.November,
	"december": time.December, "dec": time.December,
}

const monthAlt = `jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|` +
	`aug(?:ust)?|sept?(?:ember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?`

var (
	// "2026-08-05" or "2026/08/05"
	isoDateRe = regexp.MustCompile(`\b(\d{4})[-/](\d{1,2})[-/](\d{1,2})\b`)
	// "Aug 5", "August 18th, 2026" — the trailing \b keeps the day from
	// swallowing the leading digits of a following year ("July 2026").
	monthDayRe = regexp.MustCompile(`\b(` + monthAlt + `)\s+(\d{1,2})(?:st|nd|rd|th)?\b(?:,?\s+(\d{4}))?`)
	// "5 Aug", "18 July 2026"
	dayMonthRe = regexp.MustCompile(`\b(\d{1,2})(?:st|nd|rd|th)?\s+(` + monthAlt + `)(?:\s+(\d{4}))?`)
)

// parseDate extracts a calendar date from the text, understanding ISO dates
// ("2026-08-05"), month-name dates ("Aug 5", "18 July 2026"), "today"/
// "tomorrow", and weekday names. The returned time carries only the date; its
// clock fields are set by the caller.
func parseDate(text string, now time.Time, loc *time.Location) (time.Time, bool) {
	if m := isoDateRe.FindStringSubmatch(text); m != nil {
		year, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		day, _ := strconv.Atoi(m[3])
		if month >= 1 && month <= 12 && day >= 1 && day <= 31 {
			return time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc), true
		}
	}

	if m := monthDayRe.FindStringSubmatch(text); m != nil {
		if d, ok := buildMonthDate(m[1], m[2], m[3], now, loc); ok {
			return d, true
		}
	}
	if m := dayMonthRe.FindStringSubmatch(text); m != nil {
		if d, ok := buildMonthDate(m[2], m[1], m[3], now, loc); ok {
			return d, true
		}
	}

	switch {
	case strings.Contains(text, "tomorrow"):
		d := now.AddDate(0, 0, 1)
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc), true
	case strings.Contains(text, "today"):
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc), true
	}

	for name, wd := range weekdays {
		if containsWord(text, name) {
			daysAhead := int(wd) - int(now.Weekday())
			if daysAhead <= 0 {
				daysAhead += 7 // the next occurrence, never today or in the past
			}
			d := now.AddDate(0, 0, daysAhead)
			return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc), true
		}
	}

	return time.Time{}, false
}

func buildMonthDate(monthName, dayStr, yearStr string, now time.Time, loc *time.Location) (time.Time, bool) {
	month, ok := months[monthName]
	if !ok {
		return time.Time{}, false
	}
	day, err := strconv.Atoi(dayStr)
	if err != nil || day < 1 || day > 31 {
		return time.Time{}, false
	}

	year := now.Year()
	explicitYear := yearStr != ""
	if explicitYear {
		year, _ = strconv.Atoi(yearStr)
	}

	d := time.Date(year, month, day, 0, 0, 0, 0, loc)
	// With no explicit year, a date that already passed this year rolls forward
	// so "Aug 5" always means the next Aug 5, never one in the past.
	if !explicitYear {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		if d.Before(today) {
			d = d.AddDate(1, 0, 0)
		}
	}
	return d, true
}

// containsWord reports whether word appears in text delimited by word
// boundaries, so a weekday abbreviation like "sat" is not matched inside an
// unrelated word.
func containsWord(text, word string) bool {
	for idx := 0; ; {
		i := strings.Index(text[idx:], word)
		if i < 0 {
			return false
		}
		i += idx
		beforeOK := i == 0 || !isWordChar(text[i-1])
		afterOK := i+len(word) >= len(text) || !isWordChar(text[i+len(word)])
		if beforeOK && afterOK {
			return true
		}
		idx = i + 1
	}
}

func isWordChar(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

// ParseDateRange returns the start and end of a day reference for calendar queries.
func ParseDateRange(text string, loc *time.Location) (time.Time, time.Time) {
	now := time.Now().In(loc)
	text = strings.TrimSpace(strings.ToLower(text))

	var dayStart time.Time

	switch {
	case text == "" || text == "today":
		dayStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	case text == "tomorrow":
		dayStart = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	case text == "this week" || text == "week":
		dayStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		dayEnd := dayStart.AddDate(0, 0, 7)
		return dayStart, dayEnd
	default:
		for name, wd := range weekdays {
			if strings.Contains(text, name) {
				daysAhead := int(wd) - int(now.Weekday())
				if daysAhead <= 0 {
					daysAhead += 7
				}
				d := now.AddDate(0, 0, daysAhead)
				dayStart = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
				break
			}
		}
	}

	if dayStart.IsZero() {
		dayStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}

	dayEnd := dayStart.AddDate(0, 0, 1)
	return dayStart, dayEnd
}
