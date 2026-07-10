package whatsapp

import (
	"regexp"
	"strconv"
	"strings"
)

// WhatsApp understands its own lightweight markup, not Markdown:
//
//	*bold*  _italic_  ~strikethrough~  ```monospace```
//
// The LLM, however, writes Markdown (**bold**, `code`, # headings, - bullets,
// [text](url) links). Sent verbatim, that markup shows up literally in the
// chat — a wall of stray "*", "#" and "-" characters that the user has to read
// around. toWhatsAppMarkup rewrites a Markdown reply into the closest WhatsApp
// equivalent so the formatting renders instead of cluttering the message.
//
// The web channel keeps rendering the original Markdown; only WhatsApp-bound
// text passes through here.
func toWhatsAppMarkup(s string) string {
	if s == "" {
		return s
	}

	// Protect code so its contents are never touched by the emphasis/heading
	// passes below. Fenced blocks first, then inline spans.
	var codes []string
	stash := func(code string) string {
		codes = append(codes, code)
		return "\x00" + strconv.Itoa(len(codes)-1) + "\x00"
	}

	// Fenced code blocks: ```lang\n…\n``` → ```…``` (WhatsApp monospace, no lang tag).
	s = fencedCode.ReplaceAllStringFunc(s, func(m string) string {
		inner := fencedCode.FindStringSubmatch(m)[1]
		inner = strings.Trim(inner, "\n")
		return stash("```" + inner + "```")
	})
	// Inline code: `x` → ```x``` (WhatsApp has no single-backtick monospace).
	s = inlineCode.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.Trim(m, "`")
		return stash("```" + inner + "```")
	})

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// Horizontal rules carry no meaning in WhatsApp — drop them.
		if hrRule.MatchString(line) {
			lines[i] = ""
			continue
		}
		// Headings → bold, so they still stand out. Use the bold placeholder
		// (\x01) rather than a literal "*" so the italic pass below can't
		// mistake this single star for emphasis.
		if m := heading.FindStringSubmatch(line); m != nil {
			line = "\x01" + strings.TrimSpace(m[2]) + "\x01"
		}
		// Unordered bullets (-, *, +) → a real bullet glyph.
		if m := bullet.FindStringSubmatch(line); m != nil {
			line = m[1] + "• " + m[2]
		}
		// Blockquotes: WhatsApp has no quote markup — keep the text, drop ">".
		if m := blockquote.FindStringSubmatch(line); m != nil {
			line = m[1]
		}
		lines[i] = line
	}
	s = strings.Join(lines, "\n")

	// Emphasis. Bold must be handled before italic, otherwise the single-*
	// italic pass would chew into **bold**. Park bold in a placeholder first.
	s = boldStar.ReplaceAllString(s, "\x01${1}\x01")
	s = boldUnder.ReplaceAllString(s, "\x01${1}\x01")
	s = italicStar.ReplaceAllString(s, "_${1}_")
	s = strings.ReplaceAll(s, "\x01", "*")

	// Strikethrough: ~~x~~ → ~x~.
	s = strike.ReplaceAllString(s, "~${1}~")

	// Links: [text](url) → text (url).
	s = link.ReplaceAllString(s, "${1} (${2})")

	// Restore code spans.
	s = codePlaceholder.ReplaceAllStringFunc(s, func(m string) string {
		idx, err := strconv.Atoi(strings.Trim(m, "\x00"))
		if err != nil || idx < 0 || idx >= len(codes) {
			return m
		}
		return codes[idx]
	})

	return s
}

var (
	fencedCode      = regexp.MustCompile("(?s)```[^\n]*\n?(.*?)```")
	inlineCode      = regexp.MustCompile("`[^`\n]+`")
	codePlaceholder = regexp.MustCompile("\x00\\d+\x00")
	hrRule          = regexp.MustCompile(`^\s*(?:---+|\*\*\*+|___+)\s*$`)
	heading         = regexp.MustCompile(`^\s*(#{1,6})\s+(.*?)\s*$`)
	bullet          = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	blockquote      = regexp.MustCompile(`^\s*>\s?(.*)$`)
	boldStar        = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	boldUnder       = regexp.MustCompile(`__([^_\n]+)__`)
	italicStar      = regexp.MustCompile(`\*([^*\n]+)\*`)
	strike          = regexp.MustCompile(`~~([^~\n]+)~~`)
	link            = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
)
