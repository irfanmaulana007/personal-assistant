package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

// hikeResp is a logged hike as returned to the web UI: the stored hike joined
// with the names it references, so the client never has to resolve ids itself.
type hikeResp struct {
	ID           int64    `json:"id"`
	MountainID   int64    `json:"mountain_id"`
	Mountain     string   `json:"mountain"`
	Camped       bool     `json:"camped"`
	UpTrackID    int64    `json:"up_track_id"`
	UpTrack      string   `json:"up_track"`
	DownTrackID  int64    `json:"down_track_id"`
	DownTrack    string   `json:"down_track"`
	Days         int      `json:"days"`
	Nights       int      `json:"nights"`
	HikedOn      string   `json:"hiked_on"` // "YYYY-MM-DD"
	Participants []string `json:"participants"`
}

// hikeReq is the create/update payload. Mountain, trails, and participants are
// sent by name (not id): the server resolves each to an existing canonical
// record or creates one, mirroring how the chat flow avoids duplicate spellings.
type hikeReq struct {
	Mountain     string   `json:"mountain"`
	UpTrack      string   `json:"up_track"`
	DownTrack    string   `json:"down_track"`
	Camped       bool     `json:"camped"`
	Days         int      `json:"days"`
	Nights       int      `json:"nights"`
	HikedOn      string   `json:"hiked_on"` // RFC3339 or "YYYY-MM-DD"
	Participants []string `json:"participants"`
}

// nameResp is a canonical name option (mountain / hiker / trail) for the form's
// autocomplete lists.
type nameResp struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func toHikeResp(d store.HikeDetail) hikeResp {
	parts := d.Participants
	if parts == nil {
		parts = []string{}
	}
	return hikeResp{
		ID:           d.ID,
		MountainID:   d.MountainID,
		Mountain:     d.Mountain,
		Camped:       d.Camped,
		UpTrackID:    d.UpTrackID,
		UpTrack:      d.UpTrack,
		DownTrackID:  d.DownTrackID,
		DownTrack:    d.DownTrack,
		Days:         d.Days,
		Nights:       d.Nights,
		HikedOn:      hikedOnStr(d.HikedOn),
		Participants: parts,
	}
}

// hikedOnStr renders a hike date as "YYYY-MM-DD" for the UI, or "" when the
// hike has no recorded date (hiked_on is null).
func hikedOnStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// handleListHikes returns the current user's logged hikes for the active
// project, most recent first.
func (s *Server) handleListHikes(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	hikes, err := s.store.ListHikes(r.Context(), claims.UserID(), 500)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load hikes"})
		return
	}
	out := make([]hikeResp, 0, len(hikes))
	for _, h := range hikes {
		out = append(out, toHikeResp(h))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleHikeOptions returns the canonical mountains and participants a form can
// suggest, so a user reuses existing names instead of creating near-duplicates.
func (s *Server) handleHikeOptions(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	mountains, err := s.store.ListMountains(r.Context(), claims.UserID())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load mountains"})
		return
	}
	hikers, err := s.store.ListHikers(r.Context(), claims.UserID())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load participants"})
		return
	}
	mountainOpts := make([]nameResp, 0, len(mountains))
	for _, m := range mountains {
		mountainOpts = append(mountainOpts, nameResp{ID: m.ID, Name: m.Name})
	}
	hikerOpts := make([]nameResp, 0, len(hikers))
	for _, h := range hikers {
		hikerOpts = append(hikerOpts, nameResp{ID: h.ID, Name: h.Name})
	}
	writeJSON(w, http.StatusOK, map[string][]nameResp{
		"mountains": mountainOpts,
		"hikers":    hikerOpts,
	})
}

// handleListHikeTracks returns the known trails on a mountain (by id), for the
// up/down trail autocomplete once a known mountain is chosen.
func (s *Server) handleListHikeTracks(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	mountainID, err := strconv.ParseInt(r.URL.Query().Get("mountain_id"), 10, 64)
	if err != nil || mountainID <= 0 {
		writeJSON(w, http.StatusOK, []nameResp{})
		return
	}
	tracks, err := s.store.ListTracks(r.Context(), claims.UserID(), mountainID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load trails"})
		return
	}
	out := make([]nameResp, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, nameResp{ID: t.ID, Name: t.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateHike logs a new hike from the UI.
func (s *Server) handleCreateHike(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req hikeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	userID := claims.UserID()
	hike, badReq, err := s.buildHike(r, userID, req)
	if badReq != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": badReq})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create hike"})
		return
	}
	hikeID, err := s.store.CreateHike(r.Context(), userID, hike)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create hike"})
		return
	}
	if err := s.syncHikeParticipants(r, userID, hikeID, req.Participants); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save participants"})
		return
	}
	saved, err := s.store.GetHike(r.Context(), userID, hikeID)
	if err != nil || saved == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load created hike"})
		return
	}
	writeJSON(w, http.StatusOK, toHikeResp(*saved))
}

// handleUpdateHike edits an existing hike and re-syncs its participants.
func (s *Server) handleUpdateHike(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req hikeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	userID := claims.UserID()

	// The hike must already belong to this user + project before we edit it.
	existing, err := s.store.GetHike(r.Context(), userID, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load hike"})
		return
	}
	if existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "hike not found"})
		return
	}

	hike, badReq, err := s.buildHike(r, userID, req)
	if badReq != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": badReq})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update hike"})
		return
	}
	if err := s.store.UpdateHike(r.Context(), userID, id, hike); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update hike"})
		return
	}
	if err := s.store.ClearHikeParticipants(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update participants"})
		return
	}
	if err := s.syncHikeParticipants(r, userID, id, req.Participants); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save participants"})
		return
	}
	saved, err := s.store.GetHike(r.Context(), userID, id)
	if err != nil || saved == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load updated hike"})
		return
	}
	writeJSON(w, http.StatusOK, toHikeResp(*saved))
}

// handleDeleteHike removes a logged hike.
func (s *Server) handleDeleteHike(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteHike(r.Context(), claims.UserID(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete hike"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// buildHike validates a request and resolves its mountain/trails into a
// *store.Hike ready to persist. A non-empty second return is a client-facing
// validation message (HTTP 400); a non-nil error is an internal failure (500).
func (s *Server) buildHike(r *http.Request, userID int64, req hikeReq) (*store.Hike, string, error) {
	mountainName := strings.TrimSpace(req.Mountain)
	if mountainName == "" {
		return nil, "mountain is required", nil
	}
	// An empty date leaves the hike undated (nil) rather than defaulting to
	// today, so the UI can clear a date and the chat flow's dateless hikes round
	// -trip through an edit unchanged.
	var hikedOn *time.Time
	if raw := strings.TrimSpace(req.HikedOn); raw != "" {
		t, err := parseDoneAt(raw)
		if err != nil {
			return nil, "invalid hiked_on date", nil
		}
		u := t.UTC()
		hikedOn = &u
	}

	mountainID, _, err := s.resolveMountainID(r, userID, mountainName)
	if err != nil {
		return nil, "", err
	}
	upTrackID, err := s.resolveTrackID(r, userID, mountainID, req.UpTrack)
	if err != nil {
		return nil, "", err
	}
	downTrackID, err := s.resolveTrackID(r, userID, mountainID, req.DownTrack)
	if err != nil {
		return nil, "", err
	}

	return &store.Hike{
		MountainID:  mountainID,
		Camped:      req.Camped,
		UpTrackID:   upTrackID,
		DownTrackID: downTrackID,
		Days:        max0(req.Days),
		Nights:      max0(req.Nights),
		HikedOn:     hikedOn,
	}, "", nil
}

// syncHikeParticipants resolves each participant name to a canonical hiker and
// links it to the hike (deduping by name on the way).
func (s *Server) syncHikeParticipants(r *http.Request, userID, hikeID int64, names []string) error {
	seen := map[string]bool{}
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		hikerID, err := s.resolveHikerID(r, userID, name)
		if err != nil {
			return err
		}
		if err := s.store.AddHikeParticipant(r.Context(), hikeID, hikerID); err != nil {
			return err
		}
	}
	return nil
}

// resolveMountainID returns the id of the user's existing mountain matching name
// (case-insensitively) within the active project, creating one when none match.
func (s *Server) resolveMountainID(r *http.Request, userID int64, name string) (int64, string, error) {
	name = strings.TrimSpace(name)
	existing, err := s.store.ListMountains(r.Context(), userID)
	if err != nil {
		return 0, "", err
	}
	for _, m := range existing {
		if strings.EqualFold(m.Name, name) {
			return m.ID, m.Name, nil
		}
	}
	m, err := s.store.CreateMountain(r.Context(), userID, name)
	if err != nil {
		return 0, "", err
	}
	return m.ID, m.Name, nil
}

// resolveTrackID resolves a trail within a mountain; an empty name yields 0
// ("no trail recorded").
func (s *Server) resolveTrackID(r *http.Request, userID, mountainID int64, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, nil
	}
	existing, err := s.store.ListTracks(r.Context(), userID, mountainID)
	if err != nil {
		return 0, err
	}
	for _, t := range existing {
		if strings.EqualFold(t.Name, name) {
			return t.ID, nil
		}
	}
	t, err := s.store.CreateTrack(r.Context(), userID, mountainID, name)
	if err != nil {
		return 0, err
	}
	return t.ID, nil
}

// resolveHikerID resolves a participant name to a canonical hiker, creating one
// when none match.
func (s *Server) resolveHikerID(r *http.Request, userID int64, name string) (int64, error) {
	name = strings.TrimSpace(name)
	existing, err := s.store.ListHikers(r.Context(), userID)
	if err != nil {
		return 0, err
	}
	for _, h := range existing {
		if strings.EqualFold(h.Name, name) {
			return h.ID, nil
		}
	}
	h, err := s.store.CreateHiker(r.Context(), userID, name)
	if err != nil {
		return 0, err
	}
	return h.ID, nil
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
