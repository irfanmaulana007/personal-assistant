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
	"fmt"
	"log/slog"
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
	toolListEvents    = "GOOGLECALENDAR_LIST_EVENTS"
	toolListCalendars = "GOOGLECALENDAR_LIST_CALENDARS"
)

// Event is a normalized calendar event (also used for create input).
type Event struct {
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

// CreateEvent adds a timed event to the user's primary (first connected) account.
func (s *Service) CreateEvent(ctx context.Context, userID int64, ev Event) error {
	conns := s.Connections(ctx, userID)
	if len(conns) == 0 {
		return fmt.Errorf("no connected google calendar")
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
	argsJSON, _ := json.Marshal(args)
	_, err := s.client.ExecuteTool(ctx, s.apiKey(ctx), toolCreateEvent, string(argsJSON), strconv.FormatInt(userID, 10), conns[0].ID)
	return err
}
