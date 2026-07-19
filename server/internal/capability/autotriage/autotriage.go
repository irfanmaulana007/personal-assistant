// Package autotriage implements the Auto-Triage skill: it scans the assistant's
// own recent runs for things it couldn't handle automatically — agent errors and
// low-quality (poorly judged) replies — groups them into recurring patterns, and
// helps the assistant act on them by filing a bug card on the Trello Issue board
// (with duplicate detection) and by refining the prompts of the skills that keep
// underperforming.
//
// It is meant to run unattended from the nightly triage routine, but every step
// is an ordinary tool the agent can also invoke on request. The Trello Issue
// board / Bug list ids are fixed to the user's "Personal Assistant" workspace,
// mirroring the trello capability; credentials are resolved per call from
// encrypted settings, and a missing credential is reported back as plain text so
// the model can tell the user to configure it.
package autotriage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
	"github.com/irfanmaulana007/personal-assistant/server/internal/trello"
)

// Fixed ids for the "Personal Assistant" workspace Issue board and its Bug list —
// the same ids the trello capability files bug reports on. Auto-triage files its
// bug cards here.
const (
	boardIssue = "6a54edaae21957ab935c81f6"
	listBug    = "6a54edaae21957ab935c820f"
)

const (
	defaultHours = 24
	scanLimit    = 100
	maxTextChars = 1200 // truncate long inputs/errors in the report
	maxGroups    = 30

	selfKey  = "auto_triage" // never triage-tune the triage skill itself
	tunerKey = "self_tuning" // nor the self-tuner (owned by its own loop)

	// signatureTag prefixes the stable pattern signature embedded in every
	// triage-filed bug card, so a later run can recognise an already-filed
	// recurring failure and comment on it instead of creating a duplicate.
	signatureTag = "Triage-Signature: "
)

const notConfiguredMsg = "Trello is not configured — no Trello API key/token has been set. Ask the user to add their Trello API key and token on the Integrations page."

// routineSources are excluded from the scan so triage never triages its own
// scheduled runs (or the self-tuner's / morning briefing's).
var routineSources = []string{"nightly_triage", "start_of_day", "end_of_day"}

// Handler answers auto-triage tool calls (scan failures, file a bug, improve a
// skill prompt).
type Handler struct {
	store    store.Store
	client   *trello.Client
	settings *settings.Service
	log      *slog.Logger
}

// New creates an auto-triage capability handler.
func New(s store.Store, client *trello.Client, settingsSvc *settings.Service, log *slog.Logger) *Handler {
	return &Handler{store: s, client: client, settings: settingsSvc, log: log.With("component", "autotriage")}
}

func (h *Handler) Name() string { return "auto_triage" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityAutoTriage
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionAutoTriageScan:
		return h.scan(ctx, result)
	case intent.ActionAutoTriageImprovePrompt:
		return h.improvePrompt(ctx, result)
	case intent.ActionAutoTriageFileBug:
		apiKey, token, err := h.settings.TrelloCreds(ctx)
		if err != nil {
			return "", fmt.Errorf("resolve trello creds: %w", err)
		}
		if apiKey == "" || token == "" {
			return notConfiguredMsg, nil
		}
		return h.fileBug(ctx, apiKey, token, result.Entities)
	default:
		return "I can scan recent failures, file a bug for one, or improve a skill's prompt.", nil
	}
}

// --- scan ---

// failureGroup is one recurring failure pattern: a set of failed runs that share
// a signature (the same error, or the same skill(s) underperforming).
type failureGroup struct {
	Signature   string   `json:"signature"`
	Kind        string   `json:"kind"` // "error" | "low_quality"
	Count       int      `json:"count"`
	Skills      []string `json:"skills,omitempty"`
	SampleInput string   `json:"sample_input"`
	SampleError string   `json:"sample_error,omitempty"`
	AvgScore    float64  `json:"avg_score,omitempty"`
	FirstSeen   string   `json:"first_seen"`
	LastSeen    string   `json:"last_seen"`
	TraceIDs    []int64  `json:"trace_ids"`
}

type scanReport struct {
	WindowHours   int               `json:"window_hours"`
	TotalFailures int               `json:"total_failures"`
	GroupCount    int               `json:"recurring_groups"`
	Groups        []failureGroup    `json:"groups"`
	SkillPrompts  map[string]string `json:"current_skill_prompts,omitempty"`
}

func (h *Handler) scan(ctx context.Context, result *intent.ParseResult) (string, error) {
	userID := authctx.UserID(ctx)
	hours := parseInt(result.Entities["hours"], defaultHours)
	if hours <= 0 {
		hours = defaultHours
	}

	now := time.Now()
	from := now.Add(-time.Duration(hours) * time.Hour)
	traces, err := h.store.ListFailedTraces(ctx, userID, from, now, routineSources, scanLimit)
	if err != nil {
		return "", fmt.Errorf("list failed traces: %w", err)
	}
	if len(traces) == 0 {
		return fmt.Sprintf("No unhandled failures or low-quality replies in the last %dh. Nothing to triage right now.", hours), nil
	}

	// Group by signature. accum tracks running totals for the average score.
	groups := map[string]*failureGroup{}
	skillsUnion := map[string]bool{}
	scoreSum := map[string]float64{}
	scoreN := map[string]int{}
	for _, t := range traces {
		sig, kind := signatureFor(t)
		g := groups[sig]
		if g == nil {
			g = &failureGroup{Signature: sig, Kind: kind, FirstSeen: t.CreatedAt.Format(time.RFC3339), LastSeen: t.CreatedAt.Format(time.RFC3339)}
			groups[sig] = g
		}
		g.Count++
		g.TraceIDs = append(g.TraceIDs, t.ID)
		// Traces arrive newest-first within a signature; keep the earliest as
		// FirstSeen and the latest as LastSeen.
		if ts := t.CreatedAt.Format(time.RFC3339); ts < g.FirstSeen {
			g.FirstSeen = ts
		} else if ts > g.LastSeen {
			g.LastSeen = ts
		}
		if g.SampleInput == "" {
			g.SampleInput = truncate(t.Input, maxTextChars)
		}
		if g.SampleError == "" && strings.TrimSpace(t.Error) != "" {
			g.SampleError = truncate(t.Error, maxTextChars)
		}
		for _, sk := range t.Skills {
			if sk == "" {
				continue
			}
			if !contains(g.Skills, sk) {
				g.Skills = append(g.Skills, sk)
			}
			skillsUnion[sk] = true
		}
		if t.Score != nil {
			scoreSum[sig] += t.Score.Overall
			scoreN[sig]++
		}
	}

	out := make([]failureGroup, 0, len(groups))
	for sig, g := range groups {
		if n := scoreN[sig]; n > 0 {
			g.AvgScore = round1(scoreSum[sig] / float64(n))
		}
		sort.Strings(g.Skills)
		out = append(out, *g)
	}
	// Most frequent (most "recurring") first; a stable id-desc tiebreak via the
	// first trace id so the ordering is deterministic.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return firstID(out[i].TraceIDs) > firstID(out[j].TraceIDs)
	})
	if len(out) > maxGroups {
		out = out[:maxGroups]
	}

	// Attach the current prompt of every skill that appears, so the model can
	// improve the ones tied to a recurring pattern.
	prompts := map[string]string{}
	if len(skillsUnion) > 0 {
		skills, err := h.store.ListSkills(ctx)
		if err != nil {
			return "", fmt.Errorf("list skills: %w", err)
		}
		for _, sk := range skills {
			if skillsUnion[sk.Key] && sk.Key != selfKey {
				prompts[sk.Key] = sk.EffectivePrompt()
			}
		}
	}

	rep := scanReport{
		WindowHours:   hours,
		TotalFailures: len(traces),
		GroupCount:    len(out),
		Groups:        out,
		SkillPrompts:  prompts,
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal scan: %w", err)
	}
	return string(b), nil
}

// signatureFor derives a stable grouping signature for a failed trace. Agent
// errors group by their normalised error text; low-quality replies group by the
// skill(s) involved (or "general" when none), so the same failing skill's runs
// collapse into one recurring pattern.
func signatureFor(t store.Trace) (sig, kind string) {
	if strings.EqualFold(t.Status, "error") {
		return "error:" + normalizeError(t.Error), "error"
	}
	skills := "general"
	if len(t.Skills) > 0 {
		cp := append([]string(nil), t.Skills...)
		sort.Strings(cp)
		skills = strings.Join(cp, "+")
	}
	return "low_quality:" + skills, "low_quality"
}

// normalizeError reduces an error string to a low-cardinality signature: the
// first line, lower-cased, with digit runs collapsed to "#" (so ids/counts don't
// split one pattern into many) and whitespace collapsed. Empty errors map to
// "unknown".
func normalizeError(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.ToLower(s)
	var b strings.Builder
	prevDigit, prevSpace := false, false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			if !prevDigit {
				b.WriteByte('#')
			}
			prevDigit, prevSpace = true, false
		case r == ' ' || r == '\t':
			if !prevSpace {
				b.WriteByte(' ')
			}
			prevDigit, prevSpace = false, true
		default:
			b.WriteRune(r)
			prevDigit, prevSpace = false, false
		}
	}
	return truncate(strings.TrimSpace(b.String()), 160)
}

// --- file_bug ---

func (h *Handler) fileBug(ctx context.Context, apiKey, token string, e map[string]string) (string, error) {
	title := strings.TrimSpace(e["title"])
	if title == "" {
		return "What's the bug? I need a short title to file it on the Issue board.", nil
	}
	sig := strings.TrimSpace(e["signature"])
	if sig == "" {
		return "I need the failure's signature (from the triage scan) to file it and avoid duplicates.", nil
	}
	desc := strings.TrimSpace(e["description"])

	// Duplicate detection: look at the open cards on the Issue board. Re-use an
	// existing card if it carries the same triage signature, or if its title
	// matches — then comment on it instead of filing a duplicate.
	cards, err := h.client.BoardCards(ctx, apiKey, token, boardIssue)
	if err != nil {
		h.log.Warn("trello list issue cards failed", "error", err)
		return fmt.Sprintf("Couldn't read the Issue board to check for duplicates: %v", err), nil
	}
	marker := signatureTag + sig
	for _, c := range cards {
		if strings.Contains(c.Desc, marker) || strings.EqualFold(strings.TrimSpace(c.Name), title) {
			note := strings.TrimSpace(e["recurrence_note"])
			if note == "" {
				note = fmt.Sprintf("🔁 Auto-triage saw this failure pattern again (%s).", time.Now().Format("2006-01-02"))
			}
			if err := h.client.AddComment(ctx, apiKey, token, c.ID, note); err != nil {
				h.log.Warn("trello add recurrence comment failed", "card", c.ID, "error", err)
				return fmt.Sprintf("An open bug for this already exists (%s) but I couldn't add a recurrence note: %v", c.ShortURL, err), nil
			}
			return fmt.Sprintf("A bug for this pattern is already open — added a recurrence note instead of a duplicate.\n%s\nTell the user in their language.", c.ShortURL), nil
		}
	}

	// No existing card — file a new bug on the Issue → Bug list, embedding the
	// signature so future runs recognise it.
	body := desc
	if body != "" {
		body += "\n\n"
	}
	body += "---\n_Filed automatically by nightly auto-triage._\n`" + marker + "`"

	card, err := h.client.CreateCard(ctx, apiKey, token, trello.CreateCardInput{ListID: listBug, Name: title, Desc: body})
	if err != nil {
		h.log.Warn("trello file triage bug failed", "error", err)
		return fmt.Sprintf("Couldn't file the bug card: %v", err), nil
	}

	// Read-after-write: confirm the card actually persisted on the Bug list and
	// isn't archived before reporting success — not merely that its id resolves.
	if got, err := h.client.GetCard(ctx, apiKey, token, card.ID); err != nil {
		h.log.Warn("trello verify triage bug failed", "card", card.ID, "error", err)
		return fmt.Sprintf("I tried to file the bug card but couldn't verify it saved on Trello: %v", err), nil
	} else if got.Closed || got.IDList != listBug {
		h.log.Warn("trello verify triage bug mismatch", "card", card.ID, "closed", got.Closed, "list", got.IDList)
		return "I tried to file the bug card but couldn't verify it saved on Trello: the card didn't land on the Bug list.", nil
	}
	h.log.Info("auto-triage filed bug", "title", title, "signature", sig, "card", card.ID)
	return fmt.Sprintf("Filed bug %q on the Issue → Bug list.\n%s\nTell the user in their language.", title, card.ShortURL), nil
}

// --- improve_prompt ---

func (h *Handler) improvePrompt(ctx context.Context, result *intent.ParseResult) (string, error) {
	key := strings.TrimSpace(result.Entities["skill"])
	prompt := strings.TrimSpace(result.Entities["prompt"])
	reason := strings.TrimSpace(result.Entities["reason"])
	if key == "" {
		return "Which skill should I update? Pass its key.", nil
	}
	if prompt == "" {
		return "I need the full new prompt text to save for that skill.", nil
	}
	if key == selfKey || key == tunerKey {
		return fmt.Sprintf("I won't tune the %q skill itself.", key), nil
	}

	skills, err := h.store.ListSkills(ctx)
	if err != nil {
		return "", fmt.Errorf("list skills: %w", err)
	}
	var found *store.Skill
	valid := make([]string, 0, len(skills))
	for i := range skills {
		valid = append(valid, skills[i].Key)
		if skills[i].Key == key {
			found = &skills[i]
		}
	}
	if found == nil {
		return fmt.Sprintf("No skill with key %q. Valid keys: %s.", key, strings.Join(valid, ", ")), nil
	}

	if err := h.store.UpdateSkillTunedPrompt(ctx, key, prompt); err != nil {
		return "", fmt.Errorf("update skill prompt: %w", err)
	}

	// Read-after-write: confirm the new tuned prompt persisted before reporting.
	saved, err := h.store.GetSkill(ctx, found.ID)
	if err != nil {
		return "", fmt.Errorf("verify skill prompt saved: %w", err)
	}
	if saved == nil || saved.TunedPrompt != prompt {
		return "", fmt.Errorf("verify skill prompt saved: stored prompt does not match after update")
	}
	h.log.Info("skill prompt refined by auto-triage", "skill", key, "reason", reason, "chars", len(prompt))
	msg := fmt.Sprintf("Updated the %q (%s) skill prompt.", found.Name, key)
	if reason != "" {
		msg += " " + reason
	}
	return msg, nil
}

// --- helpers ---

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…[truncated]"
}

func parseInt(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func firstID(ids []int64) int64 {
	if len(ids) == 0 {
		return 0
	}
	return ids[0]
}

func round1(f float64) float64 {
	return float64(int64(f*10+0.5)) / 10
}
