package api

import (
	"testing"
	"time"
)

func TestNormalizeTimes(t *testing.T) {
	got, err := normalizeTimes([]string{"8:5", "20:00", "08:05"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// zero-padded, deduped, sorted.
	if len(got) != 2 || got[0] != "08:05" || got[1] != "20:00" {
		t.Fatalf("normalizeTimes = %v", got)
	}

	if _, err := normalizeTimes(nil); err == nil {
		t.Error("expected error for empty times")
	}
	if _, err := normalizeTimes([]string{"25:00"}); err == nil {
		t.Error("expected error for out-of-range hour")
	}
	if _, err := normalizeTimes([]string{"noon"}); err == nil {
		t.Error("expected error for non HH:MM")
	}
}

func TestValidateReminder(t *testing.T) {
	tz := time.UTC
	future := time.Now().In(tz).AddDate(0, 0, 2).Format("2006-01-02")

	// Weekly happy path.
	in, err := validateReminder(reminderReq{
		Title: "Vitamins", RepeatMode: "weekly", Times: []string{"08:00"}, Weekdays: []int{3, 1, 1},
	}, tz)
	if err != nil {
		t.Fatalf("weekly valid: %v", err)
	}
	if len(in.Weekdays) != 2 || in.Weekdays[0] != 1 || in.Weekdays[1] != 3 {
		t.Errorf("weekdays not deduped/sorted: %v", in.Weekdays)
	}

	// Missing title.
	if _, err := validateReminder(reminderReq{RepeatMode: "daily", Times: []string{"08:00"}}, tz); err == nil {
		t.Error("expected error for missing title")
	}
	// Bad mode.
	if _, err := validateReminder(reminderReq{Title: "x", RepeatMode: "hourly", Times: []string{"08:00"}}, tz); err == nil {
		t.Error("expected error for bad repeat_mode")
	}
	// Weekly without weekdays.
	if _, err := validateReminder(reminderReq{Title: "x", RepeatMode: "weekly", Times: []string{"08:00"}}, tz); err == nil {
		t.Error("expected error for weekly without weekdays")
	}
	// Monthly out of range.
	if _, err := validateReminder(reminderReq{Title: "x", RepeatMode: "monthly", DayOfMonth: 40, Times: []string{"08:00"}}, tz); err == nil {
		t.Error("expected error for day_of_month 40")
	}
	// Once in the past.
	if _, err := validateReminder(reminderReq{Title: "x", RepeatMode: "once", OnceDate: "2000-01-01", Times: []string{"08:00"}}, tz); err == nil {
		t.Error("expected error for past once_date")
	}
	// Once in the future is fine.
	if _, err := validateReminder(reminderReq{Title: "x", RepeatMode: "once", OnceDate: future, Times: []string{"08:00"}}, tz); err != nil {
		t.Errorf("future once should be valid: %v", err)
	}
}
