package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageMarshalContent(t *testing.T) {
	// Plain text message → content is a string.
	b, _ := json.Marshal(Message{Role: "user", Content: "hi"})
	if !strings.Contains(string(b), `"content":"hi"`) {
		t.Errorf("plain content wrong: %s", b)
	}

	// Multimodal message → content is an array with a text + image_url part.
	b, _ = json.Marshal(Message{Role: "user", ContentParts: []ContentPart{
		{Type: "text", Text: "what is this"},
		{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,AAAA"}},
	}})
	s := string(b)
	if !strings.Contains(s, `"content":[`) || !strings.Contains(s, `"image_url"`) || !strings.Contains(s, "data:image/png") {
		t.Errorf("multimodal content wrong: %s", s)
	}

	// Assistant tool-call message with no content → content omitted.
	b, _ = json.Marshal(Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "1"}}})
	if strings.Contains(string(b), `"content"`) {
		t.Errorf("empty content should be omitted: %s", b)
	}
}
