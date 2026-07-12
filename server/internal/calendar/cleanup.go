package calendar

import (
	"context"
	"encoding/json"
	"strconv"
	"time"
)

// This file supports the one-off duplicate-cleanup maintenance command
// (server/cmd/calcleanup). Unlike ListEvents (which expands recurrences via
// single_events=true), ListMasters returns each recurring series ONCE as its
// master, so the flood of duplicate mirror masters can be told apart and pruned.

// MasterEvent is an unexpanded calendar event (a recurring series appears once,
// as its master). Recurrence is the joined RRULE lines, "" for a one-off.
type MasterEvent struct {
	ID         string
	Title      string
	Start      time.Time
	Recurrence string
	Account    string
	Calendar   string
}

// ListMasters returns every event (recurrences NOT expanded) in [from,to) across
// all connected accounts and calendars, paging through every result. It is for
// maintenance tooling only, never the request hot path.
func (s *Service) ListMasters(ctx context.Context, userID int64, from, to time.Time) ([]MasterEvent, error) {
	conns := s.Connections(ctx, userID)
	if len(conns) == 0 {
		return nil, nil
	}
	key := s.apiKey(ctx)
	uid := strconv.FormatInt(userID, 10)
	var out []MasterEvent
	for _, c := range conns {
		for _, cal := range s.calendarIDs(ctx, key, uid, c.ID) {
			evs, err := s.mastersFor(ctx, key, uid, c.ID, cal, from, to)
			if err != nil {
				return nil, err
			}
			out = append(out, evs...)
		}
	}
	return out, nil
}

// ClearAll deletes EVERY event across all connected Google accounts and
// calendars within a wide window centered on now (recurring series are removed
// once, via their master, which deletes all their instances). It returns how
// many events were deleted and how many delete calls failed.
//
// This is a destructive recovery action, surfaced to the user so they can wipe a
// runaway flood of duplicate events that the agent can no longer clean up on its
// own and that Google Calendar's own UI refuses to bulk-delete.
func (s *Service) ClearAll(ctx context.Context, userID int64) (deleted, failed int, err error) {
	now := time.Now().In(s.tz)
	// A very wide window so long-past and far-future events are both swept up.
	from := now.AddDate(-5, 0, 0)
	to := now.AddDate(5, 0, 0)
	masters, err := s.ListMasters(ctx, userID, from, to)
	if err != nil {
		return 0, 0, err
	}
	for _, m := range masters {
		if e := s.deleteInCalendar(ctx, userID, m.Account, m.Calendar, m.ID); e != nil {
			failed++
			s.log.Warn("clear-all delete failed", "id", m.ID, "title", m.Title, "error", e)
			continue
		}
		deleted++
	}
	s.log.Info("cleared calendar events", "user", userID, "deleted", deleted, "failed", failed)
	return deleted, failed, nil
}

func (s *Service) mastersFor(ctx context.Context, key, uid, connID, calID string, from, to time.Time) ([]MasterEvent, error) {
	var out []MasterEvent
	pageToken := ""
	for page := 0; page < 200; page++ { // safety cap on pagination
		args := map[string]any{
			"calendar_id":   calID,
			"time_min":      from.In(s.tz).Format(time.RFC3339),
			"time_max":      to.In(s.tz).Format(time.RFC3339),
			"single_events": false,
			"max_results":   250,
			"timezone":      s.tz.String(),
		}
		if pageToken != "" {
			args["page_token"] = pageToken
		}
		argsJSON, _ := json.Marshal(args)
		raw, err := s.client.ExecuteTool(ctx, key, toolListEvents, string(argsJSON), uid, connID)
		if err != nil {
			return nil, err
		}
		evs, next := s.parseMasters(raw, connID, calID)
		out = append(out, evs...)
		if next == "" || next == pageToken {
			break
		}
		pageToken = next
	}
	return out, nil
}

type gMaster struct {
	ID         string   `json:"id"`
	Summary    string   `json:"summary"`
	Status     string   `json:"status"`
	Start      gTime    `json:"start"`
	Recurrence []string `json:"recurrence"`
}

// parseMasters extracts unexpanded events plus the next page token from
// Composio's list response, tolerating the same wrapper levels as parseEvents
// (top-level, "response_data", or an un-unwrapped "data").
func (s *Service) parseMasters(raw, account, calID string) ([]MasterEvent, string) {
	type holder struct {
		Items         []gMaster `json:"items"`
		Events        []gMaster `json:"events"`
		NextPageToken string    `json:"nextPageToken"`
		NextPageSnake string    `json:"next_page_token"`
		ResponseData  struct {
			Items         []gMaster `json:"items"`
			Events        []gMaster `json:"events"`
			NextPageToken string    `json:"nextPageToken"`
			NextPageSnake string    `json:"next_page_token"`
		} `json:"response_data"`
	}
	var w struct {
		holder
		Data holder `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		s.log.Warn("parse masters", "error", err)
		return nil, ""
	}
	items := pickList(
		w.Items, w.Events, w.ResponseData.Items, w.ResponseData.Events,
		w.Data.Items, w.Data.Events, w.Data.ResponseData.Items, w.Data.ResponseData.Events,
	)
	token := firstNonEmpty(
		w.NextPageToken, w.NextPageSnake, w.ResponseData.NextPageToken, w.ResponseData.NextPageSnake,
		w.Data.NextPageToken, w.Data.NextPageSnake, w.Data.ResponseData.NextPageToken, w.Data.ResponseData.NextPageSnake,
	)
	out := make([]MasterEvent, 0, len(items))
	for _, it := range items {
		if it.Status == "cancelled" || it.ID == "" {
			continue
		}
		start, _, ok := s.parseGTime(it.Start)
		if !ok {
			continue
		}
		rec := ""
		if len(it.Recurrence) > 0 {
			rec = joinStrings(it.Recurrence, "\n")
		}
		out = append(out, MasterEvent{
			ID:         it.ID,
			Title:      it.Summary,
			Start:      start,
			Recurrence: rec,
			Account:    account,
			Calendar:   calID,
		})
	}
	return out, token
}

func pickList[T any](lists ...[]T) []T {
	for _, l := range lists {
		if len(l) > 0 {
			return l
		}
	}
	return nil
}

func joinStrings(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
