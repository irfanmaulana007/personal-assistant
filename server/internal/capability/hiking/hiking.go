// Package hiking implements the Hiking Tracker skill: logging detailed hiking
// trips (mountain, up/down trails, camping, days/nights, date, participants)
// while reusing similar existing names to avoid typo-created duplicates.
package hiking

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Handler logs and summarizes hiking trips.
type Handler struct {
	store    store.Store
	timezone *time.Location
	log      *slog.Logger
}

// New creates a hiking handler.
func New(s store.Store, timezone *time.Location, log *slog.Logger) *Handler {
	return &Handler{store: s, timezone: timezone, log: log.With("component", "hiking")}
}

func (h *Handler) Name() string { return "hiking" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityHiking
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionHikeLog:
		return h.logHike(ctx, result)
	case intent.ActionHikeSummary:
		return h.summary(ctx, result)
	default:
		return "I can log a hike or summarize your hiking trips.", nil
	}
}

func (h *Handler) logHike(ctx context.Context, result *intent.ParseResult) (string, error) {
	userID := authctx.UserID(ctx)
	e := result.Entities

	mountainName := strings.TrimSpace(e["mountain"])
	if mountainName == "" {
		return "Which mountain did you hike?", nil
	}

	var notes []string

	// Resolve (or create) the mountain, folding in near-duplicate spellings.
	mountain, reusedMountain, err := h.resolveMountain(ctx, userID, mountainName)
	if err != nil {
		return "", err
	}
	if reusedMountain && !equalName(mountain.Name, mountainName) {
		notes = append(notes, fmt.Sprintf("matched %q to your existing mountain *%s*", mountainName, mountain.Name))
	}

	upTrackID, upName, upReused, err := h.resolveTrack(ctx, userID, mountain.ID, e["up_track"])
	if err != nil {
		return "", err
	}
	if upReused {
		notes = append(notes, fmt.Sprintf("reused up trail *%s*", upName))
	}
	downTrackID, downName, downReused, err := h.resolveTrack(ctx, userID, mountain.ID, e["down_track"])
	if err != nil {
		return "", err
	}
	if downReused {
		notes = append(notes, fmt.Sprintf("reused down trail *%s*", downName))
	}

	hikedOn := time.Now()
	if d := strings.TrimSpace(e["date"]); d != "" {
		if t, err := capability.ParseTime(d, h.timezone); err == nil {
			hikedOn = t
		}
	}

	hike := &store.Hike{
		MountainID:  mountain.ID,
		Camped:      parseBool(e["camped"]),
		UpTrackID:   upTrackID,
		DownTrackID: downTrackID,
		Days:        atoi(e["days"]),
		Nights:      atoi(e["nights"]),
		HikedOn:     hikedOn,
	}
	hikeID, err := h.store.CreateHike(ctx, userID, hike)
	if err != nil {
		return "", fmt.Errorf("create hike: %w", err)
	}

	// Resolve participants (fold near-duplicates), attach to the hike.
	var people []string
	for _, raw := range strings.Split(e["participants"], ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		hiker, reused, err := h.resolveHiker(ctx, userID, name)
		if err != nil {
			return "", err
		}
		if reused && !equalName(hiker.Name, name) {
			notes = append(notes, fmt.Sprintf("matched %q to *%s*", name, hiker.Name))
		}
		if err := h.store.AddHikeParticipant(ctx, hikeID, hiker.ID); err != nil {
			return "", fmt.Errorf("add participant: %w", err)
		}
		people = append(people, hiker.Name)
	}

	// Read-after-write: confirm the hike (with its participants) actually
	// persisted before telling the user it was logged.
	if saved, err := h.store.GetHike(ctx, userID, hikeID); err != nil {
		return "", fmt.Errorf("verify hike saved: %w", err)
	} else if saved == nil {
		return "", fmt.Errorf("verify hike saved: hike not found after create")
	}

	return h.formatLogged(mountain.Name, upName, downName, hike, people, notes), nil
}

func (h *Handler) formatLogged(mountain, up, down string, hike *store.Hike, people []string, notes []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Logged hike to *%s* on %s.", mountain, hike.HikedOn.In(h.timezone).Format("Mon, Jan 2 2006")))
	if up != "" {
		sb.WriteString("\nUp: " + up)
	}
	if down != "" {
		sb.WriteString("\nDown: " + down)
	}
	if hike.Days > 0 || hike.Nights > 0 {
		sb.WriteString(fmt.Sprintf("\nDuration: %dD/%dN", hike.Days, hike.Nights))
	}
	if hike.Camped {
		sb.WriteString("\nCamped: yes")
	}
	if len(people) > 0 {
		sb.WriteString("\nWith: " + strings.Join(people, ", "))
	}
	if len(notes) > 0 {
		sb.WriteString("\n\n_(" + strings.Join(notes, "; ") + ")_")
	}
	return sb.String()
}

func (h *Handler) summary(ctx context.Context, result *intent.ParseResult) (string, error) {
	limit := atoi(result.Entities["limit"])
	if limit <= 0 {
		limit = 10
	}
	hikes, err := h.store.ListHikes(ctx, authctx.UserID(ctx), limit)
	if err != nil {
		return "", fmt.Errorf("list hikes: %w", err)
	}
	if len(hikes) == 0 {
		return "You haven't logged any hikes yet. Tell me about your next trip and I'll track it!", nil
	}

	var nights int
	mountains := map[string]bool{}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Your hikes* (%d):\n", len(hikes)))
	for _, hk := range hikes {
		nights += hk.Nights
		mountains[hk.Mountain] = true
		sb.WriteString(fmt.Sprintf("\n• *%s* — %s", hk.Mountain, hk.HikedOn.In(h.timezone).Format("Jan 2 2006")))
		if hk.UpTrack != "" || hk.DownTrack != "" {
			sb.WriteString(fmt.Sprintf(" (up: %s, down: %s)", orDash(hk.UpTrack), orDash(hk.DownTrack)))
		}
		if hk.Days > 0 || hk.Nights > 0 {
			sb.WriteString(fmt.Sprintf(" %dD/%dN", hk.Days, hk.Nights))
		}
		if hk.Camped {
			sb.WriteString(" ⛺")
		}
		if len(hk.Participants) > 0 {
			sb.WriteString(" — with " + strings.Join(hk.Participants, ", "))
		}
	}
	sb.WriteString(fmt.Sprintf("\n\n%d mountain(s), %d night(s) on the trail.", len(mountains), nights))
	return sb.String(), nil
}

// resolveMountain returns the user's existing mountain matching name, or creates
// a new one. reused is true when an existing record was matched.
func (h *Handler) resolveMountain(ctx context.Context, userID int64, name string) (*store.Mountain, bool, error) {
	existing, err := h.store.ListMountains(ctx, userID)
	if err != nil {
		return nil, false, fmt.Errorf("list mountains: %w", err)
	}
	names := make([]string, len(existing))
	for i, m := range existing {
		names[i] = m.Name
	}
	if match := bestMatch(names, name); match != "" {
		for _, m := range existing {
			if m.Name == match {
				return &m, true, nil
			}
		}
	}
	m, err := h.store.CreateMountain(ctx, userID, strings.TrimSpace(name))
	return m, false, err
}

// resolveTrack resolves a trail within a mountain. An empty name yields (0, "", false).
func (h *Handler) resolveTrack(ctx context.Context, userID, mountainID int64, name string) (int64, string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, "", false, nil
	}
	existing, err := h.store.ListTracks(ctx, userID, mountainID)
	if err != nil {
		return 0, "", false, fmt.Errorf("list tracks: %w", err)
	}
	names := make([]string, len(existing))
	for i, t := range existing {
		names[i] = t.Name
	}
	if match := bestMatch(names, name); match != "" {
		for _, t := range existing {
			if t.Name == match {
				return t.ID, t.Name, !equalName(t.Name, name), nil
			}
		}
	}
	t, err := h.store.CreateTrack(ctx, userID, mountainID, name)
	if err != nil {
		return 0, "", false, err
	}
	return t.ID, t.Name, false, nil
}

// resolveHiker returns the user's existing participant matching name, or creates one.
func (h *Handler) resolveHiker(ctx context.Context, userID int64, name string) (*store.Hiker, bool, error) {
	existing, err := h.store.ListHikers(ctx, userID)
	if err != nil {
		return nil, false, fmt.Errorf("list hikers: %w", err)
	}
	names := make([]string, len(existing))
	for i, p := range existing {
		names[i] = p.Name
	}
	if match := bestMatch(names, name); match != "" {
		for _, p := range existing {
			if p.Name == match {
				return &p, true, nil
			}
		}
	}
	p, err := h.store.CreateHiker(ctx, userID, strings.TrimSpace(name))
	return p, false, err
}

func equalName(a, b string) bool { return normalize(a) == normalize(b) }

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "y", "1":
		return true
	}
	return false
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
