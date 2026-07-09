// Package persona turns a user's persona preferences into a system-prompt
// fragment that shapes the assistant's style.
package persona

import (
	"context"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Service resolves a user's persona.
type Service struct {
	store store.Store
}

// New creates a persona service.
func New(s store.Store) *Service {
	return &Service{store: s}
}

// Prompt returns a system-prompt fragment for the user's persona, or "" when the
// persona is all defaults.
func (s *Service) Prompt(ctx context.Context, userID int64) string {
	p, err := s.store.GetUserPersona(ctx, userID)
	if err != nil {
		return ""
	}

	var parts []string
	if n := strings.TrimSpace(p.Name); n != "" {
		parts = append(parts, "Your name is "+n+"; refer to yourself as "+n+" when it's natural.")
	}
	switch p.Tone {
	case "formal":
		parts = append(parts, "Use a formal, professional tone (in Indonesian, address the user with \"Anda\").")
	case "casual":
		parts = append(parts, "Use a casual, warm, conversational tone (in Indonesian, \"kamu\" is fine).")
	}
	switch p.Emoji {
	case "none":
		parts = append(parts, "Do not use emoji.")
	case "frequent":
		parts = append(parts, "Use emoji generously to add warmth and personality.")
	}
	switch p.Length {
	case "concise":
		parts = append(parts, "Keep responses short and to the point; avoid unnecessary detail.")
	case "detailed":
		parts = append(parts, "Give thorough, well-explained responses with helpful detail.")
	}
	switch p.Personality {
	case "professional":
		parts = append(parts, "Be professional and matter-of-fact.")
	case "friendly":
		parts = append(parts, "Be friendly and personable.")
	case "witty":
		parts = append(parts, "Add a bit of light humor and wit where it fits.")
	case "direct":
		parts = append(parts, "Be direct and concise; skip pleasantries and filler.")
	case "encouraging":
		parts = append(parts, "Be warm, supportive, and encouraging.")
	}
	if c := strings.TrimSpace(p.Custom); c != "" {
		parts = append(parts, c)
	}

	if len(parts) == 0 {
		return ""
	}
	return "Persona and style — the user's personal preferences for how you communicate. " +
		"Apply them in every reply and let them override the default tone and phrasing described above:\n- " +
		strings.Join(parts, "\n- ")
}
