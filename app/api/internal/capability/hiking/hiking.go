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

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
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
	case intent.ActionHikeUpdate:
		return h.updateHike(ctx, result)
	case intent.ActionHikeDelete:
		return h.deleteHike(ctx, result)
	case intent.ActionHikeParticipants:
		return h.listParticipants(ctx, result)
	case intent.ActionHikeParticipantUpdate:
		return h.updateParticipant(ctx, result)
	case intent.ActionHikeParticipantMerge:
		return h.mergeParticipants(ctx, result)
	default:
		return "I can log a hike, summarize your trips, edit a logged hike (e.g. fix its date), delete a hike by its number, or manage your hiking participants (rename or merge them).", nil
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

	// No date given → leave it unrecorded (nil) rather than fabricating today.
	// The user can fill it in later with hike_update.
	var hikedOn *time.Time
	if d := strings.TrimSpace(e["date"]); d != "" {
		if t, err := capability.ParseTime(d, h.timezone); err == nil {
			hikedOn = &t
		}
	}

	days := atoi(e["days"])
	nights := atoi(e["nights"])
	// A multi-day trip means the user necessarily stayed overnight, so infer
	// camped=true instead of asking. Single-day hikes (days<=1, nights==0)
	// keep the explicitly-provided value so the camping question still applies.
	camped := parseBool(e["camped"]) || days > 1 || nights > 0

	hike := &store.Hike{
		MountainID:  mountain.ID,
		Camped:      camped,
		UpTrackID:   upTrackID,
		DownTrackID: downTrackID,
		Days:        days,
		Nights:      nights,
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

// hikeDate renders a hike's date in the user's timezone, or a placeholder when
// the hike has no recorded date (hiked_on is null).
func (h *Handler) hikeDate(t *time.Time, layout string) string {
	if t == nil {
		return "no date set"
	}
	return t.In(h.timezone).Format(layout)
}

func (h *Handler) formatLogged(mountain, up, down string, hike *store.Hike, people []string, notes []string) string {
	var sb strings.Builder
	if hike.HikedOn != nil {
		sb.WriteString(fmt.Sprintf("Logged hike to *%s* on %s.", mountain, hike.HikedOn.In(h.timezone).Format("Mon, Jan 2 2006")))
	} else {
		sb.WriteString(fmt.Sprintf("Logged hike to *%s* (no date set — say the date and I'll add it).", mountain))
	}
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
		sb.WriteString(fmt.Sprintf("\n#%d • *%s* — %s", hk.ID, hk.Mountain, h.hikeDate(hk.HikedOn, "Jan 2 2006")))
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
	sb.WriteString("\n\n_Tip: to remove a hike, say \"delete hike\" with its number (e.g. #" + strconv.FormatInt(hikes[0].ID, 10) + ")._")
	return sb.String(), nil
}

// updateHike edits an already-logged hike, identified by its #id. Only the
// fields the caller supplies are changed; every other field keeps its current
// value (loaded via GetHike), so fixing just the date can't clobber the
// mountain, trails, duration, or participants. This is the non-destructive
// alternative to delete + re-log. Participant membership is intentionally out
// of scope here — that is managed by the participant tools.
func (h *Handler) updateHike(ctx context.Context, result *intent.ParseResult) (string, error) {
	userID := authctx.UserID(ctx)
	e := result.Entities

	raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(e["id"]), "#"))
	if raw == "" {
		return "Which hike should I edit? Give me its number from the summary — e.g. _update hike 42 to 18 March 2023_. Run hike_summary first if you're not sure.", nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return "Please give me a valid hike number to edit (the # shown in the summary).", nil
	}

	existing, err := h.store.GetHike(ctx, userID, id)
	if err != nil {
		return "", fmt.Errorf("get hike: %w", err)
	}
	if existing == nil {
		return fmt.Sprintf("I couldn't find a hike numbered #%d. Run hike_summary to see your hikes and their numbers.", id), nil
	}

	// Start from the current values; override only what the caller supplied.
	updated := existing.Hike
	var notes []string
	changed := false
	touchedDuration := false

	if v, ok := e["mountain"]; ok && strings.TrimSpace(v) != "" {
		mountain, reused, err := h.resolveMountain(ctx, userID, v)
		if err != nil {
			return "", err
		}
		if reused && !equalName(mountain.Name, v) {
			notes = append(notes, fmt.Sprintf("matched %q to your existing mountain *%s*", v, mountain.Name))
		}
		updated.MountainID = mountain.ID
		changed = true
	}

	if v, ok := e["date"]; ok && strings.TrimSpace(v) != "" {
		t, err := capability.ParseTime(strings.TrimSpace(v), h.timezone)
		if err != nil {
			return fmt.Sprintf("I couldn't understand the date %q. Try something like '18 March 2023', 'Aug 2', or 'last Saturday'.", v), nil
		}
		updated.HikedOn = &t
		changed = true
	}

	if v, ok := e["up_track"]; ok {
		tid, tname, reused, err := h.resolveTrack(ctx, userID, updated.MountainID, v)
		if err != nil {
			return "", err
		}
		if reused {
			notes = append(notes, fmt.Sprintf("reused up trail *%s*", tname))
		}
		updated.UpTrackID = tid
		changed = true
	}
	if v, ok := e["down_track"]; ok {
		tid, tname, reused, err := h.resolveTrack(ctx, userID, updated.MountainID, v)
		if err != nil {
			return "", err
		}
		if reused {
			notes = append(notes, fmt.Sprintf("reused down trail *%s*", tname))
		}
		updated.DownTrackID = tid
		changed = true
	}

	if v, ok := e["days"]; ok {
		updated.Days = atoi(v)
		changed = true
		touchedDuration = true
	}
	if v, ok := e["nights"]; ok {
		updated.Nights = atoi(v)
		changed = true
		touchedDuration = true
	}
	if v, ok := e["camped"]; ok {
		updated.Camped = parseBool(v)
		changed = true
	} else if touchedDuration && (updated.Days > 1 || updated.Nights > 0) {
		// Mirror hike_log: a multi-day/overnight trip implies camping, so infer
		// it when the duration was edited and camping wasn't given explicitly.
		updated.Camped = true
	}

	if !changed {
		return fmt.Sprintf("Tell me what to change about hike #%d — for example its date, mountain, trails, or duration.", id), nil
	}

	if err := h.store.UpdateHike(ctx, userID, id, &updated); err != nil {
		return "", fmt.Errorf("update hike: %w", err)
	}

	// Read-after-write: confirm the edit persisted before reporting it, and
	// echo the stored names/participants so the reply reflects the real row.
	saved, err := h.store.GetHike(ctx, userID, id)
	if err != nil {
		return "", fmt.Errorf("verify hike updated: %w", err)
	}
	if saved == nil {
		return "", fmt.Errorf("verify hike updated: hike not found after update")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Updated hike #%d — *%s* (%s).", saved.ID, saved.Mountain, h.hikeDate(saved.HikedOn, "Mon, Jan 2 2006")))
	if saved.UpTrack != "" {
		sb.WriteString("\nUp: " + saved.UpTrack)
	}
	if saved.DownTrack != "" {
		sb.WriteString("\nDown: " + saved.DownTrack)
	}
	if saved.Days > 0 || saved.Nights > 0 {
		sb.WriteString(fmt.Sprintf("\nDuration: %dD/%dN", saved.Days, saved.Nights))
	}
	if saved.Camped {
		sb.WriteString("\nCamped: yes")
	}
	if len(saved.Participants) > 0 {
		sb.WriteString("\nWith: " + strings.Join(saved.Participants, ", "))
	}
	if len(notes) > 0 {
		sb.WriteString("\n\n_(" + strings.Join(notes, "; ") + ")_")
	}
	return sb.String(), nil
}

// deleteHike removes one or more logged hikes by their number (the #id shown in
// the summary). Accepts a single id or a comma-separated list so several
// duplicates can be cleared in one go. Each id is verified with GetHike before
// deletion so the reply can name what was removed and flag any id that didn't
// match one of the user's hikes.
func (h *Handler) deleteHike(ctx context.Context, result *intent.ParseResult) (string, error) {
	userID := authctx.UserID(ctx)
	raw := strings.TrimSpace(result.Entities["id"])
	if raw == "" {
		return "Which hike should I delete? Give me its number from the summary — e.g. _delete hike 42_. Run hike_summary first if you're not sure.", nil
	}

	// Parse the requested ids, tolerating a leading '#' and repeated entries.
	var ids []int64
	seen := map[int64]bool{}
	var invalid []string
	for _, part := range strings.Split(raw, ",") {
		p := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "#"))
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			invalid = append(invalid, part)
			continue
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return "Please give me a valid hike number to delete (the # shown in the summary).", nil
	}

	var deleted []string
	var notFound []int64
	for _, id := range ids {
		hike, err := h.store.GetHike(ctx, userID, id)
		if err != nil {
			return "", fmt.Errorf("get hike: %w", err)
		}
		if hike == nil {
			notFound = append(notFound, id)
			continue
		}
		if err := h.store.DeleteHike(ctx, userID, id); err != nil {
			return "", fmt.Errorf("delete hike: %w", err)
		}
		deleted = append(deleted, fmt.Sprintf("#%d *%s* (%s)", id, hike.Mountain, h.hikeDate(hike.HikedOn, "Jan 2 2006")))
	}

	var sb strings.Builder
	switch len(deleted) {
	case 0:
		sb.WriteString("No hikes deleted.")
	case 1:
		sb.WriteString("Deleted hike " + deleted[0] + ".")
	default:
		sb.WriteString(fmt.Sprintf("Deleted %d hikes:\n• %s", len(deleted), strings.Join(deleted, "\n• ")))
	}
	if len(notFound) > 0 {
		nums := make([]string, len(notFound))
		for i, id := range notFound {
			nums[i] = "#" + strconv.FormatInt(id, 10)
		}
		sb.WriteString("\n\nNo hike found for: " + strings.Join(nums, ", ") + ".")
	}
	if len(invalid) > 0 {
		sb.WriteString("\n\nIgnored (not a number): " + strings.Join(invalid, ", ") + ".")
	}
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

// resolveHiker returns the user's existing participant whose canonical name or
// one of whose nicknames exactly matches name (case/whitespace-insensitive), or
// creates a new participant with the name saved exactly as given.
//
// Unlike mountains and trails, participant names are never fuzzy-folded: two
// different people can have near-identical names ("Ali" vs "Abi"), so the
// auto-matcher must not silently remap one onto the other. Intentional aliases
// are captured as nicknames (set via update, or added when merging duplicates)
// so a known alias still resolves to the right person. reused is true when an
// existing record was matched.
func (h *Handler) resolveHiker(ctx context.Context, userID int64, name string) (*store.Hiker, bool, error) {
	existing, err := h.store.ListHikers(ctx, userID)
	if err != nil {
		return nil, false, fmt.Errorf("list hikers: %w", err)
	}
	if p := matchHiker(existing, name); p != nil {
		return p, true, nil
	}
	p, err := h.store.CreateHiker(ctx, userID, strings.TrimSpace(name))
	return p, false, err
}

// matchHiker returns the participant whose canonical name or one of whose
// nicknames equals name after normalization, or nil. Exact (normalized) match
// only — no fuzzy edit-distance folding.
func matchHiker(existing []store.Hiker, name string) *store.Hiker {
	for i := range existing {
		p := &existing[i]
		if equalName(p.Name, name) {
			return p
		}
		for _, nick := range p.Nicknames {
			if equalName(nick, name) {
				return p
			}
		}
	}
	return nil
}

// listParticipants shows the user's saved participants and their nicknames.
func (h *Handler) listParticipants(ctx context.Context, _ *intent.ParseResult) (string, error) {
	hikers, err := h.store.ListHikers(ctx, authctx.UserID(ctx))
	if err != nil {
		return "", fmt.Errorf("list hikers: %w", err)
	}
	if len(hikers) == 0 {
		return "You haven't logged any hiking participants yet.", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Hiking participants* (%d):", len(hikers)))
	for _, p := range hikers {
		sb.WriteString("\n• " + p.Name)
		if len(p.Nicknames) > 0 {
			sb.WriteString(" _(aka " + strings.Join(p.Nicknames, ", ") + ")_")
		}
	}
	return sb.String(), nil
}

// updateParticipant renames a participant and/or sets their nicknames. The new
// name is stored exactly as given — no auto-matching is applied to it — so this
// is how a wrongly-recorded name gets corrected.
func (h *Handler) updateParticipant(ctx context.Context, result *intent.ParseResult) (string, error) {
	userID := authctx.UserID(ctx)
	e := result.Entities
	who := strings.TrimSpace(e["name"])
	if who == "" {
		return "Which participant should I update? Tell me their current name.", nil
	}
	existing, err := h.store.ListHikers(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("list hikers: %w", err)
	}
	target := matchHiker(existing, who)
	if target == nil {
		return fmt.Sprintf("I couldn't find a hiking participant matching %q.", who), nil
	}

	newName := target.Name
	if n := strings.TrimSpace(e["new_name"]); n != "" {
		newName = n
	}
	nicknames := target.Nicknames
	if _, ok := e["nicknames"]; ok {
		nicknames = splitList(e["nicknames"])
	}
	// A participant is never a nickname of itself.
	nicknames = dropEqual(nicknames, newName)

	updated, err := h.store.UpdateHiker(ctx, userID, target.ID, newName, nicknames)
	if err != nil {
		return "", fmt.Errorf("update hiker: %w", err)
	}
	if updated == nil {
		return fmt.Sprintf("I couldn't find a hiking participant matching %q.", who), nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Updated participant *%s*.", updated.Name))
	if len(updated.Nicknames) > 0 {
		sb.WriteString(" Nicknames: " + strings.Join(updated.Nicknames, ", ") + ".")
	}
	return sb.String(), nil
}

// mergeParticipants folds a duplicate participant into the one to keep,
// reattributing every hike and preserving the absorbed name as a nickname.
func (h *Handler) mergeParticipants(ctx context.Context, result *intent.ParseResult) (string, error) {
	userID := authctx.UserID(ctx)
	e := result.Entities
	fromName := strings.TrimSpace(e["from"])
	intoName := strings.TrimSpace(e["into"])
	if fromName == "" || intoName == "" {
		return "To merge, tell me which participant to merge *from* (the duplicate) and *into* (the one to keep).", nil
	}
	existing, err := h.store.ListHikers(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("list hikers: %w", err)
	}
	source := matchHiker(existing, fromName)
	target := matchHiker(existing, intoName)
	if source == nil {
		return fmt.Sprintf("I couldn't find a participant matching %q to merge.", fromName), nil
	}
	if target == nil {
		return fmt.Sprintf("I couldn't find a participant matching %q to merge into.", intoName), nil
	}
	if source.ID == target.ID {
		return fmt.Sprintf("%q and %q are already the same participant.", fromName, intoName), nil
	}
	merged, err := h.store.MergeHikers(ctx, userID, target.ID, source.ID)
	if err != nil {
		return "", fmt.Errorf("merge hikers: %w", err)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Merged *%s* into *%s*.", source.Name, merged.Name))
	if len(merged.Nicknames) > 0 {
		sb.WriteString(" Nicknames: " + strings.Join(merged.Nicknames, ", ") + ".")
	}
	return sb.String(), nil
}

func equalName(a, b string) bool { return normalize(a) == normalize(b) }

// splitList parses a comma-separated string into trimmed, non-empty items.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// dropEqual removes any item equal (normalized) to name.
func dropEqual(items []string, name string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if !equalName(it, name) {
			out = append(out, it)
		}
	}
	return out
}

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
