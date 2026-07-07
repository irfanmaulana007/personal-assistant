# NLP & Intent Parsing

## Overview

The intent parser converts natural language messages into structured `Intent` objects that the capability router can dispatch.

## Phase 1: Regex-Based Parsing (MVP)

Simple, fast, no external dependencies. Good enough for a personal assistant where the user learns the command patterns.

### Architecture

```go
type RegexParser struct {
    patterns map[string][]PatternRule
}

type PatternRule struct {
    Pattern    *regexp.Regexp
    Action     string
    Extractors []EntityExtractor
}

type EntityExtractor struct {
    Name    string
    Pattern *regexp.Regexp
    Group   int
}
```

### Pattern Matching Flow

1. Iterate through capability patterns in priority order
2. First match wins
3. Extract entities using sub-patterns
4. Return structured Intent

```go
func (p *RegexParser) Parse(text string) (*Intent, error) {
    normalized := strings.ToLower(strings.TrimSpace(text))

    for capability, rules := range p.patterns {
        for _, rule := range rules {
            if rule.Pattern.MatchString(normalized) {
                entities := extractEntities(normalized, rule.Extractors)
                return &Intent{
                    Capability: capability,
                    Action:     rule.Action,
                    Entities:   entities,
                    Confidence: 1.0, // regex is binary match
                    Raw:        text,
                }, nil
            }
        }
    }

    return &Intent{
        Capability: "unknown",
        Confidence: 0.0,
        Raw:        text,
    }, nil
}
```

### Pattern Examples

See individual capability docs for full pattern lists:
- [Calendar patterns](../capabilities/calendar-management.md)
- [Email patterns](../capabilities/email-management.md)
- [Reminder patterns](../capabilities/reminders-todos.md)
- [Knowledge patterns](../capabilities/knowledge-base.md)

### Limitations of Regex

- Cannot handle complex or ambiguous sentences
- Brittle to variations in phrasing
- No context awareness (multi-turn)
- Hard to maintain as patterns grow

## Phase 2: LLM-Based Parsing

Use Claude API for natural language understanding. Better accuracy, handles ambiguity, supports multi-turn context.

### Architecture

```go
type LLMParser struct {
    client  *anthropic.Client
    history []anthropic.Message // conversation context
}

func (p *LLMParser) Parse(text string) (*Intent, error) {
    resp, err := p.client.Messages.New(context.Background(), anthropic.MessageNewParams{
        Model:     "claude-haiku-4-5-20251001", // fast + cheap
        System:    systemPrompt,
        Messages:  append(p.history, userMessage(text)),
        MaxTokens: 256,
    })

    // Parse structured JSON response from Claude
    var intent Intent
    json.Unmarshal([]byte(resp.Content[0].Text), &intent)
    return &intent, nil
}
```

### System Prompt

```
You are an intent parser for a personal assistant. Given a user message, 
extract the intent as JSON with these fields:
- capability: one of "calendar", "email", "reminder", "knowledge", "weather", "search", "unknown"
- action: one of "list", "get", "create", "update", "delete", "search"
- entities: key-value pairs of extracted parameters
- confidence: 0.0 to 1.0

Respond with only the JSON object, no explanation.
```

### Hybrid Approach

Use regex first (fast, free), fall back to LLM for unmatched messages:

```go
func (p *HybridParser) Parse(text string) (*Intent, error) {
    // Try regex first
    intent, err := p.regex.Parse(text)
    if err == nil && intent.Confidence > 0 {
        return intent, nil
    }

    // Fall back to LLM
    return p.llm.Parse(text)
}
```

## Ambiguity Handling

When the parser is uncertain:

| Confidence | Action |
|-----------|--------|
| > 0.8 | Execute directly |
| 0.5 - 0.8 | Confirm with user: "Did you mean...?" |
| < 0.5 | Ask for clarification |
| 0.0 | Show help message |

## Multi-Turn Context (Phase 2+)

Track conversation state for follow-up messages:

```
User: "What's on my calendar today?"
Bot: [shows 3 events]
User: "Cancel the second one"  ← needs context from previous turn
```

Context is stored in-memory per session with a TTL (e.g., 5 minutes).
