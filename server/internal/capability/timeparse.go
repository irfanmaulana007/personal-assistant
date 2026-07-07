package capability

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseTime attempts to parse a natural language time reference into a time.Time.
func ParseTime(text string, loc *time.Location) (time.Time, error) {
	now := time.Now().In(loc)
	text = strings.TrimSpace(strings.ToLower(text))

	// Relative: "in 5 minutes", "in 2 hours"
	if rel := parseRelative(text, now); !rel.IsZero() {
		return rel, nil
	}

	// Absolute time: "at 3pm", "at 15:00"
	if abs := parseAbsoluteTime(text, now, loc); !abs.IsZero() {
		return abs, nil
	}

	// Day references: "today", "tomorrow", weekday names
	if day := parseDayReference(text, now, loc); !day.IsZero() {
		return day, nil
	}

	// Combined: "tomorrow at 3pm", "friday at 2:30pm"
	if combined := parseCombined(text, now, loc); !combined.IsZero() {
		return combined, nil
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

var timeRe = regexp.MustCompile(`(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)?`)

func parseAbsoluteTime(text string, now time.Time, loc *time.Location) time.Time {
	// Only match if the text is primarily a time reference
	if !strings.Contains(text, "at ") && !regexp.MustCompile(`^\d{1,2}(:\d{2})?\s*(am|pm)$`).MatchString(text) {
		return time.Time{}
	}

	m := timeRe.FindStringSubmatch(text)
	if m == nil {
		return time.Time{}
	}

	hour, _ := strconv.Atoi(m[1])
	minute := 0
	if m[2] != "" {
		minute, _ = strconv.Atoi(m[2])
	}

	if m[3] == "pm" && hour < 12 {
		hour += 12
	} else if m[3] == "am" && hour == 12 {
		hour = 0
	}

	t := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if t.Before(now) {
		t = t.AddDate(0, 0, 1) // next day if time already passed
	}
	return t
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

func parseDayReference(text string, now time.Time, loc *time.Location) time.Time {
	switch {
	case text == "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, loc)
	case text == "tomorrow":
		return time.Date(now.Year(), now.Month(), now.Day()+1, 9, 0, 0, 0, loc)
	}

	for name, wd := range weekdays {
		if strings.Contains(text, name) {
			daysAhead := int(wd) - int(now.Weekday())
			if daysAhead <= 0 {
				daysAhead += 7
			}
			d := now.AddDate(0, 0, daysAhead)
			return time.Date(d.Year(), d.Month(), d.Day(), 9, 0, 0, 0, loc)
		}
	}

	return time.Time{}
}

func parseCombined(text string, now time.Time, loc *time.Location) time.Time {
	// Try to split on " at "
	parts := strings.SplitN(text, " at ", 2)
	if len(parts) != 2 {
		return time.Time{}
	}

	dayPart := strings.TrimSpace(parts[0])
	timePart := "at " + strings.TrimSpace(parts[1])

	// Parse day
	day := parseDayReference(dayPart, now, loc)
	if day.IsZero() {
		return time.Time{}
	}

	// Parse time
	m := timeRe.FindStringSubmatch(timePart)
	if m == nil {
		return time.Time{}
	}

	hour, _ := strconv.Atoi(m[1])
	minute := 0
	if m[2] != "" {
		minute, _ = strconv.Atoi(m[2])
	}
	if m[3] == "pm" && hour < 12 {
		hour += 12
	} else if m[3] == "am" && hour == 12 {
		hour = 0
	}

	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc)
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
