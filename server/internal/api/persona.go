package api

import (
	"encoding/json"
	"net/http"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

type personaResp struct {
	Tone        string `json:"tone"`
	Emoji       string `json:"emoji"`
	Length      string `json:"length"`
	Personality string `json:"personality"`
	Name        string `json:"name"`
	Custom      string `json:"custom"`
}

var validPersona = map[string]map[string]bool{
	"tone":        {"formal": true, "balanced": true, "casual": true},
	"emoji":       {"none": true, "occasional": true, "frequent": true},
	"length":      {"concise": true, "balanced": true, "detailed": true},
	"personality": {"balanced": true, "professional": true, "friendly": true, "witty": true, "direct": true, "encouraging": true},
}

func pick(set map[string]bool, v, def string) string {
	if set[v] {
		return v
	}
	return def
}

// handleGetPersona returns the current user's persona.
func (s *Server) handleGetPersona(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	p, err := s.store.GetUserPersona(r.Context(), claims.UserID())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load persona"})
		return
	}
	writeJSON(w, http.StatusOK, personaResp{p.Tone, p.Emoji, p.Length, p.Personality, p.Name, p.Custom})
}

// handleSetPersona updates the current user's persona.
func (s *Server) handleSetPersona(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req personaResp
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	p := store.UserPersona{
		Tone:        pick(validPersona["tone"], req.Tone, "balanced"),
		Emoji:       pick(validPersona["emoji"], req.Emoji, "occasional"),
		Length:      pick(validPersona["length"], req.Length, "balanced"),
		Personality: pick(validPersona["personality"], req.Personality, "balanced"),
		Name:        req.Name,
		Custom:      req.Custom,
	}
	if err := s.store.SetUserPersona(r.Context(), claims.UserID(), p); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save persona"})
		return
	}
	writeJSON(w, http.StatusOK, personaResp{p.Tone, p.Emoji, p.Length, p.Personality, p.Name, p.Custom})
}
