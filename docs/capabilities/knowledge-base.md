# Knowledge Base (Notes)

## Overview

A personal note-taking system with full-text search. Save quick notes, tag them for organization, and search later.

**Storage:** SQLite `notes` + `notes_fts` tables (see [Data Model](../architecture/data-model.md))

## Commands

### Save Note

| Example Messages | Intent |
|-----------------|--------|
| "Save note: API key for service X is abc123" | Save with auto-title |
| "Note: Meeting with Bob - discussed Q3 roadmap" | Save note |
| "Remember that the wifi password is hunter2" | Save note |
| "Save note tagged work: Sprint goals for July" | Save with tag |

**Response:**
```
Note saved:
  Title: API key for service X
  Tags: none
  ID: #14

Tip: add tags with "tag note 14 as credentials"
```

### Search Notes

| Example Messages | Intent |
|-----------------|--------|
| "Find my note about API key" | Full-text search |
| "Search notes for wifi password" | Full-text search |
| "What did I save about Q3 roadmap?" | Full-text search |

**Response:**
```
Found 2 notes matching "API key":

1. #14 — API key for service X (Jul 7)
   "API key for service X is abc123"

2. #8 — Production credentials (Jun 28)
   "...the API key for prod is xyz789..."

Reply with a note number for the full content.
```

### List Notes

| Example Messages | Intent |
|-----------------|--------|
| "Show my recent notes" | List latest notes |
| "List notes tagged work" | List by tag |
| "How many notes do I have?" | Count |

### Update Note

| Example Messages | Intent |
|-----------------|--------|
| "Update note 14: new key is def456" | Replace content |
| "Tag note 14 as credentials" | Add tag |
| "Rename note 14 to Production API keys" | Update title |

### Delete Note

| Example Messages | Intent |
|-----------------|--------|
| "Delete note 14" | Delete by ID |
| "Remove the wifi password note" | Delete by search |

**Confirmation required:**
```
Delete note #14 "API key for service X"? (yes/no)
```

## Full-Text Search

SQLite FTS5 provides fast full-text search:

```sql
-- Search notes
SELECT n.id, n.title, n.content, n.tags, n.created_at
FROM notes_fts fts
JOIN notes n ON n.id = fts.rowid
WHERE notes_fts MATCH ?
ORDER BY rank
LIMIT 10;
```

FTS5 supports:
- Simple keyword matching: `"API key"`
- Phrase search: `'"wifi password"'`
- Boolean operators: `"API AND production"`
- Prefix matching: `"cred*"`

## Tagging

Tags are stored as comma-separated values in the `tags` column.

```go
func (n *Note) TagList() []string {
    if n.Tags == "" {
        return nil
    }
    return strings.Split(n.Tags, ",")
}

func (n *Note) AddTag(tag string) {
    tags := n.TagList()
    tags = append(tags, strings.TrimSpace(tag))
    n.Tags = strings.Join(tags, ",")
}
```

Common tags: `work`, `personal`, `credentials`, `ideas`, `reference`

## Edge Cases

| Case | Behavior |
|------|----------|
| Empty note | "What would you like to save?" |
| Duplicate content | Allow (user may intend it) |
| Very long note | Accept up to 10,000 characters, warn if longer |
| No search results | "No notes found matching 'X'. Try different keywords." |
| Delete non-existent note | "Note #99 not found." |

## Regex Patterns (MVP)

```go
var knowledgePatterns = []regexp.Regexp{
    regexp.MustCompile(`(?i)(save|add|create)\s*(a\s*)?(note|memo)`),
    regexp.MustCompile(`(?i)(remember|store)\s+(that|this)`),
    regexp.MustCompile(`(?i)(find|search|show|list)\s*(my\s*)?(note|memo)`),
    regexp.MustCompile(`(?i)(delete|remove)\s*(note|memo)\s*#?\d+`),
    regexp.MustCompile(`(?i)(update|edit|change)\s*(note|memo)\s*#?\d+`),
    regexp.MustCompile(`(?i)tag\s*(note|memo)\s*#?\d+`),
}
```
