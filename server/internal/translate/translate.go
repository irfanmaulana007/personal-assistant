// Package translate normalizes user-supplied labels (reminder titles,
// bucket-list titles/notes) into English, regardless of the input language, using the
// configured LLM. It is intentionally fail-soft: any error, a missing API key,
// or empty input returns the original text unchanged so a create/update never
// fails just because translation was unavailable.
package translate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
)

// ErrNotConfigured is returned by Between when no LLM API key is configured.
var ErrNotConfigured = errors.New("llm api key not configured")

// timeout bounds a single translation call so a create/update is never blocked
// for long on a slow provider (translation is short — a title or a note).
const timeout = 20 * time.Second

// Translator turns arbitrary-language text into English via the configured LLM.
type Translator struct {
	settings *settings.Service
	client   *llm.Client
	log      *slog.Logger
}

// New creates a Translator. It resolves the LLM config per call from settings,
// so provider/key changes take effect without a restart.
func New(s *settings.Service, c *llm.Client, log *slog.Logger) *Translator {
	return &Translator{settings: s, client: c, log: log.With("component", "translate")}
}

const titlePrompt = `You normalize short labels (reminder and to-do titles) into English.
Translate the text into natural English if it is in another language.
Return a clean, concise, properly capitalized title.
Keep proper nouns, brand names, numbers, times, and dates intact.
If the text is already good English, return it unchanged aside from capitalization.
Respond with ONLY the resulting title — no surrounding quotes, no trailing punctuation, no explanation.`

const textPrompt = `You normalize user notes into English.
Translate the text into natural English if it is in another language, preserving the meaning, names, numbers, and line breaks.
If the text is already English, return it unchanged.
Respond with ONLY the translated text — no surrounding quotes and no explanation.`

// Title normalizes a title/name to English with proper capitalization.
func (t *Translator) Title(ctx context.Context, s string) string {
	return t.run(ctx, titlePrompt, s)
}

// Text normalizes free-form text (e.g. a note) to English, keeping its natural
// sentence casing.
func (t *Translator) Text(ctx context.Context, s string) string {
	return t.run(ctx, textPrompt, s)
}

// run performs one translation, returning the original string on any problem.
func (t *Translator) run(ctx context.Context, system, s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	cfg, err := t.settings.LLMConfig(ctx)
	if err != nil || cfg.APIKey == "" {
		return s // not configured — store as-is
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := t.client.Complete(cctx, cfg, []llm.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: s},
	}, nil)
	if err != nil {
		t.log.Warn("translation failed; storing original text", "error", err)
		return s
	}

	out := clean(res.Message.Content)
	if out == "" || suspicious(out, s) {
		return s
	}
	return out
}

// pairPrompt drives a single, bidirectional translation between two languages
// for the group Translator skill. The model detects which of the two languages
// the message is in and translates it into the other, returning a strict JSON
// object so the caller can label both sides deterministically.
const pairPrompt = `You translate messages in a WhatsApp group that uses exactly two languages.
Language A is "%s". Language B is "%s".
The message you are given is written in either language A or language B.
Detect which one it is, then translate the message into the OTHER language.
Preserve the meaning, tone, names, numbers, and any emoji. Do not add anything, explain, or answer the message — only translate it.
Respond with ONLY a compact one-line JSON object, no code fences and nothing else:
{"source":"A" or "B","translation":"<the message translated into the other language>"}
Here "source" is the language the message is ALREADY written in (use the single letter A or B).`

// pairResult is the model's structured reply for Between.
type pairResult struct {
	Source      string `json:"source"`
	Translation string `json:"translation"`
}

// Between detects which of langA/langB the text is written in and translates it
// into the other. It returns the resolved source language (equal to langA or
// langB) and the translated text. On success source is always one of the two
// inputs; if the model's reply cannot be parsed as JSON, source is returned
// empty and translated holds the best-effort raw output so the caller can still
// show something. It never mutates global state and is safe to call per message.
func (t *Translator) Between(ctx context.Context, langA, langB, text string) (source, translated string, err error) {
	if strings.TrimSpace(text) == "" {
		return "", "", errors.New("empty text")
	}
	cfg, err := t.settings.LLMConfig(ctx)
	if err != nil {
		return "", "", err
	}
	if cfg.APIKey == "" {
		return "", "", ErrNotConfigured
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := t.client.Complete(cctx, cfg, []llm.Message{
		{Role: "system", Content: fmt.Sprintf(pairPrompt, langA, langB)},
		{Role: "user", Content: text},
	}, nil)
	if err != nil {
		return "", "", err
	}

	raw := stripFences(clean(res.Message.Content))
	var pr pairResult
	if err := json.Unmarshal([]byte(raw), &pr); err != nil {
		// Model didn't honour the JSON contract. Fall back to showing the raw
		// output as the translation with an unknown source, rather than failing.
		if out := strings.TrimSpace(raw); out != "" {
			return "", out, nil
		}
		return "", "", errors.New("empty translation")
	}
	translated = strings.TrimSpace(pr.Translation)
	if translated == "" {
		return "", "", errors.New("empty translation")
	}
	switch strings.ToUpper(strings.TrimSpace(pr.Source)) {
	case "B":
		source = langB
	default:
		// "A" or anything unexpected defaults to language A as the source.
		source = langA
	}
	return source, translated, nil
}

// stripFences removes a wrapping ```-fenced block (optionally tagged, e.g.
// ```json) that a model sometimes puts around JSON output.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		// Drop an optional language tag on the opening fence line.
		if !strings.Contains(s[:i], "{") {
			s = s[i+1:]
		}
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}

// clean trims whitespace and strips a single pair of wrapping quotes the model
// sometimes adds.
func clean(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	return s
}

// suspicious rejects a result that looks like a refusal or commentary rather
// than a translated label: far longer than the input for a short input.
func suspicious(out, in string) bool {
	return len(out) > 3*len(in)+40
}
