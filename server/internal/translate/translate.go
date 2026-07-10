// Package translate normalizes user-supplied labels (reminder titles, life-goal
// titles/notes) into English, regardless of the input language, using the
// configured LLM. It is intentionally fail-soft: any error, a missing API key,
// or empty input returns the original text unchanged so a create/update never
// fails just because translation was unavailable.
package translate

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
)

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
