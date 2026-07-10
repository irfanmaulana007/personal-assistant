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
		`{"id":"abc"}`:                            "abc",
		`{"data":{"id":"def"}}`:                   "def",
		`{"data":{"response_data":{"id":"ghi"}}}`: "ghi",
		`{"foo":1}`:                               "",
		`not json`:                                "",
	}
	for raw, want := range cases {
		if got := parseCreatedEventID(raw); got != want {
			t.Errorf("parseCreatedEventID(%q) = %q, want %q", raw, got, want)
		}
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
