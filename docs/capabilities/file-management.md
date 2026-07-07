# File Management (Phase 2)

## Overview

Handle files received via WhatsApp — download, store, index, and retrieve documents and media.

## Commands

| Example Messages | Intent |
|-----------------|--------|
| *sends an image* "Save this" | Download and store media |
| *sends a PDF* "Save this as project-spec" | Download with custom name |
| "Find the PDF I sent last week" | Search stored files |
| "List my saved files" | List files |
| "Delete file project-spec" | Delete stored file |

## File Storage

### Directory Structure
```
data/
  files/
    2026/
      07/
        image_20260707_143022.jpg
        project-spec.pdf
```

### Metadata Table

```sql
CREATE TABLE files (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    filename    TEXT NOT NULL,
    original_name TEXT,
    mime_type   TEXT NOT NULL,
    size_bytes  INTEGER NOT NULL,
    path        TEXT NOT NULL,
    tags        TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_files_created ON files(created_at);
```

## Media Download (whatsmeow)

```go
func downloadMedia(client *whatsmeow.Client, msg *events.Message) ([]byte, string, error) {
    if img := msg.Message.GetImageMessage(); img != nil {
        data, err := client.Download(img)
        return data, img.GetMimetype(), err
    }
    if doc := msg.Message.GetDocumentMessage(); doc != nil {
        data, err := client.Download(doc)
        return data, doc.GetMimetype(), err
    }
    return nil, "", fmt.Errorf("no downloadable media found")
}
```

## Supported Types

| Type | MIME Types | Phase |
|------|-----------|-------|
| Images | image/jpeg, image/png, image/webp | Phase 2 |
| Documents | application/pdf, text/plain | Phase 2 |
| Audio | audio/ogg, audio/mpeg | Phase 3 |
| Video | video/mp4 | Phase 3 |

## Storage Limits

- Max file size: 16MB (WhatsApp limit)
- Storage quota: configurable (default 1GB)
- Auto-cleanup: optional, delete files older than N days

## Edge Cases

| Case | Behavior |
|------|----------|
| Unsupported file type | "I can't save this file type yet. Supported: images, PDFs, text files." |
| Storage quota exceeded | "Storage full (1GB). Delete some files or increase quota in config." |
| Duplicate filename | Append timestamp suffix |
| File not found | "No file found matching 'X'." |
