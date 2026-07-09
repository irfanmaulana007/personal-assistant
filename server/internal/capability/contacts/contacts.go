// Package contacts implements the Ask About Contact skill: saving and looking
// up the user's personal contacts.
package contacts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Handler saves and searches the user's contacts.
type Handler struct {
	store store.Store
	log   *slog.Logger
}

// New creates a contacts handler.
func New(s store.Store, log *slog.Logger) *Handler {
	return &Handler{store: s, log: log.With("component", "contacts")}
}

func (h *Handler) Name() string { return "contact" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityContact
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionContactAdd:
		return h.add(ctx, result)
	case intent.ActionContactSearch:
		return h.search(ctx, result)
	default:
		return "I can save a contact or look one up.", nil
	}
}

func (h *Handler) add(ctx context.Context, result *intent.ParseResult) (string, error) {
	name := strings.TrimSpace(result.Entities["name"])
	if name == "" {
		return "What's the contact's name?", nil
	}
	phone := strings.TrimSpace(result.Entities["phone"])
	email := strings.TrimSpace(result.Entities["email"])
	note := strings.TrimSpace(result.Entities["note"])

	c, err := h.store.CreateContact(ctx, authctx.UserID(ctx), name, phone, email, note)
	if err != nil {
		return "", fmt.Errorf("create contact: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Saved contact: *%s*", c.Name))
	if c.Phone != "" {
		sb.WriteString("\nPhone: " + c.Phone)
	}
	if c.Email != "" {
		sb.WriteString("\nEmail: " + c.Email)
	}
	if c.Note != "" {
		sb.WriteString("\nNote: " + c.Note)
	}
	return sb.String(), nil
}

func (h *Handler) search(ctx context.Context, result *intent.ParseResult) (string, error) {
	query := strings.TrimSpace(result.Entities["query"])

	contacts, err := h.store.SearchContacts(ctx, authctx.UserID(ctx), query)
	if err != nil {
		return "", fmt.Errorf("search contacts: %w", err)
	}
	if len(contacts) == 0 {
		if query == "" {
			return "You don't have any saved contacts yet.", nil
		}
		return fmt.Sprintf("I couldn't find a contact matching %q.", query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d contact(s):\n", len(contacts)))
	for _, c := range contacts {
		sb.WriteString("\n*" + c.Name + "*")
		if c.Phone != "" {
			sb.WriteString(" — " + c.Phone)
		}
		if c.Email != "" {
			sb.WriteString(" — " + c.Email)
		}
		if c.Note != "" {
			sb.WriteString("\n  " + c.Note)
		}
	}
	return sb.String(), nil
}
