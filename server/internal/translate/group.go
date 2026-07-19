package translate

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// SkillKey is the key of the Translator skill in the skills catalog. The group
// translator only runs when the owner has this skill enabled.
const SkillKey = "translator"

// skillChecker reports which skills a user has enabled (satisfied by store.Store
// via EnabledSkillKeys). Kept as a tiny interface so the group service does not
// depend on the whole store surface.
type skillChecker interface {
	EnabledSkillKeys(ctx context.Context, userID int64) ([]string, error)
}

// pairStore reads and writes a group chat's translator config — the language
// pair plus its display mode and formality preferences (satisfied by
// *settings.Service).
type pairStore interface {
	GroupTranslateConfig(ctx context.Context, chatJID string) (langA, langB, mode, formality string)
	SetGroupTranslatePair(ctx context.Context, chatJID, langA, langB string) error
	SetGroupTranslateMode(ctx context.Context, chatJID, mode string) error
	SetGroupTranslateFormality(ctx context.Context, chatJID, formality string) error
	ClearGroupTranslatePair(ctx context.Context, chatJID string) error
}

// traceRecorder persists a completed translator run as a /logs trace so every
// `/t` translation is reviewable and can be judged (satisfied by store.Store).
type traceRecorder interface {
	CreateTrace(ctx context.Context, t *store.Trace) (int64, error)
}

// traceJudge scores a freshly recorded trace out of band with the LLM-as-judge
// (satisfied by *eval.Judge). It is fire-and-forget and never blocks the reply.
type traceJudge interface {
	JudgeInline(ctx context.Context, traceID int64)
}

// Display modes for the group translator. modeOnly (the default) posts just the
// translation; modeBoth posts the original message and its translation too.
// These strings are persisted per group via pairStore.
const (
	modeBoth = "both"
	modeOnly = "only"
)

// GroupService handles the `/t` translator command in WhatsApp group chats. It
// is deliberately separate from the LLM tool-calling agent: a `/t` message is a
// deterministic, self-contained request (set the language pair, or translate a
// line between the two configured languages), so it short-circuits the agent
// entirely for a fast, predictable reply.
type GroupService struct {
	tr       *Translator
	settings pairStore
	skills   skillChecker
	traces   traceRecorder // optional: logs each translation to /logs; nil disables it
	judge    traceJudge    // optional: scores logged translations; nil disables it
	log      *slog.Logger
}

// NewGroup creates a group translator service. traces and judge are optional
// (either may be nil): when a trace recorder is supplied, every `/t` translation
// is logged to /logs tagged with the translator skill, and when a judge is also
// supplied each logged translation is scored out of band by the LLM-as-judge.
func NewGroup(tr *Translator, s pairStore, skills skillChecker, traces traceRecorder, judge traceJudge, log *slog.Logger) *GroupService {
	return &GroupService{tr: tr, settings: s, skills: skills, traces: traces, judge: judge, log: log.With("component", "group-translate")}
}

// Handle attempts to service a group message as a `/t` translator command.
//
// It returns (reply, true) when the message is a translator command — the
// caller should send reply and stop (skip the agent). It returns ("", false)
// when the message is not a `/t` command, so the caller proceeds with the normal
// assistant. userID is the owner whose Translator-skill toggle gates the
// feature; chatJID identifies the group so its language pair is isolated.
func (g *GroupService) Handle(ctx context.Context, userID int64, chatJID, rawText string) (string, bool) {
	cmd, ok := parseCommand(rawText)
	if !ok {
		return "", false // not a translator command — let the agent handle it
	}

	// It is a `/t` command: from here every path returns handled=true so the
	// agent never sees the raw command text.
	if !g.enabled(ctx, userID) {
		return "🌐 The *Translator* skill is off. Turn it on in the web app (Skills page) to use `/t` in this group.", true
	}

	switch cmd.kind {
	case cmdSet:
		if cmd.langA == "" || cmd.langB == "" {
			return setUsage(), true
		}
		if err := g.settings.SetGroupTranslatePair(ctx, chatJID, cmd.langA, cmd.langB); err != nil {
			g.log.Error("save group translate pair", "chat", chatJID, "error", err)
			return "Sorry, I couldn't save those languages. Please try again.", true
		}
		return fmt.Sprintf(
			"✅ Translator is set for this group: *%s* ↔ *%s*.\nAnyone can now send `/t <message>` (no need to mention me) and I'll reply with the translation.\nUse `/t mode both` to also show the original, or `/t formality casual`/`formal` to set the tone.",
			cmd.langA, cmd.langB,
		), true

	case cmdOff:
		if err := g.settings.ClearGroupTranslatePair(ctx, chatJID); err != nil {
			g.log.Error("clear group translate pair", "chat", chatJID, "error", err)
		}
		return "🌐 Translator languages cleared for this group. Set a new pair with `/t set <language A> <language B>`.", true

	case cmdStatus:
		a, b, mode, formality := g.settings.GroupTranslateConfig(ctx, chatJID)
		if a == "" || b == "" {
			return "🌐 No languages set for this group yet. Set them with `/t set Indonesian Japanese` (use your two languages).", true
		}
		return statusText(a, b, mode, formality), true

	case cmdMode:
		_, _, mode, _ := g.settings.GroupTranslateConfig(ctx, chatJID)
		next := cmd.mode
		if !cmd.hasArg {
			next = toggleMode(mode) // bare "/t mode" flips the current setting
		} else if next == "" {
			return modeUsage(), true // an argument was given but not recognised
		}
		if err := g.settings.SetGroupTranslateMode(ctx, chatJID, next); err != nil {
			g.log.Error("save group translate mode", "chat", chatJID, "error", err)
			return "Sorry, I couldn't update the display mode. Please try again.", true
		}
		return modeConfirm(next), true

	case cmdFormality:
		if !cmd.hasArg {
			_, _, _, formality := g.settings.GroupTranslateConfig(ctx, chatJID)
			return formalityStatus(formality), true // bare "/t formality" reports the current setting
		}
		if cmd.formality == "" {
			return formalityUsage(), true // an argument was given but not recognised
		}
		if err := g.settings.SetGroupTranslateFormality(ctx, chatJID, cmd.formality); err != nil {
			g.log.Error("save group translate formality", "chat", chatJID, "error", err)
			return "Sorry, I couldn't update the formality. Please try again.", true
		}
		return formalityConfirm(cmd.formality), true

	case cmdHelp:
		return helpText(), true

	case cmdTranslate:
		a, b, mode, formality := g.settings.GroupTranslateConfig(ctx, chatJID)
		if a == "" || b == "" {
			return "🌐 No languages set for this group yet. First run `/t set Indonesian Japanese` (use your two languages), then `/t <message>` to translate.", true
		}
		if strings.TrimSpace(cmd.text) == "" {
			return helpText(), true // bare "/t" with nothing to translate
		}
		start := time.Now()
		res, err := g.tr.Between(ctx, a, b, formality, cmd.text)
		latencyMs := int(time.Since(start).Milliseconds())
		if err != nil {
			// ErrNotConfigured means the LLM was never called (no API key), so
			// there is nothing to log; every other error reached the LLM and is
			// recorded as a failed translation trace.
			if err == ErrNotConfigured {
				return "The assistant isn't configured yet — set the LLM API key in the web Settings page.", true
			}
			g.recordTranslation(ctx, userID, chatJID, cmd.text, res, err, latencyMs)
			g.log.Warn("group translation failed", "chat", chatJID, "error", err)
			return "Sorry, I couldn't translate that right now. Please try again.", true
		}
		g.recordTranslation(ctx, userID, chatJID, cmd.text, res, nil, latencyMs)
		return formatTranslation(a, b, res.Source, cmd.text, res.Translated, mode), true
	}

	return "", false
}

// recordTranslation persists one group `/t` translation as a /logs trace tagged
// with the translator skill, then kicks off out-of-band judging on success. It
// is fail-soft: it no-ops when no trace recorder is wired and never affects the
// reply the group sees. input is the original message, res the translation
// outcome (its Translated field is the output shown to the group), and runErr is
// non-nil when the translation LLM call failed after being reached.
func (g *GroupService) recordTranslation(ctx context.Context, userID int64, chatJID, input string, res BetweenResult, runErr error, latencyMs int) {
	if g.traces == nil {
		return
	}
	t := &store.Trace{
		UserID:           userID,
		Platform:         "whatsapp",
		Source:           "chat",
		Input:            input,
		Output:           res.Translated,
		Model:            res.Model,
		PromptTokens:     res.PromptTokens,
		CompletionTokens: res.CompletionTokens,
		TotalTokens:      res.TotalTokens,
		LatencyMs:        latencyMs,
		Skills:           []string{SkillKey},
		Status:           "ok",
	}
	if runErr != nil {
		t.Status = "error"
		t.Error = runErr.Error()
	}
	traceID, err := g.traces.CreateTrace(ctx, t)
	if err != nil {
		g.log.Warn("record translation trace", "chat", chatJID, "error", err)
		return
	}
	// Only a successful translation has an output for the judge to score.
	if g.judge != nil && runErr == nil {
		g.judge.JudgeInline(ctx, traceID)
	}
}

// enabled reports whether the owner has the Translator skill turned on.
func (g *GroupService) enabled(ctx context.Context, userID int64) bool {
	if g.skills == nil {
		return false
	}
	keys, err := g.skills.EnabledSkillKeys(ctx, userID)
	if err != nil {
		g.log.Warn("resolve enabled skills", "error", err)
		return false
	}
	for _, k := range keys {
		if k == SkillKey {
			return true
		}
	}
	return false
}

// --- command parsing ---

type cmdKind int

const (
	cmdTranslate cmdKind = iota // "/t <message>" — translate a line
	cmdSet                      // "/t set <A> <B>" — configure the pair
	cmdOff                      // "/t off" — clear the pair
	cmdStatus                   // "/t status" — show the current settings
	cmdMode                     // "/t mode [both|only]" — set/toggle display mode
	cmdFormality                // "/t formality [asis|casual|formal]" — set the register
	cmdHelp                     // "/t help" — usage
)

// command is a parsed `/t` invocation.
type command struct {
	kind         cmdKind
	langA, langB string // set only for cmdSet
	text         string // the payload to translate (cmdTranslate)
	mode         string // canonical display mode for cmdMode (modeBoth/modeOnly), "" if none/unrecognised
	formality    string // canonical formality for cmdFormality, "" if none/unrecognised
	hasArg       bool   // an explicit argument followed cmdMode/cmdFormality (distinguishes "toggle/status" from "bad value")
}

var (
	// mentionRE matches a WhatsApp @-mention token as it appears in message text:
	// an "@" followed by a long run of digits (the mentioned phone number). Real
	// words, emails, and short "@1234"-style tokens never take this shape, so
	// stripping these anywhere is safe.
	mentionRE = regexp.MustCompile(`@\d{5,}`)
	// multiSpace collapses runs of spaces/tabs left behind after stripping
	// mentions (newlines are preserved so multi-line messages survive).
	multiSpace = regexp.MustCompile(`[ \t]{2,}`)
	// pairSeps splits a "set" argument into two language names when the user
	// joins them with a word/symbol rather than a plain space.
	pairSeps = regexp.MustCompile(`(?i)\s*(?:↔|<->|<>|->|→|/|,|;|\||&|\band\b|\bdan\b)\s*`)
)

// IsCommand reports whether a group message is a translator command (`/t` or
// `/translate`, after any @-mentions are stripped). Transports use it to let a
// self-contained `/t` command through a group without the assistant being
// @mentioned, while ordinary prompts still require a mention.
func IsCommand(raw string) bool {
	_, ok := parseCommand(raw)
	return ok
}

// parseCommand recognises a `/t` (or `/translate`) command in a group message,
// after removing any @-mentions of the assistant. It returns ok=false when the
// message is not a translator command.
func parseCommand(raw string) (command, bool) {
	s := stripMentions(raw)
	if s == "" {
		return command{}, false
	}

	// The command keyword must be the first token: "/t" or "/translate".
	rest, ok := matchKeyword(s)
	if !ok {
		return command{}, false
	}
	rest = strings.TrimSpace(rest)

	// Sub-command dispatch on the first word of the remainder.
	first, remainder := splitFirstWord(rest)
	switch strings.ToLower(first) {
	case "set", "setup", "use":
		a, b := parseLangPair(remainder)
		return command{kind: cmdSet, langA: a, langB: b}, true
	case "off", "clear", "reset", "disable", "stop":
		return command{kind: cmdOff}, true
	case "status", "show", "current", "lang", "langs", "languages":
		return command{kind: cmdStatus}, true
	case "mode", "display", "view":
		m, hasArg := parseMode(remainder)
		return command{kind: cmdMode, mode: m, hasArg: hasArg}, true
	case "formality", "tone", "register", "style":
		f, hasArg := parseFormality(remainder)
		return command{kind: cmdFormality, formality: f, hasArg: hasArg}, true
	case "help", "?":
		return command{kind: cmdHelp}, true
	default:
		return command{kind: cmdTranslate, text: rest}, true
	}
}

// stripMentions removes @-mention tokens from anywhere in the text and collapses
// the whitespace they leave behind, preserving line breaks.
func stripMentions(s string) string {
	s = mentionRE.ReplaceAllString(s, " ")
	s = multiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// matchKeyword checks whether s begins with the "/t" or "/translate" keyword as
// a whole token, returning the remaining text after the keyword. "/translate"
// is tried first so it isn't mis-read as "/t" followed by "ranslate".
func matchKeyword(s string) (rest string, ok bool) {
	for _, k := range []string{"/translate", "/t"} {
		if !strings.HasPrefix(strings.ToLower(s), k) {
			continue
		}
		after := s[len(k):]
		// The keyword must be the whole first token: either the message is
		// exactly the keyword, or the next character is whitespace.
		if after == "" {
			return "", true
		}
		if r := after[0]; r == ' ' || r == '\t' || r == '\n' {
			return after, true
		}
	}
	return "", false
}

// splitFirstWord returns the first whitespace-delimited word and the untrimmed
// remainder after it.
func splitFirstWord(s string) (first, rest string) {
	s = strings.TrimLeft(s, " \t")
	i := strings.IndexAny(s, " \t\n")
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i:]
}

// parseLangPair extracts two language names from a "set" argument. It first
// tries a joining separator (comma, slash, "and", ↔, …) so multi-word names
// like "Bahasa Indonesia" survive; otherwise it falls back to the first two
// space-separated tokens. Returns empty strings when it can't find two names.
func parseLangPair(s string) (a, b string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if parts := pairSeps.Split(s, -1); len(parts) == 2 {
		a, b = tidyLang(parts[0]), tidyLang(parts[1])
		if a != "" && b != "" {
			return a, b
		}
	}
	fields := strings.Fields(s)
	if len(fields) == 2 {
		return tidyLang(fields[0]), tidyLang(fields[1])
	}
	return "", ""
}

// tidyLang trims and capitalises the first letter of a language name for
// consistent display (e.g. "japanese" → "Japanese").
func tidyLang(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	return string(r)
}

// parseMode maps a "/t mode" argument to a canonical display mode. hasArg is
// false when no argument was given (the caller then toggles). When an argument
// is present but unrecognised, mode is "" and hasArg is true so the caller can
// show usage rather than silently guessing.
func parseMode(s string) (mode string, hasArg bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", false
	}
	switch s {
	case "both", "full", "dual", "two", "all", "input", "bilingual", "original":
		return modeBoth, true
	case "only", "single", "translation", "translated", "translation-only", "translationonly", "translated-only", "output", "just":
		return modeOnly, true
	}
	return "", true
}

// parseFormality maps a "/t formality" argument to a canonical formality
// (translate.Formality*). hasArg mirrors parseMode: false when no argument was
// given (report current), and true with an empty result when the argument was
// not recognised (show usage). Aliases cover common English and Indonesian words.
func parseFormality(s string) (formality string, hasArg bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", false
	}
	switch s {
	case "asis", "as-is", "as", "original", "keep", "same", "default", "normal", "auto", "apaadanya":
		return FormalityAsIs, true
	case "casual", "informal", "relaxed", "friendly", "santai", "akrab":
		return FormalityCasual, true
	case "formal", "polite", "professional", "respectful", "sopan", "resmi":
		return FormalityFormal, true
	}
	return "", true
}

// normalizeMode resolves a stored mode (possibly "" from an older row or an
// unset default) to a concrete mode. The default is modeOnly — a group shows
// just the translation until someone opts into modeBoth.
func normalizeMode(m string) string {
	if m == modeBoth {
		return modeBoth
	}
	return modeOnly
}

// toggleMode flips between the two display modes, treating any unset/unknown
// current value as the default (modeOnly).
func toggleMode(current string) string {
	if normalizeMode(current) == modeBoth {
		return modeOnly
	}
	return modeBoth
}

// --- reply formatting ---

// formatTranslation renders a translation reply according to the group's
// display mode: modeBoth shows the original and the translation; modeOnly shows
// just the translation.
func formatTranslation(langA, langB, source, original, translated, mode string) string {
	if normalizeMode(mode) == modeOnly {
		return formatOnly(langA, langB, source, translated)
	}
	return formatBoth(langA, langB, source, original, translated)
}

// formatOnly renders just the translation, prefixed with the target language's
// flag when it can be determined (falling back to the 🌐 globe otherwise) so the
// line still signals which language it is in.
func formatOnly(langA, langB, source, translated string) string {
	if source == "" {
		return "🌐 " + translated
	}
	target := langB
	if strings.EqualFold(source, langB) {
		target = langA
	}
	if ft := languageFlag(target); ft != "" {
		return ft + " " + translated
	}
	return "🌐 " + translated
}

// formatBoth renders a translation showing both languages, source line first.
// It labels each line with a flag emoji when both languages are recognised, and
// falls back to plain "Language:" labels otherwise so the output is always
// self-explanatory. When the source language is unknown (model didn't report
// it), it shows just the translation.
func formatBoth(langA, langB, source, original, translated string) string {
	if source == "" {
		return "🌐 " + translated
	}
	target := langB
	if strings.EqualFold(source, langB) {
		target = langA
	}

	fs, ft := languageFlag(source), languageFlag(target)
	if fs != "" && ft != "" {
		return fs + " " + original + "\n" + ft + " " + translated
	}
	return source + ": " + original + "\n" + target + ": " + translated
}

// setUsage is shown when `/t set` is given without two clear language names.
func setUsage() string {
	return "To set this group's languages: `/t set <language A> <language B>` — for example `/t set Indonesian Japanese`."
}

// helpText explains the translator commands.
func helpText() string {
	var b strings.Builder
	b.WriteString("*Translator* — chat across two languages in this group.\n\n")
	b.WriteString("• `/t set Indonesian Japanese` — set the two languages\n")
	b.WriteString("• `/t <message>` — translate a message (auto-detects the direction)\n")
	b.WriteString("• `/t mode both` / `/t mode only` — show the original + translation, or just the translation (`/t mode` alone toggles)\n")
	b.WriteString("• `/t formality asis` / `casual` / `formal` — keep the tone, or make it casual or formal\n")
	b.WriteString("• `/t status` — show the current settings\n")
	b.WriteString("• `/t off` — clear the languages\n\n")
	b.WriteString("Grammar is always corrected in the translation. Just send `/t …` — no need to mention me. To ask me anything else, mention me.")
	return b.String()
}

// statusText summarises a group's current translator settings.
func statusText(langA, langB, mode, formality string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🌐 This group translates between *%s* and *%s*.\n", langA, langB))
	if normalizeMode(mode) == modeOnly {
		b.WriteString("• Display: *translation only*\n")
	} else {
		b.WriteString("• Display: *original + translation*\n")
	}
	b.WriteString("• Formality: *" + formalityLabel(formality) + "* (grammar always corrected)\n")
	b.WriteString("\nSend `/t <message>` to translate — no need to mention me.")
	return b.String()
}

// modeConfirm acknowledges a display-mode change.
func modeConfirm(mode string) string {
	if normalizeMode(mode) == modeOnly {
		return "✅ Now showing *only the translation* for `/t` messages. Use `/t mode both` to include the original too."
	}
	return "✅ Now showing *both the original and the translation* for `/t` messages. Use `/t mode only` for translation-only."
}

// modeUsage is shown when `/t mode` is given an unrecognised value.
func modeUsage() string {
	return "Set how translations are shown: `/t mode both` (original + translation) or `/t mode only` (translation only). Send `/t mode` alone to toggle."
}

// formalityLabel renders a formality value for display.
func formalityLabel(formality string) string {
	switch formality {
	case FormalityCasual:
		return "casual"
	case FormalityFormal:
		return "formal"
	default:
		return "as-is"
	}
}

// formalityConfirm acknowledges a formality change.
func formalityConfirm(formality string) string {
	switch formality {
	case FormalityCasual:
		return "✅ Formality set to *casual* — I'll make translations friendly and relaxed (grammar is always corrected)."
	case FormalityFormal:
		return "✅ Formality set to *formal* — I'll make translations polite and formal (grammar is always corrected)."
	default:
		return "✅ Formality set to *as-is* — I'll keep each message's original tone (grammar is always corrected)."
	}
}

// formalityStatus reports the current formality and the available options.
func formalityStatus(formality string) string {
	return fmt.Sprintf("🌐 Formality is *%s* (grammar is always corrected).\nChange it with `/t formality asis`, `/t formality casual`, or `/t formality formal`.", formalityLabel(formality))
}

// formalityUsage is shown when `/t formality` is given an unrecognised value.
func formalityUsage() string {
	return "Set the tone with `/t formality asis` (keep as-is), `/t formality casual`, or `/t formality formal`. Grammar is always corrected."
}

// languageFlag maps a language name to a flag emoji for the common cases,
// returning "" for anything unrecognised (the caller then falls back to a text
// label). Matching is case-insensitive and covers the language's own name plus
// common Indonesian/English aliases.
func languageFlag(name string) string {
	return langFlags[strings.ToLower(strings.TrimSpace(name))]
}

var langFlags = map[string]string{
	"indonesian": "🇮🇩", "indonesia": "🇮🇩", "bahasa": "🇮🇩", "bahasa indonesia": "🇮🇩",
	"japanese": "🇯🇵", "japan": "🇯🇵", "jepang": "🇯🇵", "nihongo": "🇯🇵",
	"english": "🇬🇧", "inggris": "🇬🇧",
	"german": "🇩🇪", "deutsch": "🇩🇪", "jerman": "🇩🇪",
	"french": "🇫🇷", "francais": "🇫🇷", "français": "🇫🇷", "perancis": "🇫🇷", "prancis": "🇫🇷",
	"spanish": "🇪🇸", "espanol": "🇪🇸", "español": "🇪🇸", "spanyol": "🇪🇸",
	"korean": "🇰🇷", "korea": "🇰🇷", "hangul": "🇰🇷",
	"chinese": "🇨🇳", "mandarin": "🇨🇳", "china": "🇨🇳", "cina": "🇨🇳",
	"arabic": "🇸🇦", "arab": "🇸🇦",
	"thai": "🇹🇭", "thailand": "🇹🇭",
	"vietnamese": "🇻🇳", "vietnam": "🇻🇳",
	"russian": "🇷🇺", "russia": "🇷🇺", "rusia": "🇷🇺",
	"italian": "🇮🇹", "italiano": "🇮🇹", "italia": "🇮🇹",
	"portuguese": "🇵🇹", "portugues": "🇵🇹", "português": "🇵🇹",
	"dutch": "🇳🇱", "belanda": "🇳🇱",
	"hindi": "🇮🇳", "india": "🇮🇳",
	"malay": "🇲🇾", "melayu": "🇲🇾", "malaysia": "🇲🇾",
	"filipino": "🇵🇭", "tagalog": "🇵🇭",
	"turkish": "🇹🇷", "turki": "🇹🇷",
}
