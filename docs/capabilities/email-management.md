# Email Management

## Overview

Read, search, and summarize emails from Gmail. Draft replies for user review. **Never auto-send emails.**

**Integration:** [Gmail API](../integrations/gmail.md)

## Commands

### Inbox Summary

| Example Messages | Intent |
|-----------------|--------|
| "Check my email" | List unread inbox |
| "Any new emails?" | List unread inbox |
| "Show my inbox" | List recent inbox |
| "How many unread emails?" | Count unread |

**Response format:**
```
You have 5 unread emails:

1. Alice Smith - Project update (10:30 AM)
2. Bob Jones - Meeting notes (9:15 AM)
3. GitHub - [PR #42] Review requested (8:00 AM)
4. AWS - Invoice for June 2026 (Yesterday)
5. Newsletter - Weekly digest (Yesterday)

Reply with a number to read the full email.
```

### Read Email

| Example Messages | Intent |
|-----------------|--------|
| "Read email 2" | Read by number (from list) |
| "Show me the email from Alice" | Read by sender |
| "Open the GitHub notification" | Read by subject keyword |

**Response format:**
```
From: Alice Smith <alice@example.com>
Date: Jul 7, 2026 10:30 AM
Subject: Project update

Hi,

Just wanted to share the Q3 planning doc. Key points:
- Timeline shifted to August launch
- Need your input on the API design
- Budget approved for 2 additional contractors

Let me know if you have questions.

Best,
Alice
```

For long emails (> 2000 chars):
```
[... message truncated — showing first 2000 characters]
```

### Search Email

| Example Messages | Intent |
|-----------------|--------|
| "Find emails from alice about project" | Search by sender + keyword |
| "Search emails about invoice" | Search by keyword |
| "Find emails from last week" | Search by date |

Uses Gmail search syntax internally.

### Draft Reply

| Example Messages | Intent |
|-----------------|--------|
| "Reply to Alice saying I'll review it tomorrow" | Draft reply |
| "Draft a response to email 2" | Draft reply to listed email |

**Flow:**
1. Identify which email to reply to
2. Generate draft content from user's instructions
3. Show preview and confirm
4. Save as draft (NEVER send)

**Confirmation:**
```
Draft created:
  To: alice@example.com
  Subject: Re: Project update

  "Thanks Alice! I'll review the planning doc tomorrow and share
   my thoughts on the API design by end of week."

Saved as draft. Open Gmail to review and send.
```

## Safety Rules

1. **NEVER call `Messages.Send()`** — only `Drafts.Create()`
2. Always show draft preview before saving
3. Confirm with user before creating draft
4. Log all draft creation actions for audit

## Entity Extraction

| Entity | Examples |
|--------|---------|
| Email number | "email 2", "number 3", "the first one" |
| Sender | "from Alice", "from bob@company.com" |
| Subject keyword | "about project", "regarding invoice" |
| Date range | "from last week", "since Monday" |
| Reply content | free-form text after "saying" / "that" |

## Edge Cases

| Case | Behavior |
|------|----------|
| No unread emails | "Your inbox is clear — no unread emails!" |
| Email has attachments | Note attachment names: "Attachments: report.pdf, data.xlsx" |
| HTML-only email | Strip HTML, show plain text |
| Very long thread | Show only the latest message, note thread length |
| Ambiguous sender match | List matches, ask user to pick |

## Regex Patterns (MVP)

```go
var emailPatterns = []regexp.Regexp{
    regexp.MustCompile(`(?i)(check|show|read|open).*(email|inbox|mail)`),
    regexp.MustCompile(`(?i)(any|how many).*(new|unread).*(email|mail)`),
    regexp.MustCompile(`(?i)(read|show|open)\s*(email|mail)?\s*#?\d+`),
    regexp.MustCompile(`(?i)(find|search).*(email|mail)`),
    regexp.MustCompile(`(?i)(reply|respond|draft).*(to|email|mail)`),
}
```
