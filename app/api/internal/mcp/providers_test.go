package mcp

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestNotionStrictClassification(t *testing.T) {
	n, ok := Lookup("notion")
	if !ok {
		t.Fatal("notion provider not found")
	}

	// Read tool: exposed in both modes.
	if !n.Exposes("notion-search", nil, ModeRead) {
		t.Error("notion-search should be exposed in read mode")
	}
	if !n.Exposes("notion-search", nil, ModeReadWrite) {
		t.Error("notion-search should be exposed in read&write mode")
	}

	// Write tool: hidden in read mode, exposed in read&write.
	if n.Exposes("notion-create-pages", nil, ModeRead) {
		t.Error("notion-create-pages must NOT be exposed in read-only mode")
	}
	if !n.Exposes("notion-create-pages", nil, ModeReadWrite) {
		t.Error("notion-create-pages should be exposed in read&write mode")
	}

	// Unknown tool: never exposed (strict allow-list), even with a read-only hint,
	// because Notion does not use annotations.
	if n.Exposes("notion-brand-new-tool", boolPtr(true), ModeReadWrite) {
		t.Error("unlisted Notion tool must never be exposed")
	}
}

func TestAnnotationAssistedClassification(t *testing.T) {
	cf, ok := Lookup("cloudflare")
	if !ok {
		t.Fatal("cloudflare provider not found")
	}

	// Seeded read tool.
	if !cf.Exposes("workers_list", nil, ModeRead) {
		t.Error("seeded read tool should be exposed in read mode")
	}
	// Unlisted tool the server marks read-only: exposed via annotation.
	if !cf.Exposes("some_new_get", boolPtr(true), ModeRead) {
		t.Error("annotation readOnlyHint=true should expose in read mode")
	}
	// Unlisted tool the server marks NOT read-only: a write, hidden in read mode.
	if cf.Exposes("some_new_write", boolPtr(false), ModeRead) {
		t.Error("annotation readOnlyHint=false must be hidden in read-only mode")
	}
	if !cf.Exposes("some_new_write", boolPtr(false), ModeReadWrite) {
		t.Error("annotation readOnlyHint=false should be exposed in read&write mode")
	}
	// Unlisted, unannotated tool: hidden everywhere (never expose the unclassified).
	if cf.Exposes("mystery", nil, ModeReadWrite) {
		t.Error("unlisted+unannotated tool must never be exposed")
	}
}

func TestModeNormalize(t *testing.T) {
	if Mode("").Normalize() != ModeRead {
		t.Error("empty mode should normalize to read")
	}
	if Mode("garbage").Normalize() != ModeRead {
		t.Error("unknown mode should normalize to read")
	}
	if Mode("readwrite").Normalize() != ModeReadWrite {
		t.Error("readwrite should stay readwrite")
	}
}
