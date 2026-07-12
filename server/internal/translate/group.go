package translate

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
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

// pairStore reads and writes a group chat's configured language pair (satisfied
// by *settings.Service).
type pairStore interface {
	GroupTranslatePair(ctx context.Context, chatJID string) (langA, langB string)
	SetGroupTranslatePair(ctx context.Context, chatJID, langA, langB string) error
	ClearGroupTranslatePair(ctx context.Context, chatJID string) error
}

// GroupService handles the `/t` translator command in WhatsApp group chats. It
// is deliberately separate from the LLM tool-calling agent: a `/t` message is a
// deterministic, self-contained request (set the language pair, or translate a
// line between the two configured languages), so it short-circuits the agent
// entirely for a fast, predictable reply.
type GroupService struct {
	tr       *Translator
	settings pairStore
	skills   skillChecker
	log      *slog.Logger
}

// NewGroup creates a group translator service.
func NewGroup(tr *Translator, s pairStore, skills skillChecker, log *slog.Logger) *GroupService {
	return &GroupService{tr: tr, settings: s, skills: skills, log: log.With("component", "group-translate")}
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
			"✅ Translator is set for this group: *%s* ↔ *%s*.\nAnyone can now mention me with `/t <message>` and I'll post it in both languages.",
			cmd.langA, cmd.langB,
		), true

	case cmdOff:
		if err := g.settings.ClearGroupTranslatePair(ctx, chatJID); err != nil {
			g.log.Error("clear group translate pair", "chat", chatJID, "error", err)
		}
		return "🌐 Translator languages cleared for this group. Set a new pair with `/t set <language A> <language B>`.", true

	case cmdStatus:
		a, b := g.settings.GroupTranslatePair(ctx, chatJID)
		if a == "" || b == "" {
			return "🌐 No languages set for this group yet. Set them with `/t set Indonesian Japanese` (use your two languages).", true
		}
		return fmt.Sprintf("🌐 This group translates between *%s* and *%s*.\nMention me with `/t <message>` to translate.", a, b), true

	case cmdHelp:
		return helpText(), true

	case cmdTranslate:
		a, b := g.settings.GroupTranslatePair(ctx, chatJID)
		if a == "" || b == "" {
			return "🌐 No languages set for this group yet. First run `/t set Indonesian Japanese` (use your two languages), then `/t <message>` to translate.", true
		}
		if strings.TrimSpace(cmd.text) == "" {
			return helpText(), true // bare "/t" with nothing to translate
		}
		source, translated, err := g.tr.Between(ctx, a, b, cmd.text)
		if err != nil {
			if err == ErrNotConfigured {
				return "The assistant isn't configured yet — set the LLM API key in the web Settings page.", true
			}
			g.log.Warn("group translation failed", "chat", chatJID, "error", err)
			return "Sorry, I couldn't translate that right now. Please try again.", true
		}
		return formatBoth(a, b, source, cmd.text, translated), true
	}

	return "", false
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
	cmdStatus                   // "/t status" — show the current pair
	cmdHelp                     // "/t help" — usage
)

// command is a parsed `/t` invocation.
type command struct {
	kind         cmdKind
	langA, langB string // set only for cmdSet
	text         string // the payload to translate (cmdTranslate)
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

// --- reply formatting ---

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
	b.WriteString("• `/t status` — show the current languages\n")
	b.WriteString("• `/t off` — clear the languages\n\n")
	b.WriteString("Always mention me together with the command. I reply with the message in both languages so everyone follows along.")
	return b.String()
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
