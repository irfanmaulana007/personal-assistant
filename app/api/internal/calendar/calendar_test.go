package calendar

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func testService() *Service {
	return &Service{tz: time.FixedZone("WIB", 7*3600), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestParseCreatedEventID(t *testing.T) {
	cases := map[string]string{
		`{"id":"abc"}`: "abc",
		// The real runtime shape: ExecuteTool strips the outer "data", leaving the
		// id under a top-level "response_data".
		`{"response_data":{"id":"jkl"}}`: "jkl",
		// Tolerated (un-unwrapped) shapes.
		`{"data":{"id":"def"}}`:                   "def",
		`{"data":{"response_data":{"id":"ghi"}}}`: "ghi",
		`{"foo":1}`: "",
		`not json`:  "",
	}
	for raw, want := range cases {
		if got := parseCreatedEventID(raw); got != want {
			t.Errorf("parseCreatedEventID(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestMatchEventID(t *testing.T) {
	tz := time.FixedZone("WIB", 7*3600)
	at := func(h, m int) time.Time { return time.Date(2026, 3, 10, h, m, 0, 0, tz) }
	existing := []Event{
		{ID: "e1", Title: "Take medicine", Start: at(8, 0)},
		{ID: "e2", Title: "Standup", Start: at(9, 0)},
	}

	// Same title + start (case/whitespace-insensitive) matches.
	if got := matchEventID(existing, Event{Title: "  take MEDICINE ", Start: at(8, 0)}); got != "e1" {
		t.Errorf("expected e1, got %q", got)
	}
	// Seconds differ but the minute is the same -> still a match.
	if got := matchEventID(existing, Event{Title: "Standup", Start: at(9, 0).Add(30 * time.Second)}); got != "e2" {
		t.Errorf("expected e2 within the minute, got %q", got)
	}
	// Different time -> no match (would legitimately create a new event).
	if got := matchEventID(existing, Event{Title: "Standup", Start: at(10, 0)}); got != "" {
		t.Errorf("expected no match for different time, got %q", got)
	}
	// Different title -> no match.
	if got := matchEventID(existing, Event{Title: "Lunch", Start: at(8, 0)}); got != "" {
		t.Errorf("expected no match for different title, got %q", got)
	}
	// A candidate with an empty id is never adopted (can't reference it later).
	if got := matchEventID([]Event{{ID: "", Title: "Standup", Start: at(9, 0)}}, Event{Title: "Standup", Start: at(9, 0)}); got != "" {
		t.Errorf("expected no match for empty-id event, got %q", got)
	}
}

func TestParseEvents_TimedTopLevelItems(t *testing.T) {
	s := testService()
	raw := `{"items":[{"summary":"Standup","location":"Zoom","start":{"dateTime":"2026-03-10T15:00:00+07:00"},"end":{"dateTime":"2026-03-10T15:30:00+07:00"}}]}`
	evs := s.parseEvents(raw, "primary")
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	if evs[0].Title != "Standup" || evs[0].AllDay {
		t.Errorf("unexpected event: %+v", evs[0])
	}
	if evs[0].Start.Hour() != 15 || evs[0].Start.Minute() != 0 {
		t.Errorf("start not parsed to local 15:00: %v", evs[0].Start)
	}
}

func TestParseEvents_AllDayUnderDataWrapper(t *testing.T) {
	s := testService()
	raw := `{"data":{"events":[{"summary":"Holiday","start":{"date":"2026-03-11"},"end":{"date":"2026-03-12"}}]}}`
	evs := s.parseEvents(raw, "primary")
	if len(evs) != 1 || !evs[0].AllDay || evs[0].Title != "Holiday" {
		t.Fatalf("expected 1 all-day event, got %+v", evs)
	}
}

// TestParseEvents_ResponseDataWrapper covers the real runtime shape: ExecuteTool
// strips the outer "data", leaving events under a top-level "response_data".
// Before the fix this returned nothing, so the agent reported an empty calendar
// and the reconciler's dedup was blind (re-creating events every cycle).
func TestParseEvents_ResponseDataWrapper(t *testing.T) {
	s := testService()
	raw := `{"response_data":{"items":[{"id":"e1","summary":"Standup","start":{"dateTime":"2026-03-10T09:00:00+07:00"},"end":{"dateTime":"2026-03-10T09:30:00+07:00"}}]}}`
	evs := s.parseEvents(raw, "primary")
	if len(evs) != 1 || evs[0].Title != "Standup" || evs[0].ID != "e1" {
		t.Fatalf("expected 1 event from response_data wrapper, got %+v", evs)
	}
}

func TestParseEvents_SkipsUnparseableAndEmpty(t *testing.T) {
	s := testService()
	if evs := s.parseEvents(`not json`, "primary"); evs != nil {
		t.Errorf("invalid json should yield nil, got %+v", evs)
	}
	// An item with no usable start is skipped.
	raw := `{"items":[{"summary":"NoStart"},{"summary":"Good","start":{"dateTime":"2026-03-10T09:00:00+07:00"}}]}`
	evs := s.parseEvents(raw, "primary")
	if len(evs) != 1 || evs[0].Title != "Good" {
		t.Errorf("expected only the parseable event, got %+v", evs)
	}
}
