// Package calendar is the single, isolated integration point for the user's
// Composio-connected Google Calendar(s). It fans out over every ACTIVE
// googlecalendar connection (a user may connect multiple Google accounts) and
// normalizes Composio's provider-shaped responses into a small Event type.
//
// NOTE: the Composio Google-Calendar tool slugs, argument names, and response
// shapes below are best-effort and must be verified against a live Composio +
// Google account. All access is deliberately confined to this file so that risk
// stays contained; callers only depend on the stable Event type and methods.
package calendar

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/composio"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
)

const toolkitSlug = "googlecalendar"

// Composio Google Calendar tool slugs.
const (
	toolCreateEvent   = "GOOGLECALENDAR_CREATE_EVENT"
	toolDeleteEvent   = "GOOGLECALENDAR_DELETE_EVENT"
	toolListEvents    = "GOOGLECALENDAR_LIST_EVENTS"
	toolListCalendars = "GOOGLECALENDAR_LIST_CALENDARS"
)

// Event is a normalized calendar event (also used for create input).
type Event struct {
	ID       string // calendar event id (list results / create result)
	Title    string
	Location string
	Start    time.Time
	End      time.Time
	AllDay   bool
	Account  string // which connected account it came from (list only)
	Calendar string // which calendar within the account (list only)
}

// Service talks to the user's Composio-connected Google Calendars.
type Service struct {
	client   *composio.Client
	settings *settings.Service
	tz       *time.Location
	log      *slog.Logger
}

// New creates a calendar service.
func New(client *composio.Client, settingsSvc *settings.Service, tz *time.Location, log *slog.Logger) *Service {
	return &Service{client: client, settings: settingsSvc, tz: tz, log: log.With("component", "calendar")}
}

func (s *Service) apiKey(ctx context.Context) string {
	if s.settings == nil {
		return ""
	}
	key, err := s.settings.ComposioKey(ctx)
	if err != nil {
		return ""
	}
	return key
}

// Connections returns the user's ACTIVE googlecalendar connections (one per
// connected Google account).
func (s *Service) Connections(ctx context.Context, userID int64) []composio.Connection {
	key := s.apiKey(ctx)
	if key == "" {
		return nil
	}
	conns, err := s.client.ListConnections(ctx, key, strconv.FormatInt(userID, 10))
	if err != nil {
		s.log.Warn("list connections", "error", err)
		return nil
	}
	var out []composio.Connection
	for _, c := range conns {
		if c.ToolkitSlug == toolkitSlug && strings.EqualFold(c.Status, "ACTIVE") {
			out = append(out, c)
		}
	}
	return out
}

// HasCalendar reports whether the user has at least one connected Google Calendar.
func (s *Service) HasCalendar(ctx context.Context, userID int64) bool {
	return len(s.Connections(ctx, userID)) > 0
}

// PrimaryConnection returns the id of the user's first connected Google account.
func (s *Service) PrimaryConnection(ctx context.Context, userID int64) (string, bool) {
	conns := s.Connections(ctx, userID)
	if len(conns) == 0 {
		return "", false
	}
	return conns[0].ID, true
}

// CreateEvent adds an event to the given connection's primary calendar and
// returns its event id. rrule, when non-empty, makes it recurring (e.g.
// "RRULE:FREQ=WEEKLY;BYDAY=MO,WE").
//
// Before inserting, it checks the calendar for an identical event (same title
// and start time) and, if one already exists, returns that event's id instead
// of creating a duplicate. This is the safety net that keeps a flaky create
// response (where the new id can't be parsed) from re-creating the same event
// on every reconcile cycle.
func (s *Service) CreateEvent(ctx context.Context, userID int64, connID string, ev Event, rrule string) (string, error) {
	if id := s.existingEventID(ctx, userID, connID, ev); id != "" {
		s.log.Info("skip duplicate calendar event", "title", ev.Title, "start", ev.Start, "existing_id", id)
		return id, nil
	}
	dur := ev.End.Sub(ev.Start)
	if dur <= 0 {
		dur = time.Hour
	}
	args := map[string]any{
		"summary":                ev.Title,
		"start_datetime":         ev.Start.In(s.tz).Format("2006-01-02T15:04:05"),
		"event_duration_hour":    int(dur / time.Hour),
		"event_duration_minutes": int((dur % time.Hour) / time.Minute),
		"calendar_id":            "primary",
		"timezone":               s.tz.String(),
	}
	if ev.Location != "" {
		args["location"] = ev.Location
	}
	if rrule != "" {
		args["recurrence"] = []string{rrule}
	}
	argsJSON, _ := json.Marshal(args)
	raw, err := s.client.ExecuteTool(ctx, s.apiKey(ctx), toolCreateEvent, string(argsJSON), strconv.FormatInt(userID, 10), connID)
	if err != nil {
		return "", err
	}
	return parseCreatedEventID(raw), nil
}

// existingEventID looks for an event on the connection's primary calendar that
// matches ev by title and start time (to the minute) and returns its id, or ""
// if none is found. Recurring reminders are expanded (single_events) so the
// instance that falls on ev.Start is matched. Fail-soft: any lookup error
// returns "" (treated as "no duplicate found"), so a transient list failure
// never blocks a legitimate create.
func (s *Service) existingEventID(ctx context.Context, userID int64, connID string, ev Event) string {
	if ev.Title == "" || ev.Start.IsZero() {
		return ""
	}
	key := s.apiKey(ctx)
	if key == "" {
		return ""
	}
	// Narrow window centered on the target start so single_events expansion
	// returns the specific instance we're about to (re)create.
	from := ev.Start.Add(-time.Minute)
	to := ev.Start.Add(2 * time.Minute)
	uid := strconv.FormatInt(userID, 10)
	return matchEventID(s.eventsFor(ctx, key, uid, connID, "primary", from, to), ev)
}

// matchEventID returns the id of the first event in existing that has the same
// title (case-insensitive, trimmed) and start time (to the minute) as ev, or ""
// if none match.
func matchEventID(existing []Event, ev Event) string {
	want := strings.ToLower(strings.TrimSpace(ev.Title))
	for _, e := range existing {
		if strings.ToLower(strings.TrimSpace(e.Title)) != want {
			continue
		}
		if e.ID != "" && e.Start.Truncate(time.Minute).Equal(ev.Start.Truncate(time.Minute)) {
			return e.ID
		}
	}
	return ""
}

// DeleteEvent removes a mirrored event from a connection's primary calendar.
func (s *Service) DeleteEvent(ctx context.Context, userID int64, connID, eventID string) error {
	args, _ := json.Marshal(map[string]any{"calendar_id": "primary", "event_id": eventID})
	_, err := s.client.ExecuteTool(ctx, s.apiKey(ctx), toolDeleteEvent, string(args), strconv.FormatInt(userID, 10), connID)
	return err
}

// parseCreatedEventID tolerantly pulls the new event's id out of Composio's
// create response (top-level, or under a "data"/"response_data" wrapper).
func parseCreatedEventID(raw string) string {
	var w struct {
		ID   string `json:"id"`
		Data struct {
			ID           string `json:"id"`
			ResponseData struct {
				ID string `json:"id"`
			} `json:"response_data"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(raw), &w)
	return firstNonEmpty(w.ID, w.Data.ID, w.Data.ResponseData.ID)
}

// ListEvents returns events in [from, to) across every connected account and
// every calendar in each, sorted by start. Fail-soft: unreachable accounts or
// calendars are skipped (logged), never fatal.
func (s *Service) ListEvents(ctx context.Context, userID int64, from, to time.Time) []Event {
	conns := s.Connections(ctx, userID)
	if len(conns) == 0 {
		return nil
	}
	key := s.apiKey(ctx)
	uid := strconv.FormatInt(userID, 10)

	var out []Event
	for _, c := range conns {
		for _, cal := range s.calendarIDs(ctx, key, uid, c.ID) {
			out = append(out, s.eventsFor(ctx, key, uid, c.ID, cal, from, to)...)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// calendarIDs lists the calendars in a connected account, defaulting to
// ["primary"] when the account can't be enumerated.
func (s *Service) calendarIDs(ctx context.Context, key, uid, connID string) []string {
	raw, err := s.client.ExecuteTool(ctx, key, toolListCalendars, "{}", uid, connID)
	if err != nil {
		s.log.Warn("list calendars", "error", err)
		return []string{"primary"}
	}
	var w struct {
		Items []calItem `json:"items"`
		Data  struct {
			Items []calItem `json:"items"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(raw), &w)
	items := w.Items
	if len(items) == 0 {
		items = w.Data.Items
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		if it.ID != "" {
			ids = append(ids, it.ID)
		}
	}
	if len(ids) == 0 {
		return []string{"primary"}
	}
	return ids
}

func (s *Service) eventsFor(ctx context.Context, key, uid, connID, calID string, from, to time.Time) []Event {
	args, _ := json.Marshal(map[string]any{
		"calendar_id":   calID,
		"timeMin":       from.In(s.tz).Format(time.RFC3339),
		"timeMax":       to.In(s.tz).Format(time.RFC3339),
		"single_events": true,
		"order_by":      "startTime",
		"max_results":   50,
		"timezone":      s.tz.String(),
	})
	raw, err := s.client.ExecuteTool(ctx, key, toolListEvents, string(args), uid, connID)
	if err != nil {
		s.log.Warn("list events", "calendar", calID, "error", err)
		return nil
	}
	return s.parseEvents(raw, calID)
}

type calItem struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

type gEvent struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Location string `json:"location"`
	Start    gTime  `json:"start"`
	End      gTime  `json:"end"`
}

type gTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
}

// parseEvents tolerantly extracts an events array from Composio's response,
// which may nest under "items"/"events" and/or a "data" wrapper.
func (s *Service) parseEvents(raw, calID string) []Event {
	var w struct {
		Items  []gEvent `json:"items"`
		Events []gEvent `json:"events"`
		Data   struct {
			Items  []gEvent `json:"items"`
			Events []gEvent `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		s.log.Warn("parse events", "error", err)
		return nil
	}
	items := firstNonEmptyEvents(w.Items, w.Events, w.Data.Items, w.Data.Events)
	out := make([]Event, 0, len(items))
	for _, it := range items {
		start, allDay, ok := s.parseGTime(it.Start)
		if !ok {
			continue
		}
		end, _, _ := s.parseGTime(it.End)
		out = append(out, Event{
			ID:       it.ID,
			Title:    strings.TrimSpace(firstNonEmpty(it.Summary, "(no title)")),
			Location: it.Location,
			Start:    start,
			End:      end,
			AllDay:   allDay,
			Calendar: calID,
		})
	}
	return out
}

func (s *Service) parseGTime(t gTime) (time.Time, bool, bool) {
	if t.DateTime != "" {
		if v, err := time.Parse(time.RFC3339, t.DateTime); err == nil {
			return v.In(s.tz), false, true
		}
	}
	if t.Date != "" {
		if v, err := time.ParseInLocation("2006-01-02", t.Date, s.tz); err == nil {
			return v, true, true
		}
	}
	return time.Time{}, false, false
}

func firstNonEmptyEvents(lists ...[]gEvent) []gEvent {
	for _, l := range lists {
		if len(l) > 0 {
			return l
		}
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
