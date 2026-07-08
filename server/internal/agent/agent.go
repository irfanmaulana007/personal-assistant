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

	"github.com/irfanmaulana007/personal-assistant/server/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/server/internal/config"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
)

// ErrNotConfigured is returned when no LLM API key has been configured.
var ErrNotConfigured = errors.New("llm api key not configured")

// maxIterations bounds the tool-calling loop to avoid runaway LLM/tool cycles.
const maxIterations = 5

// Agent orchestrates the LLM tool-calling loop.
type Agent struct {
	client   *llm.Client
	settings *settings.Service
	router   *capability.Router
	owner    config.OwnerConfig
	log      *slog.Logger
}

// New creates an agent.
func New(client *llm.Client, settingsSvc *settings.Service, router *capability.Router, owner config.OwnerConfig, log *slog.Logger) *Agent {
	return &Agent{
		client:   client,
		settings: settingsSvc,
		router:   router,
		owner:    owner,
		log:      log.With("component", "agent"),
	}
}

// Result is the outcome of an agent run.
type Result struct {
	Reply string
	Usage llm.Usage
	Model string
	Tools []string // names of tools invoked during the run
}

// Message is a prior conversation turn used as context.
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// Run executes the tool-calling loop for a single user message. history holds
// prior turns (oldest first) for conversational context.
func (a *Agent) Run(ctx context.Context, userMessage string, history []Message) (*Result, error) {
	cfg, err := a.settings.LLMConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve llm config: %w", err)
	}
	if cfg.APIKey == "" {
		return nil, ErrNotConfigured
	}

	messages := []llm.Message{{Role: "system", Content: a.systemPrompt()}}
	for _, m := range history {
		messages = append(messages, llm.Message{Role: m.Role, Content: m.Content})
	}
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})

	tools := toolSchemas()
	var total llm.Usage
	var used []string

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
			used = append(used, tc.Function.Name)
			result := a.execTool(ctx, tc)
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

func (a *Agent) systemPrompt() string {
	loc := a.owner.Location()
	now := time.Now().In(loc)
	name := a.owner.Name
	if name == "" {
		name = "the user"
	}

	return fmt.Sprintf(`You are a helpful personal assistant for %s.
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
}

func addUsage(total *llm.Usage, u llm.Usage) {
	total.PromptTokens += u.PromptTokens
	total.CompletionTokens += u.CompletionTokens
	total.TotalTokens += u.TotalTokens
}
