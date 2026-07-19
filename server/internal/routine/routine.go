// Package routine runs daily "scheduled skills": code-owned prompts that fire
// once a day at a configurable local time, run through the assistant agent with
// full tool access, and (when the agent produces a message) deliver its reply to
// the owner over WhatsApp.
//
// A routine is deliberately general — it is just a prompt run once a day as the
// owner, so it can do anything the agent can do; the shipped defaults merely set
// up a few useful jobs. There are three out of the box:
//
//   - start_of_day: a morning briefing that also checks the calendar and
//     reminders and messages the owner about anything on for today or tomorrow.
//   - end_of_day: a self-improvement pass that reviews the day's low-quality
//     conversations and refines the prompts of the skills involved (requires the
//     Self-Tuning skill to be enabled).
//   - nightly_triage: a failure-triage pass that scans the day's unhandled runs,
//     files a bug card on the Trello Issue board for each recurring pattern
//     (skipping duplicates), and refines the prompts of the skills that keep
//     failing (requires the Auto-Triage skill to be enabled).
//
// Each routine's time, prompt, and on/off state are editable and persisted in
// settings; the code owns only the catalog and the defaults. This replaces the
// older, hard-coded reminder "digest".
package routine

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/agent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
	"github.com/irfanmaulana007/personal-assistant/server/internal/transport"
)

const (
	// checkInterval is how often the scheduler wakes to see whether a routine is
	// due; graceWindow bounds how late after the slot a missed run may still fire
	// (so a long outage doesn't blast a stale briefing mid-afternoon).
	checkInterval = time.Minute
	graceWindow   = 30 * time.Minute

	// Sentinel is what a routine prompt returns when there is nothing worth
	// sending; the scheduler then delivers nothing.
	Sentinel = "NOTHING_TO_REPORT"
)

// AgentRunner runs a prompt through the assistant agent. *agent.Agent satisfies
// it. The user id must already be set on the context (via authctx).
type AgentRunner interface {
	Run(ctx context.Context, message string, history []agent.Message, image string) (*agent.Result, error)
}

// Def is a code-owned daily routine and its defaults.
type Def struct {
	Key            string
	Name           string
	Description    string
	DefaultTime    string // local "HH:MM"
	DefaultEnabled bool
	DefaultPrompt  string
	// MaxIterations overrides the agent's tool-loop budget for this routine (0 =
	// agent default). Tool-heavy routines like the self-tuner need more room.
	MaxIterations int
}

// Catalog is the fixed set of daily routines the app ships with.
var Catalog = []Def{
	{
		Key:            "start_of_day",
		Name:           "Start of day",
		Description:    "A morning run: whatever you tell it to do. The default puts together a briefing and checks your calendar and reminders for today and tomorrow, messaging you on WhatsApp if anything is coming up.",
		DefaultTime:    "07:00",
		DefaultEnabled: false,
		DefaultPrompt: `Good morning! Put together my start-of-day briefing.

1. Check my calendar events and my reminders for today and tomorrow (use your tools to look them up).
2. If there is at least one event or reminder across today or tomorrow, write me a short, friendly WhatsApp good-morning message that groups everything under "Today" and "Tomorrow" in chronological order, each item on its own line with its time. Keep it warm and concise.
3. If there are no events and no reminders at all for either today or tomorrow, reply with exactly NOTHING_TO_REPORT and nothing else.

Reply with ONLY the message to send me — no preamble, no explanation.`,
	},
	{
		Key:            "end_of_day",
		Name:           "End of day",
		Description:    "An evening run: whatever you tell it to do. The default is a self-improvement pass — it reviews today's low-quality conversations and refines the prompts of the skills involved. Requires the Self-Tuning skill to be enabled.",
		DefaultTime:    "21:00",
		DefaultEnabled: false,
		MaxIterations:  12,
		DefaultPrompt: `It's the end of the day. Improve my assistant by learning from today's conversations that didn't go well.

1. Call review_skill_performance (no arguments) to pull recent low-quality conversations that involved a skill, along with each involved skill's current prompt.
2. If it reports there is nothing to review, reply with exactly NOTHING_TO_REPORT and nothing else — do not send a message.
3. Otherwise, for each skill that clearly underperformed, work out WHY from the conversation's input, the reply, the tools called, and the judge's rationale. Then call update_skill_prompt with an improved prompt for that skill: keep what already works, make a focused fix for the observed problem, and preserve tool names and any required output markers exactly. Prioritise the worst-scoring skills first. Do not tune a skill you have no evidence is failing, and never touch the self_tuning skill.
4. When done, write me a short WhatsApp message summarising which skills you improved and, in one line each, what you changed. If you ended up changing nothing, reply with exactly NOTHING_TO_REPORT instead.

Reply with ONLY the message to send me — no preamble, no explanation.`,
	},
	{
		Key:            "nightly_triage",
		Name:           "Nightly triage",
		Description:    "A nightly run: whatever you tell it to do. The default triages the day's failures — it scans the runs the assistant couldn't handle (errors and low-quality replies), files a bug card on the Trello Issue board for each recurring pattern (skipping duplicates), and refines the prompts of the skills that keep failing. Requires the Auto-Triage skill to be enabled.",
		DefaultTime:    "23:00",
		DefaultEnabled: false,
		MaxIterations:  20,
		DefaultPrompt: `It's late — triage the day's failures so recurring problems get tracked and fixed.

1. Call triage_scan_failures (no arguments) to pull the day's runs I couldn't handle automatically — errors and low-quality replies — grouped into recurring failure patterns.
2. If it reports there is nothing to triage, reply with exactly NOTHING_TO_REPORT and nothing else — do not send a message.
3. Otherwise, for each recurring pattern worth acting on (prioritise the ones that recurred most), call triage_file_bug with the group's signature copied verbatim, a clear English title, and a description covering the sample input, the error, the occurrence count, and the first/last-seen times. The tool detects duplicates and will comment on an existing card instead of filing a second one. Do NOT file a bug for a one-off.
4. When a recurring pattern is clearly caused by a specific skill's instructions, also call triage_improve_prompt to save a focused fix for that skill's prompt — keep what works, preserve tool names and required output markers exactly, and pass a one-line reason. Never tune the auto_triage or self_tuning skills.
5. When done, write me a short WhatsApp message summarising which bugs you filed (or recurrences you noted) and any skill prompts you improved, one line each. If you ended up doing nothing, reply with exactly NOTHING_TO_REPORT instead.

Reply with ONLY the message to send me — no preamble, no explanation.`,
	},
}

func defByKey(key string) (Def, bool) {
	for _, d := range Catalog {
		if d.Key == key {
			return d, true
		}
	}
	return Def{}, false
}

// Service resolves routine config, runs routines on demand, and drives the
// daily scheduler.
type Service struct {
	settings *settings.Service
	store    store.Store
	agent    AgentRunner
	timezone *time.Location
	send     transport.SendFunc
	ownerJID string
	log      *slog.Logger
}

// New creates a routine service. tz governs when routines fire; ownerJID is the
// nominal WhatsApp recipient (the send func may override the actual number).
func New(settingsSvc *settings.Service, st store.Store, ag AgentRunner, tz *time.Location, ownerJID string, log *slog.Logger) *Service {
	if tz == nil {
		tz = time.UTC
	}
	return &Service{
		settings: settingsSvc,
		store:    st,
		agent:    ag,
		timezone: tz,
		ownerJID: ownerJID,
		log:      log.With("component", "routine"),
	}
}

// SetSendFunc sets the function used to deliver a routine's message (WhatsApp).
func (s *Service) SetSendFunc(fn transport.SendFunc) { s.send = fn }

// --- config resolution (stored value, falling back to the code default) ---

func (s *Service) enabled(ctx context.Context, d Def) bool {
	switch s.settings.RoutineField(ctx, d.Key, "enabled") {
	case "true":
		return true
	case "false":
		return false
	default:
		return d.DefaultEnabled
	}
}

func (s *Service) timeOf(ctx context.Context, d Def) string {
	if v := s.settings.RoutineField(ctx, d.Key, "time"); v != "" {
		return v
	}
	return d.DefaultTime
}

func (s *Service) promptOf(ctx context.Context, d Def) string {
	if v := s.settings.RoutineField(ctx, d.Key, "prompt"); v != "" {
		return v
	}
	return d.DefaultPrompt
}

// View is the API-facing shape of a routine's current configuration.
type View struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Enabled       bool   `json:"enabled"`
	Time          string `json:"time"`           // local "HH:MM"
	Prompt        string `json:"prompt"`         // effective prompt (stored override or default)
	DefaultPrompt string `json:"default_prompt"` // the built-in default, for a "reset" affordance
	LastRun       string `json:"last_run"`       // "YYYY-MM-DD" of the last fire, or ""
}

func (s *Service) view(ctx context.Context, d Def) View {
	return View{
		Key:           d.Key,
		Name:          d.Name,
		Description:   d.Description,
		Enabled:       s.enabled(ctx, d),
		Time:          s.timeOf(ctx, d),
		Prompt:        s.promptOf(ctx, d),
		DefaultPrompt: d.DefaultPrompt,
		LastRun:       s.settings.RoutineField(ctx, d.Key, "last_run"),
	}
}

// List returns every routine's current configuration, in catalog order.
func (s *Service) List(ctx context.Context) []View {
	out := make([]View, 0, len(Catalog))
	for _, d := range Catalog {
		out = append(out, s.view(ctx, d))
	}
	return out
}

// Update is a partial change to a routine's configuration. Nil fields are left
// unchanged.
type Update struct {
	Enabled *bool
	Time    *string
	Prompt  *string
}

// Update applies a partial change to a routine and returns its new view.
// An unknown key or a malformed time is an error.
func (s *Service) Update(ctx context.Context, key string, u Update) (View, error) {
	d, ok := defByKey(key)
	if !ok {
		return View{}, fmt.Errorf("unknown routine %q", key)
	}
	if u.Time != nil {
		hh, mm, ok := parseHM(*u.Time)
		if !ok {
			return View{}, fmt.Errorf("time must be HH:MM")
		}
		if err := s.settings.SetRoutineField(ctx, key, "time", fmt.Sprintf("%02d:%02d", hh, mm)); err != nil {
			return View{}, err
		}
	}
	if u.Enabled != nil {
		val := "false"
		if *u.Enabled {
			val = "true"
		}
		if err := s.settings.SetRoutineField(ctx, key, "enabled", val); err != nil {
			return View{}, err
		}
	}
	if u.Prompt != nil {
		// An empty prompt clears the override, reverting to the built-in default.
		if err := s.settings.SetRoutineField(ctx, key, "prompt", strings.TrimSpace(*u.Prompt)); err != nil {
			return View{}, err
		}
	}
	return s.view(ctx, d), nil
}

// RunNow runs a routine immediately, ignoring its schedule (for a "run now"
// button / testing). It reports whether a message was sent and the message body.
func (s *Service) RunNow(ctx context.Context, key string) (sent bool, message string, err error) {
	d, ok := defByKey(key)
	if !ok {
		return false, "", fmt.Errorf("unknown routine %q", key)
	}
	return s.run(ctx, d)
}

// run executes a routine's prompt through the agent (as the owner) and delivers
// the reply over WhatsApp, unless the reply is empty or the sentinel.
func (s *Service) run(ctx context.Context, d Def) (sent bool, message string, err error) {
	owner, err := s.store.FirstAdmin(ctx)
	if err != nil {
		return false, "", fmt.Errorf("resolve owner: %w", err)
	}
	if owner == nil {
		return false, "", fmt.Errorf("no admin user configured")
	}
	uctx := authctx.WithUserID(ctx, owner.ID)
	if d.MaxIterations > 0 {
		uctx = agent.WithMaxIterations(uctx, d.MaxIterations)
	}

	prompt := s.promptOf(ctx, d)
	start := time.Now()
	res, err := s.agent.Run(uctx, prompt, nil, "")
	s.recordTrace(uctx, owner.ID, d, prompt, res, err, int(time.Since(start).Milliseconds()))
	if err != nil {
		return false, "", fmt.Errorf("agent run: %w", err)
	}
	reply := strings.TrimSpace(res.Reply)
	if reply == "" || strings.EqualFold(reply, Sentinel) {
		return false, reply, nil // nothing to report
	}
	if s.send == nil {
		return false, reply, fmt.Errorf("whatsapp not connected")
	}
	if err := s.send(ctx, s.ownerJID, reply); err != nil {
		return false, reply, fmt.Errorf("send: %w", err)
	}
	return true, reply, nil
}

// recordTrace persists one agent run as a trace so scheduled routines show up on
// the Logs page alongside interactive chats. The trace is tagged Source=d.Key
// ("start_of_day" / "end_of_day") so the log can be attributed to the routine
// that triggered it, and Platform="whatsapp" since that is the delivery channel.
// Trace persistence is best-effort: a failure here must not abort the routine.
func (s *Service) recordTrace(ctx context.Context, userID int64, d Def, prompt string, res *agent.Result, runErr error, latencyMs int) {
	trace := &store.Trace{
		UserID:    userID,
		Platform:  "whatsapp",
		Source:    d.Key,
		Input:     prompt,
		LatencyMs: latencyMs,
	}
	if runErr != nil {
		trace.Status = "error"
		trace.Error = runErr.Error()
	} else if res != nil {
		trace.Output = res.Reply
		trace.Model = res.Model
		trace.PromptTokens = res.Usage.PromptTokens
		trace.CompletionTokens = res.Usage.CompletionTokens
		trace.TotalTokens = res.Usage.TotalTokens
		trace.ImageModel = res.ImageModel
		trace.ImagePromptTokens = res.ImagePromptTokens
		trace.ImageCompletionTokens = res.ImageCompletionTokens
		trace.ImageTotalTokens = res.ImageTotalTokens
		trace.ToolCount = len(res.Tools)
		trace.Skills = res.Skills
		for _, tool := range res.Tools {
			trace.Tools = append(trace.Tools, store.ToolInvocation{
				Name: tool.Name, Arguments: tool.Arguments, Result: tool.Result, LatencyMs: tool.LatencyMs,
				Model: tool.Model, PromptTokens: tool.PromptTokens,
				CompletionTokens: tool.CompletionTokens, TotalTokens: tool.TotalTokens,
			})
		}
		for _, st := range res.Steps {
			trace.Steps = append(trace.Steps, store.LLMCall{
				Step: st.Step, Model: st.Model, PromptTokens: st.PromptTokens,
				CompletionTokens: st.CompletionTokens, TotalTokens: st.TotalTokens,
				LatencyMs: st.LatencyMs, FinishReason: st.FinishReason, ToolCalls: st.ToolCalls,
			})
		}
	}
	if _, err := s.store.CreateTrace(ctx, trace); err != nil {
		s.log.Warn("record routine trace", "routine", d.Key, "error", err)
	}
}

// MigrateFromDigest carries the legacy reminder daily-recap time over to the
// start_of_day routine the first time it runs, then clears the old setting so
// the two never both fire. It is a no-op once the routine has its own time or
// when no legacy digest was configured.
func (s *Service) MigrateFromDigest(ctx context.Context) {
	if s.settings.RoutineField(ctx, "start_of_day", "time") != "" {
		return // routine already configured; nothing to migrate
	}
	old := strings.TrimSpace(s.settings.ReminderDigestTime(ctx))
	if old == "" {
		return
	}
	_ = s.settings.SetRoutineField(ctx, "start_of_day", "time", old)
	_ = s.settings.SetRoutineField(ctx, "start_of_day", "enabled", "true")
	_ = s.settings.SetReminderDigestTime(ctx, "")
	s.log.Info("migrated reminder digest to start_of_day routine", "time", old)
}

// StartScheduler runs the daily routine loop until ctx is cancelled.
func (s *Service) StartScheduler(ctx context.Context) {
	s.log.Info("routine scheduler started")
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.log.Info("routine scheduler stopped")
			return
		case <-ticker.C:
			now := time.Now().In(s.timezone)
			for _, d := range Catalog {
				s.maybeRun(ctx, d, now)
			}
		}
	}
}

// maybeRun fires a routine at most once per day, once its local slot has passed
// (within the grace window).
func (s *Service) maybeRun(ctx context.Context, d Def, now time.Time) {
	if !s.enabled(ctx, d) {
		return
	}
	hh, mm, ok := parseHM(s.timeOf(ctx, d))
	if !ok {
		return
	}
	slot := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, s.timezone)
	if now.Before(slot) {
		return // not yet time today
	}
	today := now.Format("2006-01-02")
	if s.settings.RoutineField(ctx, d.Key, "last_run") == today {
		return // already ran today
	}
	// Claim today's slot regardless of outcome so it fires at most once; skip the
	// send if we're well past the slot (server was down through it).
	if err := s.settings.SetRoutineField(ctx, d.Key, "last_run", today); err != nil {
		s.log.Error("failed to record routine run date", "routine", d.Key, "error", err)
	}
	if now.Sub(slot) > graceWindow {
		return
	}
	sent, _, err := s.run(ctx, d)
	if err != nil {
		s.log.Warn("routine run failed", "routine", d.Key, "error", err)
		return
	}
	if sent {
		s.log.Info("routine sent", "routine", d.Key)
	} else {
		s.log.Info("routine had nothing to report", "routine", d.Key)
	}
}

// parseHM parses an "HH:MM" 24-hour string.
func parseHM(hm string) (int, int, bool) {
	parts := strings.SplitN(strings.TrimSpace(hm), ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	hh, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	mm, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, false
	}
	return hh, mm, true
}
