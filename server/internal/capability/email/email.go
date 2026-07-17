package email

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/integration/google"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
)

// Handler handles email-related commands.
type Handler struct {
	client *google.GmailClient
	// lastInbox caches the most recent inbox listing for "read email N" commands
	lastInbox []google.EmailSummary
}

// New creates a new email handler.
func New(client *google.GmailClient) *Handler {
	return &Handler{client: client}
}

func (h *Handler) Name() string { return "email" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityEmail
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionEmailInbox:
		return h.inbox(ctx)
	case intent.ActionEmailRead:
		return h.read(ctx, result)
	case intent.ActionEmailSearch:
		return h.search(ctx, result)
	case intent.ActionEmailDraft:
		return h.draft(ctx, result)
	default:
		return "I can check your inbox, read emails, search, or draft replies. Try: _check my email_", nil
	}
}

func (h *Handler) inbox(ctx context.Context) (string, error) {
	emails, err := h.client.ListInbox(ctx, 10)
	if err != nil {
		return "", fmt.Errorf("list inbox: %w", err)
	}

	if len(emails) == 0 {
		return "Your inbox is clear — no unread emails!", nil
	}

	h.lastInbox = emails

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Inbox* — %d unread:\n\n", len(emails)))

	for i, email := range emails {
		sb.WriteString(fmt.Sprintf("%d. *%s*\n", i+1, email.Subject))
		sb.WriteString(fmt.Sprintf("   From: %s\n", email.From))
		if email.Snippet != "" {
			snippet := email.Snippet
			if len(snippet) > 100 {
				snippet = snippet[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("   _%s_\n", snippet))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Reply with _read email N_ to read the full email.")

	return sb.String(), nil
}

func (h *Handler) read(ctx context.Context, result *intent.ParseResult) (string, error) {
	indexStr := result.Entities["index"]
	if indexStr == "" {
		return "Which email would you like to read? Example: _read email 1_", nil
	}

	idx, err := strconv.Atoi(indexStr)
	if err != nil || idx < 1 {
		return "Please provide a valid email number. Example: _read email 1_", nil
	}

	if idx > len(h.lastInbox) {
		return fmt.Sprintf("I only have %d emails in the last listing. Try _check my email_ first.", len(h.lastInbox)), nil
	}

	emailID := h.lastInbox[idx-1].ID
	detail, err := h.client.ReadEmail(ctx, emailID)
	if err != nil {
		return "", fmt.Errorf("read email: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*%s*\n\n", detail.Subject))
	sb.WriteString(fmt.Sprintf("From: %s\n", detail.From))
	sb.WriteString(fmt.Sprintf("To: %s\n", detail.To))
	sb.WriteString(fmt.Sprintf("Date: %s\n\n", detail.Date))
	sb.WriteString(detail.Body)

	if len(detail.Attachments) > 0 {
		sb.WriteString(fmt.Sprintf("\n\n📎 Attachments: %s", strings.Join(detail.Attachments, ", ")))
	}

	return sb.String(), nil
}

func (h *Handler) search(ctx context.Context, result *intent.ParseResult) (string, error) {
	query := result.Entities["query"]
	if query == "" {
		return "What should I search for? Example: _search email about project update_", nil
	}

	emails, err := h.client.SearchEmail(ctx, query, 10)
	if err != nil {
		return "", fmt.Errorf("search email: %w", err)
	}

	if len(emails) == 0 {
		return fmt.Sprintf("No emails found matching %q.", query), nil
	}

	h.lastInbox = emails

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Search results* for %q — %d found:\n\n", query, len(emails)))

	for i, email := range emails {
		unread := ""
		if email.Unread {
			unread = " 🔵"
		}
		sb.WriteString(fmt.Sprintf("%d. *%s*%s\n", i+1, email.Subject, unread))
		sb.WriteString(fmt.Sprintf("   From: %s\n\n", email.From))
	}

	sb.WriteString("Reply with _read email N_ to read the full email.")

	return sb.String(), nil
}

func (h *Handler) draft(ctx context.Context, result *intent.ParseResult) (string, error) {
	to := result.Entities["to"]
	body := result.Entities["body"]

	if to == "" && body == "" {
		return "Please specify who to reply to and the message. Example: _draft reply to john@example.com: sounds good, see you there_", nil
	}

	if body == "" {
		return fmt.Sprintf("What should I write to %s?", to), nil
	}

	subject := "Re: (via assistant)"
	draftID, err := h.client.CreateDraft(ctx, to, subject, body, "")
	if err != nil {
		return "", fmt.Errorf("create draft: %w", err)
	}

	// Read-after-write: confirm the draft actually exists in the mailbox before
	// telling the user it was created.
	if _, err := h.client.DraftExists(ctx, draftID); err != nil {
		return "", fmt.Errorf("verify draft saved: %w", err)
	}

	return fmt.Sprintf("Draft created (ID: %s).\nTo: %s\n\n_%s_\n\nNote: This is saved as a *draft* — it has NOT been sent.", draftID, to, body), nil
}
