// Package agent implements an LLM tool-calling agent that drives the existing
// capability handlers. It replaces the regex intent parser: the LLM decides
// which capability tool to call, the tool call is mapped onto an
// intent.ParseResult and executed via the capability.Router, and the tool
// result is fed back to the model until it produces a final reply.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/server/internal/config"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/media"
	"github.com/irfanmaulana007/personal-assistant/server/internal/memory"
	"github.com/irfanmaulana007/personal-assistant/server/internal/persona"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/skills"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// ErrNotConfigured is returned when no LLM API key has been configured.
var ErrNotConfigured = errors.New("llm api key not configured")

// maxIterations bounds the tool-calling loop to avoid runaway LLM/tool cycles.
const maxIterations = 5

// saveIntentRE matches messages that ask the assistant to persist something
// (add/save/record/remind/schedule) in either Indonesian or English. When it
// matches, the turn is treated as a write and the first LLM step is forced to
// call a tool (tool_choice=required) — see hasSaveIntent. This is the
// structural guard against weak models (e.g. deepseek-v4-flash) fabricating a
// "done ✅" confirmation without ever calling the save tool.
//
// Indonesian verbs are matched as prefixes so agglutinated forms are covered
// (catat→catatan, tambah→tambahkan/tambahin, masuk→masukin/masukkan). English
// verbs are matched as whole words to avoid false hits inside longer words
// (add vs. address, log vs. login).
var saveIntentRE = regexp.MustCompile(`(?i)(\b(catat|catet|simpan|tambah|nambah|masuk|jadwal|ingat|inget|tandai))|(\b(add|save|remind|reminder|schedule|record|remember|jot)\b)`)

// hasSaveIntent reports whether the user message expresses an intent to persist
// something, so the turn should force a tool call rather than accept a
// text-only (potentially fabricated) confirmation.
func hasSaveIntent(msg string) bool {
	return saveIntentRE.MatchString(msg)
}

// ToolProvider supplies extra, dynamically-resolved tools (e.g. connected
// Composio apps) and executes them. Implementations read the current user from
// the context (via authctx). All methods must tolerate a nil/empty result.
type ToolProvider interface {
	// Tools returns the extra tools available to the current user.
	Tools(ctx context.Context) []llm.Tool
	// Handles reports whether a tool call name belongs to this provider.
	Handles(name string) bool
	// Execute runs the tool and returns a result string for the model.
	Execute(ctx context.Context, name, argsJSON string) string
}

// Agent orchestrates the LLM tool-calling loop.
type Agent struct {
	client   *llm.Client
	settings *settings.Service
	skills   *skills.Service
	memory   *memory.Service
	persona  *persona.Service
	router   *capability.Router
	owner    config.OwnerConfig
	provider ToolProvider // optional; extra tools (may be nil)
	log      *slog.Logger
}

// New creates an agent. provider may be nil (no extra tools).
func New(client *llm.Client, settingsSvc *settings.Service, skillsSvc *skills.Service, memSvc *memory.Service, personaSvc *persona.Service, router *capability.Router, owner config.OwnerConfig, provider ToolProvider, log *slog.Logger) *Agent {
	return &Agent{
		client:   client,
		settings: settingsSvc,
		skills:   skillsSvc,
		memory:   memSvc,
		persona:  personaSvc,
		router:   router,
		owner:    owner,
		provider: provider,
		log:      log.With("component", "agent"),
	}
}

// ToolInvocation records a single tool call made during a run.
type ToolInvocation struct {
	Name      string
	Arguments string
	Result    string
	LatencyMs int
}

// LLMCall records one LLM round-trip within a run (the agent loops, so a run
// can involve several calls).
type LLMCall struct {
	Step             int
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	LatencyMs        int
	FinishReason     string
	ToolCalls        []string // tool names requested in this step
}

// Result is the outcome of an agent run.
type Result struct {
	Reply  string
	Usage  llm.Usage
	Model  string
	Tools  []ToolInvocation // tools invoked during the run, in order
	Steps  []LLMCall        // each LLM round-trip, in order
	Skills []string         // skill keys active for this run
	Images []media.Image    // images produced by tools (e.g. Image Generator)
}

// Message is a prior conversation turn used as context.
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// Run executes the tool-calling loop for a single user message. history holds
// prior turns (oldest first) for conversational context. image, when non-empty,
// is a data: URL for a user-attached image. It is always exposed to tools via
// the media context (so edit_image can read it), and additionally sent inline
// to the model as an image_url block only when the configured model is
// vision-capable (cfg.Vision) — text-only providers reject image content.
func (a *Agent) Run(ctx context.Context, userMessage string, history []Message, image string) (*Result, error) {
	cfg, err := a.settings.LLMConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve llm config: %w", err)
	}
	if cfg.APIKey == "" {
		return nil, ErrNotConfigured
	}

	userID := authctx.UserID(ctx)

	// Collect images produced by tools out of band (they can't ride the text
	// tool-result channel), and expose the user's inbound image so the
	// edit_image tool can read its bytes.
	ctx, imageCollector := media.WithCollector(ctx)
	if image != "" {
		if in, ok := media.ParseDataURL(image); ok {
			ctx = media.WithInbound(ctx, in)
		}
	}

	// Resolve the user's enabled skills: their prompts enrich the system
	// prompt and their tools are added to the tool list.
	var enabledSkills []store.Skill
	if a.skills != nil {
		enabledSkills = a.skills.Enabled(ctx, userID)
	}
	enabledKeys := make([]string, 0, len(enabledSkills))
	for _, sk := range enabledSkills {
		enabledKeys = append(enabledKeys, sk.Key)
	}

	// Retrieve long-term memories relevant to this message for prompt injection.
	var memories []store.Memory
	if a.memory != nil {
		memories = a.memory.Relevant(ctx, userID, userMessage, 6)
	}

	personaPrompt := ""
	if a.persona != nil {
		personaPrompt = a.persona.Prompt(ctx, userID)
	}

	messages := []llm.Message{{Role: "system", Content: a.systemPrompt(enabledSkills, memories, personaPrompt)}}
	for _, m := range history {
		messages = append(messages, llm.Message{Role: m.Role, Content: m.Content})
	}
	// Only attach the image to the chat request when the configured model can
	// actually ingest it. Text-only providers (e.g. deepseek-v4-flash) reject an
	// image_url content block with a deserialization 400. The image is always
	// stashed on the context above via media.WithInbound, so tools like
	// edit_image can still read its bytes out of band even when it's not sent
	// inline to the model.
	if image != "" && cfg.Vision {
		messages = append(messages, llm.Message{Role: "user", ContentParts: []llm.ContentPart{
			{Type: "text", Text: userMessage},
			{Type: "image_url", ImageURL: &llm.ImageURL{URL: image}},
		}})
	} else {
		messages = append(messages, llm.Message{Role: "user", Content: userMessage})
	}

	tools := toolSchemas()
	tools = append(tools, skillToolSchemas(enabledKeys)...)
	if a.provider != nil {
		tools = append(tools, a.provider.Tools(ctx)...)
	}
	// Save-intent turns must not be answered with a fabricated confirmation.
	// When the user asks to persist something and tools are available, force the
	// model to call a tool until at least one has run this turn (tool_choice=
	// "required"); once a tool has executed, later steps fall back to "auto" so
	// the model can summarize the real result.
	forceOnSaveIntent := len(tools) > 0 && hasSaveIntent(userMessage)

	var total llm.Usage
	var used []ToolInvocation
	var steps []LLMCall

	recordStep := func(res *llm.CompletionResult, latencyMs int) {
		names := make([]string, 0, len(res.Message.ToolCalls))
		for _, tc := range res.Message.ToolCalls {
			names = append(names, tc.Function.Name)
		}
		steps = append(steps, LLMCall{
			Step: len(steps) + 1, Model: cfg.Model,
			PromptTokens: res.Usage.PromptTokens, CompletionTokens: res.Usage.CompletionTokens,
			TotalTokens: res.Usage.TotalTokens, LatencyMs: latencyMs,
			FinishReason: res.FinishReason, ToolCalls: names,
		})
	}

	for i := 0; i < maxIterations; i++ {
		start := time.Now()
		var res *llm.CompletionResult
		var err error
		if forceOnSaveIntent && len(used) == 0 {
			res, err = a.client.CompleteRequiringTool(ctx, cfg, messages, tools)
		} else {
			res, err = a.client.Complete(ctx, cfg, messages, tools)
		}
		if err != nil {
			return nil, err
		}
		recordStep(res, int(time.Since(start).Milliseconds()))
		addUsage(&total, res.Usage)

		msg := res.Message
		messages = append(messages, msg)

		if len(msg.ToolCalls) == 0 {
			return &Result{Reply: strings.TrimSpace(msg.Content), Usage: total, Model: cfg.Model, Tools: used, Steps: steps, Skills: enabledKeys, Images: imageCollector.Images()}, nil
		}

		for _, tc := range msg.ToolCalls {
			tStart := time.Now()
			var result string
			if a.provider != nil && a.provider.Handles(tc.Function.Name) {
				result = a.provider.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
			} else {
				result = a.execTool(ctx, tc)
			}
			used = append(used, ToolInvocation{
				Name: tc.Function.Name, Arguments: tc.Function.Arguments, Result: result,
				LatencyMs: int(time.Since(tStart).Milliseconds()),
			})
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    result,
			})
		}
	}

	// Tool budget exhausted — force a final textual answer without tools.
	start := time.Now()
	res, err := a.client.Complete(ctx, cfg, messages, nil)
	if err != nil {
		return nil, err
	}
	recordStep(res, int(time.Since(start).Milliseconds()))
	addUsage(&total, res.Usage)
	reply := strings.TrimSpace(res.Message.Content)
	if reply == "" {
		reply = "I wasn't able to finish that request. Could you rephrase or break it into smaller steps?"
	}
	return &Result{Reply: reply, Usage: total, Model: cfg.Model, Tools: used, Steps: steps, Skills: enabledKeys, Images: imageCollector.Images()}, nil
}

// execTool maps a tool call onto a capability action and runs it via the router.
func (a *Agent) execTool(ctx context.Context, tc llm.ToolCall) string {
	spec, ok := toolByName[tc.Function.Name]
	if !ok {
		a.log.Warn("unknown tool called", "tool", tc.Function.Name)
		return fmt.Sprintf("Error: unknown tool %q.", tc.Function.Name)
	}

	entities := map[string]string{}
	if args := strings.TrimSpace(tc.Function.Arguments); args != "" && args != "{}" {
		raw := map[string]any{}
		if err := json.Unmarshal([]byte(args), &raw); err != nil {
			return fmt.Sprintf("Error: invalid tool arguments: %v", err)
		}
		for k, v := range raw {
			entities[k] = fmt.Sprintf("%v", v)
		}
	}

	result := &intent.ParseResult{
		Capability: spec.capability,
		Action:     spec.action,
		Entities:   entities,
		Confidence: 1.0,
		RawText:    tc.Function.Arguments,
	}

	a.log.Debug("executing tool", "tool", tc.Function.Name, "entities", entities)
	return a.router.Route(ctx, result)
}

func (a *Agent) systemPrompt(enabledSkills []store.Skill, memories []store.Memory, personaPrompt string) string {
	loc := a.owner.Location()
	now := time.Now().In(loc)
	name := a.owner.Name
	if name == "" {
		name = "the user"
	}

	base := fmt.Sprintf(`You are a helpful personal assistant for %s.
The current date and time is %s (timezone: %s).

You can manage the user's calendar, email, reminders, and notes by calling the provided tools.

Guidelines:
- Always reply in the same language the user writes in (for example, reply in Indonesian when they write in Indonesian, in English when they write in English).
- When a request maps to a tool, call it. For time/datetime arguments, pass natural language such as "5pm", "in 30 minutes", or "tomorrow at 9am" — the tools parse these.
- After a tool returns, summarize the result for the user clearly, concisely, and in a friendly tone.
- If a request is ambiguous or missing a required detail, ask one short clarifying question instead of guessing.
- For general questions or small talk, just reply directly without calling any tool.
- Never claim to have sent an email — the email tool only creates drafts.
- Never claim you saved something (a reminder, note, bucket-list item, memory, etc.) unless you actually called the tool that saves it and it returned successfully.

Memory:
- You have long-term memory. Call the "remember" tool to save durable facts about the user (plans, budgets, preferences, decisions, ongoing tasks) so you can use them later and in future sessions. Do this proactively when the user shares something worth keeping.
- Use the "recall" tool to look something up when needed. Rely on the "things you remember" below when present.
- Never claim you have saved or will remember something unless you actually called the "remember" tool, and never claim you have no record without checking.

Reminders & events (you MUST call a tool to save anything — never claim you did without calling one). Everything the user wants to be reminded of — one-time or recurring — is saved as a reminder (and mirrored to their Google Calendar automatically when connected):
- REPEATING things → "reminder_schedule" (e.g. "every month on the 5th" → repeat=monthly, day_of_month=5; "every weekday at 8am" → repeat=weekly with those weekdays; "daily at 8pm"). Convert phrases into concrete times ("9 pagi" → 09:00). If the user gives no time, omit it — do not invent one — and their default reminder time is applied.
- ONE-TIME / dated things (an appointment, a flight, "besok jam 10", "meeting on Aug 5 at 2pm") → "schedule_event" with a natural date/time, or "reminder_schedule" with repeat=once and a date.
- When the user asks what's on their schedule/calendar/agenda or what's coming up, call BOTH "reminder_list" (their reminders) and "list_calendar" (connected Google Calendar events) and present one merged, time-ordered answer. Do not say you lack calendar access. Present each item at its actual event time, not an earlier notification time.
- Always confirm exactly what you created, and reply in the user's language.`,
		name,
		now.Format("Monday, January 2, 2006 at 3:04 PM"),
		a.owner.Timezone,
	)

	var b strings.Builder
	b.WriteString(base)

	if len(memories) > 0 {
		b.WriteString("\n\nRelevant things you remember about the user:")
		for _, m := range memories {
			b.WriteString("\n- " + m.Content)
		}
	}

	if len(enabledSkills) > 0 {
		b.WriteString("\n\nThe user has enabled these skills:")
		for _, sk := range enabledSkills {
			if sk.Prompt != "" {
				b.WriteString(fmt.Sprintf("\n\n## %s\n%s", sk.Name, sk.Prompt))
			}
		}
	}

	// Persona goes last so it has recency and stays authoritative over the
	// default tone and any skill prompts above — every reply must respect it.
	if personaPrompt != "" {
		b.WriteString("\n\n" + personaPrompt)
	}
	return b.String()
}

func addUsage(total *llm.Usage, u llm.Usage) {
	total.PromptTokens += u.PromptTokens
	total.CompletionTokens += u.CompletionTokens
	total.TotalTokens += u.TotalTokens
}
