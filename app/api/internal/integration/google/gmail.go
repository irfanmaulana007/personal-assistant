package google

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// GmailClient wraps the Gmail API.
type GmailClient struct {
	auth *Auth
	log  *slog.Logger
}

// NewGmailClient creates a new Gmail API client.
func NewGmailClient(auth *Auth, log *slog.Logger) *GmailClient {
	return &GmailClient{
		auth: auth,
		log:  log.With("component", "gmail"),
	}
}

// EmailSummary holds a brief view of an email.
type EmailSummary struct {
	ID      string
	From    string
	Subject string
	Snippet string
	Date    string
	Unread  bool
}

// EmailDetail holds the full content of an email.
type EmailDetail struct {
	ID          string
	From        string
	To          string
	Subject     string
	Body        string
	Date        string
	Attachments []string
}

func (g *GmailClient) service(ctx context.Context) (*gmail.Service, error) {
	ts, err := g.auth.TokenSource(ctx)
	if err != nil {
		return nil, err
	}
	return gmail.NewService(ctx, option.WithTokenSource(ts))
}

// ListInbox returns recent unread emails.
func (g *GmailClient) ListInbox(ctx context.Context, maxResults int) ([]EmailSummary, error) {
	srv, err := g.service(ctx)
	if err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	}

	resp, err := srv.Users.Messages.List("me").
		LabelIds("INBOX").
		Q("is:unread").
		MaxResults(int64(maxResults)).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	var summaries []EmailSummary
	for _, msg := range resp.Messages {
		detail, err := srv.Users.Messages.Get("me", msg.Id).
			Format("metadata").
			MetadataHeaders("From", "Subject", "Date").
			Context(ctx).
			Do()
		if err != nil {
			g.log.Warn("failed to get message", "id", msg.Id, "error", err)
			continue
		}

		summary := EmailSummary{
			ID:      msg.Id,
			Snippet: detail.Snippet,
			Unread:  true,
		}
		for _, h := range detail.Payload.Headers {
			switch h.Name {
			case "From":
				summary.From = h.Value
			case "Subject":
				summary.Subject = h.Value
			case "Date":
				summary.Date = h.Value
			}
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// ReadEmail returns the full content of an email by ID.
func (g *GmailClient) ReadEmail(ctx context.Context, messageID string) (*EmailDetail, error) {
	srv, err := g.service(ctx)
	if err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	}

	msg, err := srv.Users.Messages.Get("me", messageID).
		Format("full").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}

	detail := &EmailDetail{
		ID: messageID,
	}

	for _, h := range msg.Payload.Headers {
		switch h.Name {
		case "From":
			detail.From = h.Value
		case "To":
			detail.To = h.Value
		case "Subject":
			detail.Subject = h.Value
		case "Date":
			detail.Date = h.Value
		}
	}

	detail.Body = extractBody(msg.Payload)

	// Collect attachment names
	for _, part := range msg.Payload.Parts {
		if part.Filename != "" {
			detail.Attachments = append(detail.Attachments, part.Filename)
		}
	}

	// Truncate body to 2000 chars
	if len(detail.Body) > 2000 {
		detail.Body = detail.Body[:2000] + "\n... (truncated)"
	}

	return detail, nil
}

// SearchEmail searches emails by query.
func (g *GmailClient) SearchEmail(ctx context.Context, query string, maxResults int) ([]EmailSummary, error) {
	srv, err := g.service(ctx)
	if err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	}

	resp, err := srv.Users.Messages.List("me").
		Q(query).
		MaxResults(int64(maxResults)).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}

	var summaries []EmailSummary
	for _, msg := range resp.Messages {
		detail, err := srv.Users.Messages.Get("me", msg.Id).
			Format("metadata").
			MetadataHeaders("From", "Subject", "Date").
			Context(ctx).
			Do()
		if err != nil {
			continue
		}

		summary := EmailSummary{
			ID:      msg.Id,
			Snippet: detail.Snippet,
		}
		for _, h := range detail.Payload.Headers {
			switch h.Name {
			case "From":
				summary.From = h.Value
			case "Subject":
				summary.Subject = h.Value
			case "Date":
				summary.Date = h.Value
			}
		}

		// Check if unread
		for _, label := range detail.LabelIds {
			if label == "UNREAD" {
				summary.Unread = true
				break
			}
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// CreateDraft creates a draft reply to an email. Never sends.
func (g *GmailClient) CreateDraft(ctx context.Context, to, subject, body, threadID string) (string, error) {
	srv, err := g.service(ctx)
	if err != nil {
		return "", fmt.Errorf("create gmail service: %w", err)
	}

	// Build the raw email
	raw := fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		to, subject, body)

	encoded := base64.URLEncoding.EncodeToString([]byte(raw))

	draft := &gmail.Draft{
		Message: &gmail.Message{
			Raw:      encoded,
			ThreadId: threadID,
		},
	}

	created, err := srv.Users.Drafts.Create("me", draft).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("create draft: %w", err)
	}

	return created.Id, nil
}

// DraftExists reports whether a draft with the given id is present in the user's
// mailbox. Used to confirm a just-created draft actually persisted
// (read-after-write verification).
func (g *GmailClient) DraftExists(ctx context.Context, draftID string) (bool, error) {
	srv, err := g.service(ctx)
	if err != nil {
		return false, fmt.Errorf("create gmail service: %w", err)
	}
	// "minimal" keeps the round-trip cheap: we only need to know it exists.
	if _, err := srv.Users.Drafts.Get("me", draftID).Format("minimal").Context(ctx).Do(); err != nil {
		return false, fmt.Errorf("get draft: %w", err)
	}
	return true, nil
}

// extractBody extracts plain text from a MIME message payload.
func extractBody(payload *gmail.MessagePart) string {
	if payload.MimeType == "text/plain" && payload.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			return string(data)
		}
	}

	for _, part := range payload.Parts {
		if body := extractBody(part); body != "" {
			return body
		}
	}

	// Fallback: try to decode whatever body data exists
	if payload.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			return stripHTML(string(data))
		}
	}

	return ""
}

// stripHTML does a basic removal of HTML tags.
func stripHTML(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}
