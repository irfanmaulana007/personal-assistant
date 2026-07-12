// Package routine runs daily "scheduled skills": code-owned prompts that fire
// once a day at a configurable local time, run through the assistant agent (with
// full tool access — reminders, calendar, etc.), and deliver the agent's reply
// to the owner over WhatsApp.
//
// There are two routines out of the box — a start-of-day and an end-of-day
// briefing. Each routine's time, prompt, and on/off state are editable and
// persisted in settings; the code owns only the catalog and the defaults. This
// replaces the older, hard-coded reminder "digest".
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
}

// Catalog is the fixed set of daily routines the app ships with.
var Catalog = []Def{
	{
		Key:            "start_of_day",
		Name:           "Start of day",
		Description:    "A morning briefing of your reminders and calendar for today and tomorrow, sent over WhatsApp.",
		DefaultTime:    "07:00",
		DefaultEnabled: false,
		DefaultPrompt: `Good morning! Put together my briefing for the day ahead.

1. List all of my reminders for today and tomorrow.
2. List all of my calendar events for today and tomorrow.
3. If there is at least one reminder or event, write me a short, friendly WhatsApp good-morning message that groups everything under "Today" and "Tomorrow" in chronological order, each item on its own line with its time. Keep it warm and concise.
4. If there are no reminders and no events at all for today or tomorrow, reply with exactly NOTHING_TO_REPORT and nothing else.

Reply with ONLY the message to send me — no preamble, no explanation.`,
	},
	{
		Key:            "end_of_day",
		Name:           "End of day",
		Description:    "An evening wind-down that recaps what's coming up tomorrow, sent over WhatsApp.",
		DefaultTime:    "21:00",
		DefaultEnabled: false,
		DefaultPrompt: `It's the end of the day — help me wind down and get ready for tomorrow.

1. List all of my reminders and calendar events for tomorrow.
2. Write me a short, friendly WhatsApp end-of-day message that recaps what's coming up tomorrow, in chronological order with each item's time, so I can plan ahead. Add a brief, encouraging sign-off.
3. If there is nothing scheduled for tomorrow, reply with exactly NOTHING_TO_REPORT and nothing else.

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
