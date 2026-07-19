package whatsapp

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
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

// FormatGrammarReply rewrites an English-Tutor agent reply for WhatsApp.
//
// The English Tutor skill instructs the model to begin an English reply with the
// grammatically corrected version of the user's message, wrapped between the
// markers [[grammar]] and [[/grammar]] (see the seeded "english_tutor" skill).
// Sent verbatim, those markers show up literally in the WhatsApp chat — the web
// client parses and hides them, but WhatsApp does not. This replaces the marker
// block with a compact, readable correction card and returns Markdown (which
// SendMessage then converts to WhatsApp markup in a single pass):
//
//	📝 **English check**
//	~~i has two apple~~
//	✅ I **have** two **apples**
//
//	<the assistant's actual reply>
//
// The original message is struck through (the "before"), the corrected version
// sits beneath it with the words that changed bolded, and a blank line separates
// the card from the assistant's real reply. When the correction is identical to
// the original (the skill repeats a correct message unchanged) it renders a
// single "looks good" line instead of a redundant before/after.
//
// original is the user's inbound message; reply is the agent's full reply text
// (with the [[grammar]] block). A reply that carries no grammar block — a
// non-English turn, or the skill disabled — is returned unchanged, so ordinary
// replies are untouched.
func FormatGrammarReply(original, reply string) string {
	m := grammarBlock.FindStringSubmatch(reply)
	if m == nil {
		return reply
	}
	corrected := strings.TrimSpace(m[1])
	// Drop the marker block to recover the assistant's actual reply body.
	body := strings.TrimSpace(grammarBlock.ReplaceAllString(reply, ""))
	if corrected == "" {
		return body
	}

	card := renderCorrectionCard(strings.TrimSpace(original), corrected)
	if body == "" {
		return card
	}
	return card + "\n\n" + body
}

// renderCorrectionCard builds the Markdown correction card shown above the reply.
func renderCorrectionCard(original, corrected string) string {
	if original != "" && sameText(original, corrected) {
		return "📝 **English check**\n✅ " + corrected + " — looks good! 👍"
	}
	var b strings.Builder
	b.WriteString("📝 **English check**")
	if original != "" {
		b.WriteString("\n~~" + original + "~~")
	}
	b.WriteString("\n✅ " + highlightChanges(original, corrected))
	return b.String()
}

// highlightChanges returns the corrected sentence with the words that differ
// from the original wrapped in Markdown bold (**word**). It runs a word-level
// longest-common-subsequence diff: corrected words that aren't part of the
// common subsequence with the original are the ones the tutor changed or added,
// so they get emphasized. Matching is case-insensitive and ignores surrounding
// punctuation, while the corrected words are emitted verbatim. Consecutive
// changed words are merged into a single bold run for cleaner rendering. With no
// original to diff against, the corrected text is returned unchanged.
func highlightChanges(original, corrected string) string {
	corr := strings.Fields(corrected)
	if original == "" || len(corr) == 0 {
		return corrected
	}
	changed := changedMask(normalizeTokens(strings.Fields(original)), normalizeTokens(corr))

	var b strings.Builder
	bold := false
	for i, tok := range corr {
		if i > 0 {
			b.WriteByte(' ')
		}
		if changed[i] && !bold {
			b.WriteString("**")
			bold = true
		}
		b.WriteString(tok)
		if bold && (i == len(corr)-1 || !changed[i+1]) {
			b.WriteString("**")
			bold = false
		}
	}
	return b.String()
}

// changedMask returns a per-token flag over corr: true where the corrected token
// is not matched by the longest common subsequence with orig (i.e. it was added
// or changed). orig and corr are the normalized token sequences.
func changedMask(orig, corr []string) []bool {
	n, m := len(orig), len(corr)
	// dp[i][j] = LCS length of orig[i:] and corr[j:].
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if orig[i] == corr[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	changed := make([]bool, m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case orig[i] == corr[j]:
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			changed[j] = true
			j++
		}
	}
	for ; j < m; j++ {
		changed[j] = true
	}
	return changed
}

// normalizeTokens lowercases each token and strips surrounding punctuation so
// the diff compares words, not their trailing commas or capitalization.
func normalizeTokens(toks []string) []string {
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = strings.ToLower(strings.TrimFunc(t, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		}))
	}
	return out
}

// sameText reports whether two strings are equal ignoring case and runs of
// whitespace — used to detect that the tutor returned the message unchanged.
func sameText(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}

var (
	// grammarBlock matches the English Tutor skill's correction wrapper,
	// [[grammar]]corrected sentence[[/grammar]], case-insensitively and across
	// newlines. Mirrors the web client's splitGrammar regex (client Message.tsx).
	grammarBlock = regexp.MustCompile(`(?is)\[\[grammar\]\](.*?)\[\[/grammar\]\]`)

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
