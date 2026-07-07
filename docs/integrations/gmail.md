# Gmail Integration

## Overview

Integration with Gmail API to read, search, summarize, and draft emails. **Critical safety rule: the assistant never auto-sends emails.** All outgoing emails are saved as drafts for the user to review and send manually.

## Prerequisites

1. Enable the Gmail API in Google Cloud Console (same project as Calendar)
2. Add Gmail scopes to the OAuth2 consent screen
3. Re-authorize if adding Gmail after initial Calendar setup

## Required Scopes

```go
var gmailScopes = []string{
    "https://www.googleapis.com/auth/gmail.readonly",
    "https://www.googleapis.com/auth/gmail.compose",  // for drafts
    "https://www.googleapis.com/auth/gmail.labels",
}
```

## API Operations

### List Recent Emails (Inbox Summary)

```go
func listInbox(srv *gmail.Service, maxResults int64) ([]*gmail.Message, error) {
    resp, err := srv.Users.Messages.List("me").
        LabelIds("INBOX").
        MaxResults(maxResults).
        Q("is:unread").
        Do()

    var messages []*gmail.Message
    for _, m := range resp.Messages {
        msg, err := srv.Users.Messages.Get("me", m.Id).
            Format("metadata").
            MetadataHeaders("From", "Subject", "Date").
            Do()
        messages = append(messages, msg)
    }
    return messages, err
}
```

**Response format example:**
```
You have 3 unread emails:

1. From: alice@example.com
   Subject: Project update - Q3 planning
   Date: Jul 7, 2026 10:30 AM

2. From: bob@company.com
   Subject: Meeting notes from today
   Date: Jul 7, 2026 09:15 AM

3. From: notifications@github.com
   Subject: [PR #42] Review requested
   Date: Jul 7, 2026 08:00 AM

Reply with the number to read the full email.
```

### Read Full Email

```go
func readEmail(srv *gmail.Service, messageID string) (*gmail.Message, error) {
    return srv.Users.Messages.Get("me", messageID).
        Format("full").
        Do()
}
```

- Extract plain text body from multipart MIME
- Truncate very long emails (> 2000 chars) with "... [truncated]"
- Show key metadata: From, To, Subject, Date

### Search Emails

```go
func searchEmails(srv *gmail.Service, query string, maxResults int64) ([]*gmail.Message, error) {
    resp, err := srv.Users.Messages.List("me").
        Q(query).
        MaxResults(maxResults).
        Do()
    // fetch metadata for each result...
}
```

Supports Gmail search syntax: `from:alice subject:project after:2026/07/01`

### Create Draft (Never Auto-Send)

```go
func createDraft(srv *gmail.Service, to, subject, body string) (*gmail.Draft, error) {
    msg := &gmail.Message{
        Raw: base64.URLEncoding.EncodeToString(
            []byte(fmt.Sprintf(
                "To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
                to, subject, body,
            )),
        ),
    }
    return srv.Users.Drafts.Create("me", &gmail.Draft{Message: msg}).Do()
}
```

**Response format:**
```
Draft created:
  To: alice@example.com
  Subject: Re: Project update - Q3 planning
  Body preview: "Thanks for the update. I'll review the timeline and..."

Open Gmail to review and send.
```

## Safety Rules

1. **Never call `Messages.Send()`** — only `Drafts.Create()`
2. Always confirm with the user before creating a draft
3. Show draft preview before saving
4. Log all draft creation actions

## Email Body Parsing

```go
func extractPlainText(msg *gmail.Message) string {
    // Walk MIME parts, prefer text/plain
    for _, part := range msg.Payload.Parts {
        if part.MimeType == "text/plain" {
            data, _ := base64.URLEncoding.DecodeString(part.Body.Data)
            return string(data)
        }
    }
    // Fallback: strip HTML from text/html part
    // ...
}
```

## Rate Limiting

Gmail API quotas:
- 250 quota units per user per second
- `messages.list` = 5 units, `messages.get` = 5 units
- Add small delays between batch reads
