package capability

import (
	"testing"
	"time"
)

// tz is a fixed offset zone so tests never depend on the host's location.
var tz = time.FixedZone("WIB", 7*3600)

// TestParseTimeAbsolute covers inputs that resolve to a fixed instant
// regardless of the current time — dates carrying an explicit year.
func TestParseTimeAbsolute(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
	}{
		// ISO date + 24h time — the exact format schedule_event's error message
		// suggests, and the one run #94 was rejected on.
		{"2026-08-05 14:00", time.Date(2026, 8, 5, 14, 0, 0, 0, tz)},
		{"2026-07-18 09:00", time.Date(2026, 7, 18, 9, 0, 0, 0, tz)},
		{"2026/07/18", time.Date(2026, 7, 18, 9, 0, 0, 0, tz)}, // date only -> 9am
		// Month-name dates with an explicit year, both orderings.
		{"18 July 2026 at 9am", time.Date(2026, 7, 18, 9, 0, 0, 0, tz)},
		{"July 18 2026 at 9am", time.Date(2026, 7, 18, 9, 0, 0, 0, tz)},
		{"Aug 5, 2026 at 2:30pm", time.Date(2026, 8, 5, 14, 30, 0, 0, tz)},
		// The day number must not be read as the hour (Bug B): here "5" is the
		// day and "9am" is the time.
		{"August 5 2026 at 9am", time.Date(2026, 8, 5, 9, 0, 0, 0, tz)},
	}

	for _, c := range cases {
		got, err := ParseTime(c.in, tz)
		if err != nil {
			t.Errorf("ParseTime(%q) unexpected error: %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("ParseTime(%q) = %s, want %s", c.in, got.Format(time.RFC3339), c.want.Format(time.RFC3339))
		}
	}
}

// TestParseTimeWeekdayNotHijacked guards the specific regression from run #94:
// a weekday plus an "at <time>" must resolve to that weekday, not to today
// bumped by the time parser.
func TestParseTimeWeekdayNotHijacked(t *testing.T) {
	got, err := ParseTime("this Saturday at 9am", tz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Weekday() != time.Saturday {
		t.Errorf("ParseTime(\"this Saturday at 9am\") landed on %s, want Saturday", got.Weekday())
	}
	if got.Hour() != 9 || got.Minute() != 0 {
		t.Errorf("got %02d:%02d, want 09:00", got.Hour(), got.Minute())
	}
	now := time.Now().In(tz)
	if !got.After(now) {
		t.Errorf("resolved time %s is not in the future (now %s)", got, now)
	}
}

// TestParseTimeRelativeToNow covers now-relative forms, computing the
// expectation from time.Now() the same way the parser does.
func TestParseTimeRelativeToNow(t *testing.T) {
	now := time.Now().In(tz)

	// tomorrow at 3pm
	got, err := ParseTime("tomorrow at 3pm", tz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantDay := now.AddDate(0, 0, 1)
	if got.Year() != wantDay.Year() || got.YearDay() != wantDay.YearDay() {
		t.Errorf("tomorrow: got %s, want the day after %s", got, now)
	}
	if got.Hour() != 15 || got.Minute() != 0 {
		t.Errorf("tomorrow at 3pm: got %02d:%02d, want 15:00", got.Hour(), got.Minute())
	}

	// "in 2 hours"
	got, err = ParseTime("in 2 hours", tz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d := got.Sub(now); d < 110*time.Minute || d > 130*time.Minute {
		t.Errorf("in 2 hours: got delta %s, want ~2h", d)
	}
}

func TestParseTimeUnparseable(t *testing.T) {
	for _, in := range []string{"", "sometime soonish", "the usual place"} {
		if _, err := ParseTime(in, tz); err == nil {
			t.Errorf("ParseTime(%q) expected an error, got none", in)
		}
	}
}
