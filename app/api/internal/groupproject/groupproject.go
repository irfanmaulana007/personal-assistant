// Package groupproject binds a WhatsApp group to a single app project and lets
// the group's owner manage that binding by chatting with the assistant.
//
// The assistant runs as a linked device on the owner's personal WhatsApp
// account, so it can be present in many groups. This service answers the
// question "which project am I acting as in THIS group?" and lets the owner
// self-assign one. Once a group is bound to a project, every downstream
// capability (memory, skills, reminders, notes, …) is scoped to that project by
// the existing whatsapp_mappings → authctx machinery; a group that is not yet
// bound stays inert — the assistant refuses to act and prompts the owner to
// pick a project rather than silently falling back to a default (which would
// leak across projects via the unscoped "project 0" path).
//
// Like the `/t` group translator, this is a deterministic pre-agent handler: a
// binding command is a self-contained config request, so it short-circuits the
// LLM agent for a fast, predictable, and safe reply (no reliance on a weak model
// to call a tool correctly, and no project-scoped tool ever runs unscoped).
package groupproject

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

// projectStore is the slice of the store this service needs: read/write the
// group→project binding and resolve the owner's projects. Kept small so tests
// can fake it.
type projectStore interface {
	GetWhatsAppMapping(ctx context.Context, jid string) (*store.WhatsAppMapping, error)
	CreateWhatsAppMapping(ctx context.Context, m store.WhatsAppMapping) (*store.WhatsAppMapping, error)
	DeleteWhatsAppMapping(ctx context.Context, id int64) error
	ListProjectsForUser(ctx context.Context, userID int64) ([]store.ProjectSummary, error)
	GetProject(ctx context.Context, id int64) (*store.Project, error)
}

// Service manages the project binding for WhatsApp groups.
type Service struct {
	store projectStore
	log   *slog.Logger
}

// New creates a group-project service over the store.
func New(s projectStore, log *slog.Logger) *Service {
	return &Service{store: s, log: log.With("component", "group-project")}
}

// Handle services a group message for the group→project binding. ownerID is the
// account owner (whose projects are assignable); assigned reports whether the
// group already has a binding (resolved upstream); isOwner reports whether this
// sender is the account owner (only the owner may change the binding).
//
// It returns (reply, true) when the message is fully handled here — the caller
// sends reply and must NOT run the agent. It returns ("", false) only when the
// group is already bound and the message is ordinary chat (or a binding attempt
// by a non-owner), so the caller proceeds to the scoped agent. For an UNBOUND
// group it always returns handled=true: an unbound group must never reach the
// agent, because there is no project to scope it to.
func (s *Service) Handle(ctx context.Context, chatJID, rawText string, ownerID int64, assigned, isOwner bool) (string, bool) {
	cmd := classify(rawText)

	if assigned {
		// A bound group runs the normal scoped agent for everything except an
		// explicit, owner-issued binding change. Ordinary chat — including "which
		// project are you?" — flows to the agent, which knows the bound project
		// from its system prompt and answers naturally.
		switch cmd.kind {
		case cmdAssign, cmdAssignBare, cmdUnassign, cmdList:
			if !isOwner {
				// Don't hijack a non-owner's message on a coincidental keyword match;
				// let the agent handle it.
				return "", false
			}
			return s.handleOwnerCommand(ctx, chatJID, ownerID, cmd, true)
		default:
			return "", false
		}
	}

	// Unbound group: nothing is scoped, so the agent must not run. Every path
	// below returns handled=true.
	switch cmd.kind {
	case cmdAssign, cmdAssignBare, cmdUnassign, cmdList:
		if !isOwner {
			return notOwnerReply(), true
		}
		reply, _ := s.handleOwnerCommand(ctx, chatJID, ownerID, cmd, false)
		return reply, true
	default:
		return s.unassignedPrompt(ctx, ownerID, isOwner), true
	}
}

// handleOwnerCommand executes an owner-issued binding command. bound reports
// whether the group is currently bound (controls a couple of replies). The
// second return is the handled flag: it is false only when an assignBare target
// does not resolve to a real project in an already-bound group — there the
// message was probably ordinary chat that merely began with "project", so we let
// the agent handle it instead of erroring.
func (s *Service) handleOwnerCommand(ctx context.Context, chatJID string, ownerID int64, cmd command, bound bool) (string, bool) {
	switch cmd.kind {
	case cmdList:
		return s.listReply(ctx, chatJID, ownerID), true

	case cmdUnassign:
		if !bound {
			return "🤷 I'm not assigned to any project in this group yet, so there's nothing to detach. Assign me with `project <name>`.", true
		}
		return s.unassign(ctx, chatJID), true

	case cmdAssign, cmdAssignBare:
		p := s.resolveProject(ctx, ownerID, cmd.target)
		if p == nil {
			// A bare "project <text>" that names nothing real inside an already-bound
			// group is almost certainly normal chat — defer to the agent.
			if cmd.kind == cmdAssignBare && bound {
				return "", false
			}
			return s.cantFindReply(ctx, chatJID, ownerID, cmd.target), true
		}
		return s.assign(ctx, chatJID, p), true
	}
	return "", false
}

// assign binds the group to the project (upserting the mapping) and confirms.
func (s *Service) assign(ctx context.Context, chatJID string, p *store.Project) string {
	_, err := s.store.CreateWhatsAppMapping(ctx, store.WhatsAppMapping{
		JID:       chatJID,
		Kind:      "group",
		ProjectID: p.ID,
		// A group can never confer superadmin (mirrors resolveWhatsAppScope); admin
		// gives the group full use of the project's capabilities.
		Role:  store.ProjectRoleAdmin,
		Label: p.Name,
	})
	if err != nil {
		s.log.Error("assign group to project", "chat", chatJID, "project", p.ID, "error", err)
		return "Sorry, I couldn't save that assignment. Please try again."
	}
	s.log.Info("group assigned to project", "chat", chatJID, "project", p.ID, "name", p.Name)
	return fmt.Sprintf(
		"✅ Done — I'm now the assistant for the *%s* project in this group. Everything I do here (memory, skills, reminders, notes) is scoped to *%s* only.\n\nOnly you can change this: `assign to project <name>` to switch, or `unassign` to detach.",
		p.Name, p.Name,
	)
}

// unassign removes the group's binding and confirms.
func (s *Service) unassign(ctx context.Context, chatJID string) string {
	m, err := s.store.GetWhatsAppMapping(ctx, chatJID)
	if err != nil {
		s.log.Error("lookup mapping for unassign", "chat", chatJID, "error", err)
	}
	if m == nil {
		return "🤷 I'm not assigned to any project in this group yet, so there's nothing to detach."
	}
	name := m.Label
	if p, _ := s.store.GetProject(ctx, m.ProjectID); p != nil {
		name = p.Name
	}
	if err := s.store.DeleteWhatsAppMapping(ctx, m.ID); err != nil {
		s.log.Error("delete mapping for unassign", "chat", chatJID, "error", err)
		return "Sorry, I couldn't detach from that project. Please try again."
	}
	s.log.Info("group unassigned from project", "chat", chatJID, "project", m.ProjectID)
	label := "the project"
	if name != "" {
		label = fmt.Sprintf("the *%s* project", name)
	}
	return fmt.Sprintf(
		"✅ I've detached from %s. I'm now unassigned in this group and won't act until you assign me again with `project <name>`.",
		label,
	)
}

// listReply lists the owner's projects, marking the currently bound one.
func (s *Service) listReply(ctx context.Context, chatJID string, ownerID int64) string {
	projects := s.ownerProjects(ctx, ownerID)
	if len(projects) == 0 {
		return "You don't have any projects yet. Create one in the web app first, then assign it here."
	}
	var currentID int64
	if m, _ := s.store.GetWhatsAppMapping(ctx, chatJID); m != nil {
		currentID = m.ProjectID
	}
	var b strings.Builder
	b.WriteString("📂 Your projects:\n")
	b.WriteString(bulletList(projects, currentID))
	b.WriteString("\nAssign me with `project <name>` or `assign to project <name>`.")
	return b.String()
}

// cantFindReply is shown when an explicit assign names a project that does not
// exist, so the owner can pick a real one.
func (s *Service) cantFindReply(ctx context.Context, chatJID string, ownerID int64, target string) string {
	projects := s.ownerProjects(ctx, ownerID)
	if len(projects) == 0 {
		return "You don't have any projects yet. Create one in the web app first, then assign it here."
	}
	var currentID int64
	if m, _ := s.store.GetWhatsAppMapping(ctx, chatJID); m != nil {
		currentID = m.ProjectID
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🤔 I couldn't find a project called %q. Your projects:\n", strings.TrimSpace(target)))
	b.WriteString(bulletList(projects, currentID))
	b.WriteString("\nTry `project <name>` with one of these.")
	return b.String()
}

// unassignedPrompt is the reply for any message in an unbound group. The owner
// gets an actionable prompt listing the projects; anyone else is told to ask the
// owner (project names are not leaked to non-owners).
func (s *Service) unassignedPrompt(ctx context.Context, ownerID int64, isOwner bool) string {
	if !isOwner {
		return "👋 I'm not assigned to a project in this group yet, so I can't help here until the owner picks one. Please ask the group owner to assign me to a project."
	}
	projects := s.ownerProjects(ctx, ownerID)
	var b strings.Builder
	b.WriteString("👋 I'm not assigned to a project in this group yet, so I can't help here until you choose one.\n\n")
	b.WriteString("Assign me (only you, the owner, can):\n")
	b.WriteString("• `project <name>` — e.g. `project Personal`\n")
	b.WriteString("• or `assign to project <name>`\n")
	if len(projects) > 0 {
		b.WriteString("\nYour projects:\n")
		b.WriteString(bulletList(projects, 0))
		b.WriteString("\nSee them anytime with `list projects`.")
	} else {
		b.WriteString("\nYou don't have any projects yet — create one in the web app first.")
	}
	return b.String()
}

// notOwnerReply is shown when a non-owner tries to change the binding in an
// unbound group.
func notOwnerReply() string {
	return "🔒 Only the group owner can assign or change my project. Please ask them to do it."
}

// ownerProjects returns the owner's projects sorted by id (stable display).
func (s *Service) ownerProjects(ctx context.Context, ownerID int64) []store.ProjectSummary {
	projects, err := s.store.ListProjectsForUser(ctx, ownerID)
	if err != nil {
		s.log.Error("list owner projects", "owner", ownerID, "error", err)
		return nil
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].ID < projects[j].ID })
	return projects
}

// bulletList renders projects as bullets, marking the bound one with "(current)".
func bulletList(projects []store.ProjectSummary, currentID int64) string {
	var b strings.Builder
	for _, p := range projects {
		b.WriteString("• *" + p.Name + "*")
		if p.Slug != "" && !strings.EqualFold(p.Slug, p.Name) {
			b.WriteString(" (`" + p.Slug + "`)")
		}
		if p.ID == currentID && currentID != 0 {
			b.WriteString(" — current")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// resolveProject matches a free-text target to one of the owner's projects. It
// tries, in order: the "default" alias (project id 1, the personal project), an
// exact case-insensitive name or slug match, then a unique substring match.
// Returns nil when nothing matches or a substring is ambiguous.
func (s *Service) resolveProject(ctx context.Context, ownerID int64, target string) *store.Project {
	target = strings.TrimSpace(strings.Trim(target, `"'“”`))
	if target == "" {
		return nil
	}
	projects := s.ownerProjects(ctx, ownerID)
	if len(projects) == 0 {
		return nil
	}
	lower := strings.ToLower(target)

	// "default"/"personal"/"1" → the default personal project (id 1).
	if lower == "default" || lower == "personal" || lower == "1" {
		for _, p := range projects {
			if p.ID == 1 {
				return &p.Project
			}
		}
	}

	// Exact name or slug.
	for i := range projects {
		if strings.EqualFold(projects[i].Name, target) || strings.EqualFold(projects[i].Slug, target) {
			return &projects[i].Project
		}
	}

	// Unique substring on name or slug.
	var match *store.Project
	for i := range projects {
		if strings.Contains(strings.ToLower(projects[i].Name), lower) ||
			strings.Contains(strings.ToLower(projects[i].Slug), lower) {
			if match != nil {
				return nil // ambiguous
			}
			match = &projects[i].Project
		}
	}
	return match
}

// --- command classification ---

type cmdKind int

const (
	cmdNone       cmdKind = iota // not a binding command
	cmdStatus                    // asks which project (handled by the agent when bound)
	cmdList                      // list the owner's projects
	cmdAssign                    // explicit "assign to project X" (definite intent)
	cmdAssignBare                // bare "project X" (assign only if X resolves)
	cmdUnassign                  // detach from the current project
)

type command struct {
	kind   cmdKind
	target string // project name for cmdAssign/cmdAssignBare
}

var (
	// mentionRE strips WhatsApp @-mention tokens (an "@" + a long run of digits),
	// which prefix a message that addressed the assistant.
	mentionRE  = regexp.MustCompile(`@\d{5,}`)
	multiSpace = regexp.MustCompile(`[ \t]{2,}`)

	projectWord = `(?:projects?|projek|proyek)`

	// assignVerbRE matches an explicit binding change: an assign verb, then the
	// project keyword (only small connector words may sit between), then a target
	// to end of line. Anchored at the start so a mid-sentence "…set a reminder for
	// project X…" does not match — the verb must lead the message.
	assignVerbRE = regexp.MustCompile(`(?i)^\s*(?:please\s+|pls\s+|tolong\s+|coba\s+|kamu\s+|lo\s+|lu\s+)?` +
		`(?:re-?assign|assign|switch|change|move|set|use|act\s+as|become|connect|reconnect|link|bind|join|` +
		`pindah(?:kan|in)?|ganti|jadi(?:kan|lah|in)?|pakai|gunakan|atur|ubah|masuk(?:kan|in)?)\s+` +
		`(?:this\s+group\s+)?(?:to\s+|as\s+|into\s+|ke\s+|the\s+|jadi\s+)?` +
		projectWord + `\s+(?:to\s+|ke\s+|jadi\s+|=\s*)?(.+?)\s*$`)

	// bareProjectRE matches a message that is just "project <something>".
	bareProjectRE = regexp.MustCompile(`(?i)^\s*` + projectWord + `\s+(.+?)\s*$`)

	// listRE recognises a request to list projects.
	listRE = regexp.MustCompile(`(?i)\b(list|daftar|available|semua|all)\b`)

	// unassignRE recognises a detach request.
	unassignRE = regexp.MustCompile(`(?i)\b(unassign|un-assign|detach|disconnect|reset|clear|remove|unbind|lepas(?:kan)?|copot|keluar(?:kan)?|hapus)\b`)

	// interrogativeRE recognises a question word, marking a status query.
	interrogativeRE = regexp.MustCompile(`(?i)\b(what|which|apa|apaan|mana)\b`)

	// projectAnywhereRE finds the project keyword anywhere in the text.
	projectAnywhereRE = regexp.MustCompile(`(?i)` + projectWord)
)

// classify parses a group message into a binding command. It is intentionally
// conservative: an ordinary request that merely mentions the word "project" (in
// a bound group) is left as cmdNone/cmdStatus so it reaches the agent.
func classify(raw string) command {
	text := stripMentions(raw)
	if text == "" {
		return command{kind: cmdNone}
	}
	lower := strings.ToLower(text)
	hasProject := projectAnywhereRE.MatchString(lower)

	// Detach ("unassign", "lepas project", …) — needs the project keyword so a
	// bare "reset"/"clear" in normal chat doesn't trigger it.
	if hasProject && unassignRE.MatchString(lower) {
		return command{kind: cmdUnassign}
	}

	// List ("list projects", "project apa aja", "daftar project", …).
	if hasProject && (listRE.MatchString(lower) ||
		strings.Contains(lower, "apa aja") || strings.Contains(lower, "apa saja")) {
		return command{kind: cmdList}
	}

	// Explicit assign ("assign to project X", "pindah ke project Beta", …).
	if m := assignVerbRE.FindStringSubmatch(text); m != nil {
		t := cleanTarget(m[1])
		if t != "" && !isInterrogativeTarget(t) {
			return command{kind: cmdAssign, target: t}
		}
	}

	// Bare "project <rest>".
	if m := bareProjectRE.FindStringSubmatch(text); m != nil {
		t := cleanTarget(m[1])
		if t == "" || isInterrogativeTarget(t) {
			return command{kind: cmdStatus}
		}
		return command{kind: cmdAssignBare, target: t}
	}

	// A project-related question that isn't one of the above.
	if hasProject && (strings.Contains(text, "?") || interrogativeRE.MatchString(lower)) {
		return command{kind: cmdStatus}
	}
	if hasProject {
		return command{kind: cmdStatus}
	}
	return command{kind: cmdNone}
}

// stripMentions removes @-mention tokens and collapses the whitespace they leave.
func stripMentions(s string) string {
	s = mentionRE.ReplaceAllString(s, " ")
	s = multiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// cleanTarget tidies a captured project name: trims surrounding quotes and
// punctuation and drops a few leading filler words.
func cleanTarget(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'“”.?!`)
	s = strings.TrimSpace(s)
	for _, filler := range []string{"the ", "ke ", "to ", "jadi ", "yang ", "yg "} {
		if strings.HasPrefix(strings.ToLower(s), filler) {
			s = strings.TrimSpace(s[len(filler):])
		}
	}
	return strings.Trim(s, `"'“”.?!`)
}

// isInterrogativeTarget reports whether a captured target is really a question
// ("apa", "mana", "what", …) rather than a project name — i.e. the message was a
// status query like "project apa?".
func isInterrogativeTarget(t string) bool {
	l := strings.ToLower(strings.TrimSpace(t))
	switch l {
	case "apa", "apaan", "mana", "what", "which", "itu", "ini", "sekarang", "now", "current", "saat ini", "ini apa", "itu apa":
		return true
	}
	// A short target that is only a question word plus punctuation.
	return interrogativeRE.MatchString(l) && len(strings.Fields(l)) <= 2 && strings.Contains(l, "?")
}
