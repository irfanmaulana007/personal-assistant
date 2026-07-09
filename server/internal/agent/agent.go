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
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/server/internal/config"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/skills"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// ErrNotConfigured is returned when no LLM API key has been configured.
var ErrNotConfigured = errors.New("llm api key not configured")

// maxIterations bounds the tool-calling loop to avoid runaway LLM/tool cycles.
const maxIterations = 5

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
	router   *capability.Router
	owner    config.OwnerConfig
	provider ToolProvider // optional; extra tools (may be nil)
	log      *slog.Logger
}

// New creates an agent. provider may be nil (no extra tools).
func New(client *llm.Client, settingsSvc *settings.Service, skillsSvc *skills.Service, router *capability.Router, owner config.OwnerConfig, provider ToolProvider, log *slog.Logger) *Agent {
	return &Agent{
		client:   client,
		settings: settingsSvc,
		skills:   skillsSvc,
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
}

// Result is the outcome of an agent run.
type Result struct {
	Reply string
	Usage llm.Usage
	Model string
	Tools []ToolInvocation // tools invoked during the run, in order
}

// Message is a prior conversation turn used as context.
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// Run executes the tool-calling loop for a single user message. history holds
// prior turns (oldest first) for conversational context. image, when non-empty,
// is a data: URL attached to the user message (requires a vision-capable model).
func (a *Agent) Run(ctx context.Context, userMessage string, history []Message, image string) (*Result, error) {
	cfg, err := a.settings.LLMConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve llm config: %w", err)
	}
	if cfg.APIKey == "" {
		return nil, ErrNotConfigured
	}

	// Resolve the user's enabled skills: their prompts enrich the system
	// prompt and their tools are added to the tool list.
	var enabledSkills []store.Skill
	if a.skills != nil {
		enabledSkills = a.skills.Enabled(ctx, authctx.UserID(ctx))
	}
	enabledKeys := make([]string, 0, len(enabledSkills))
	for _, sk := range enabledSkills {
		enabledKeys = append(enabledKeys, sk.Key)
	}

	messages := []llm.Message{{Role: "system", Content: a.systemPrompt(enabledSkills)}}
	for _, m := range history {
		messages = append(messages, llm.Message{Role: m.Role, Content: m.Content})
	}
	if image != "" {
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
	var total llm.Usage
	var used []ToolInvocation

	for i := 0; i < maxIterations; i++ {
		res, err := a.client.Complete(ctx, cfg, messages, tools)
		if err != nil {
			return nil, err
		}
		addUsage(&total, res.Usage)

		msg := res.Message
		messages = append(messages, msg)

		if len(msg.ToolCalls) == 0 {
			return &Result{Reply: strings.TrimSpace(msg.Content), Usage: total, Model: cfg.Model, Tools: used}, nil
		}

		for _, tc := range msg.ToolCalls {
			var result string
			if a.provider != nil && a.provider.Handles(tc.Function.Name) {
				result = a.provider.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
			} else {
				result = a.execTool(ctx, tc)
			}
			used = append(used, ToolInvocation{Name: tc.Function.Name, Arguments: tc.Function.Arguments, Result: result})
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    result,
			})
		}
	}

	// Tool budget exhausted — force a final textual answer without tools.
	res, err := a.client.Complete(ctx, cfg, messages, nil)
	if err != nil {
		return nil, err
	}
	addUsage(&total, res.Usage)
	reply := strings.TrimSpace(res.Message.Content)
	if reply == "" {
		reply = "I wasn't able to finish that request. Could you rephrase or break it into smaller steps?"
	}
	return &Result{Reply: reply, Usage: total, Model: cfg.Model, Tools: used}, nil
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

func (a *Agent) systemPrompt(enabledSkills []store.Skill) string {
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
- When a request maps to a tool, call it. For time/datetime arguments, pass natural language such as "5pm", "in 30 minutes", or "tomorrow at 9am" — the tools parse these.
- After a tool returns, summarize the result for the user clearly, concisely, and in a friendly tone.
- If a request is ambiguous or missing a required detail, ask one short clarifying question instead of guessing.
- For general questions or small talk, just reply directly without calling any tool.
- Never claim to have sent an email — the email tool only creates drafts.`,
		name,
		now.Format("Monday, January 2, 2006 at 3:04 PM"),
		a.owner.Timezone,
	)

	if len(enabledSkills) > 0 {
		var b strings.Builder
		b.WriteString(base)
		b.WriteString("\n\nThe user has enabled these skills:")
		for _, sk := range enabledSkills {
			if sk.Prompt != "" {
				b.WriteString(fmt.Sprintf("\n\n## %s\n%s", sk.Name, sk.Prompt))
			}
		}
		return b.String()
	}
	return base
}

func addUsage(total *llm.Usage, u llm.Usage) {
	total.PromptTokens += u.PromptTokens
	total.CompletionTokens += u.CompletionTokens
	total.TotalTokens += u.TotalTokens
}
