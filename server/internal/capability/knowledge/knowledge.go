package knowledge

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Handler handles knowledge base / notes commands.
type Handler struct {
	store         store.Store
	maxNoteLength int
}

// New creates a new knowledge handler.
func New(s store.Store, maxNoteLength int) *Handler {
	return &Handler{
		store:         s,
		maxNoteLength: maxNoteLength,
	}
}

func (h *Handler) Name() string { return "knowledge" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityKnowledge
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionNoteSave:
		return h.save(ctx, result)
	case intent.ActionNoteSearch:
		return h.search(ctx, result)
	case intent.ActionNoteList:
		return h.list(ctx, result)
	case intent.ActionNoteDelete:
		return h.delete(ctx, result)
	default:
		return "I can save, search, list, or delete notes. Try: _save note Meeting Notes: discussed roadmap_", nil
	}
}

func (h *Handler) save(ctx context.Context, result *intent.ParseResult) (string, error) {
	title := result.Entities["title"]
	if title == "" {
		return "Please specify a title. Example: _save note Meeting Notes: discussed the roadmap_", nil
	}

	content := result.Entities["content"]

	// Check for tags in the title (e.g., "Meeting Notes #work #important")
	tags := ""
	if idx := strings.Index(title, " #"); idx != -1 {
		tagPart := title[idx:]
		title = strings.TrimSpace(title[:idx])
		var tagList []string
		for _, t := range strings.Fields(tagPart) {
			if strings.HasPrefix(t, "#") {
				tagList = append(tagList, strings.TrimPrefix(t, "#"))
			}
		}
		tags = strings.Join(tagList, ",")
	}

	if len(content) > h.maxNoteLength {
		return fmt.Sprintf("Note content is too long (max %d characters).", h.maxNoteLength), nil
	}

	userID := authctx.UserID(ctx)
	note, err := h.store.CreateNote(ctx, userID, title, content, tags)
	if err != nil {
		return "", fmt.Errorf("create note: %w", err)
	}

	// Read-after-write: confirm the note actually persisted before telling the
	// user it was saved, and build the confirmation from the re-read record.
	note, err = h.store.GetNote(ctx, userID, note.ID)
	if err != nil {
		return "", fmt.Errorf("verify note saved: %w", err)
	}
	if note == nil {
		return "", fmt.Errorf("verify note saved: note not found after create")
	}

	response := fmt.Sprintf("Note saved (#%d): *%s*", note.ID, note.Title)
	if tags != "" {
		response += fmt.Sprintf("\nTags: %s", tags)
	}
	if content != "" {
		preview := content
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		response += fmt.Sprintf("\n_%s_", preview)
	}

	return response, nil
}

func (h *Handler) search(ctx context.Context, result *intent.ParseResult) (string, error) {
	query := result.Entities["query"]
	if query == "" {
		return "What should I search for? Example: _search notes about roadmap_", nil
	}

	notes, err := h.store.SearchNotes(ctx, authctx.UserID(ctx), query)
	if err != nil {
		return "", fmt.Errorf("search notes: %w", err)
	}

	if len(notes) == 0 {
		return fmt.Sprintf("No notes found matching %q.", query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Search results* for %q — %d found:\n\n", query, len(notes)))

	for _, n := range notes {
		sb.WriteString(fmt.Sprintf("#%d — *%s*\n", n.ID, n.Title))
		if n.Tags != "" {
			sb.WriteString(fmt.Sprintf("   Tags: %s\n", n.Tags))
		}
		preview := n.Content
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		if preview != "" {
			sb.WriteString(fmt.Sprintf("   _%s_\n", preview))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func (h *Handler) list(ctx context.Context, result *intent.ParseResult) (string, error) {
	tag := result.Entities["tag"]

	notes, err := h.store.ListNotes(ctx, authctx.UserID(ctx), tag)
	if err != nil {
		return "", fmt.Errorf("list notes: %w", err)
	}

	if len(notes) == 0 {
		if tag != "" {
			return fmt.Sprintf("No notes tagged with %q.", tag), nil
		}
		return "No notes saved yet. Try: _save note My First Note: hello world_", nil
	}

	var sb strings.Builder
	if tag != "" {
		sb.WriteString(fmt.Sprintf("*Notes* tagged %q (%d):\n\n", tag, len(notes)))
	} else {
		sb.WriteString(fmt.Sprintf("*All Notes* (%d):\n\n", len(notes)))
	}

	for _, n := range notes {
		sb.WriteString(fmt.Sprintf("#%d — *%s*", n.ID, n.Title))
		if n.Tags != "" {
			sb.WriteString(fmt.Sprintf(" [%s]", n.Tags))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func (h *Handler) delete(ctx context.Context, result *intent.ParseResult) (string, error) {
	idStr := result.Entities["id"]
	if idStr == "" {
		return "Which note should I delete? Example: _delete note 1_", nil
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return "Please provide a valid note number.", nil
	}

	// Verify note exists
	userID := authctx.UserID(ctx)
	note, err := h.store.GetNote(ctx, userID, id)
	if err != nil {
		return fmt.Sprintf("Note #%d not found.", id), nil
	}

	if err := h.store.DeleteNote(ctx, userID, id); err != nil {
		return "", fmt.Errorf("delete note: %w", err)
	}

	return fmt.Sprintf("Note #%d (*%s*) deleted.", id, note.Title), nil
}
