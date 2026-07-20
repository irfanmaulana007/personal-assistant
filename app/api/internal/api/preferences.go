package api

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// Display-preference setting keys and their defaults.
const (
	prefTimezoneKey = "pref_timezone"
	prefCurrencyKey = "pref_currency"
	prefUsdToIdrKey = "pref_usd_to_idr"

	defaultTimezone = "UTC"
	defaultCurrency = "USD"
	defaultUsdToIdr = 16000
)

type preferencesResp struct {
	Timezone string  `json:"timezone"`   // "UTC" or "Asia/Jakarta"
	Currency string  `json:"currency"`   // "USD" or "IDR"
	UsdToIdr float64 `json:"usd_to_idr"` // conversion rate for display
}

func (s *Server) readPref(r *http.Request, key, def string) string {
	if b, err := s.store.GetSetting(r.Context(), key); err == nil && len(b) > 0 {
		return string(b)
	}
	return def
}

// handleGetPreferences returns the app display preferences (any authed user).
func (s *Server) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	rate := float64(defaultUsdToIdr)
	if v, err := strconv.ParseFloat(s.readPref(r, prefUsdToIdrKey, ""), 64); err == nil && v > 0 {
		rate = v
	}
	writeJSON(w, http.StatusOK, preferencesResp{
		Timezone: s.readPref(r, prefTimezoneKey, defaultTimezone),
		Currency: s.readPref(r, prefCurrencyKey, defaultCurrency),
		UsdToIdr: rate,
	})
}

// handleUpdatePreferences sets the app display preferences (admin only).
func (s *Server) handleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	var req preferencesResp
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Timezone != "UTC" && req.Timezone != "Asia/Jakarta" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timezone must be UTC or Asia/Jakarta"})
		return
	}
	if req.Currency != "USD" && req.Currency != "IDR" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "currency must be USD or IDR"})
		return
	}
	if req.UsdToIdr <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "usd_to_idr must be positive"})
		return
	}

	ctx := r.Context()
	if err := s.store.SetSetting(ctx, prefTimezoneKey, []byte(req.Timezone)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save"})
		return
	}
	_ = s.store.SetSetting(ctx, prefCurrencyKey, []byte(req.Currency))
	_ = s.store.SetSetting(ctx, prefUsdToIdrKey, []byte(strconv.FormatFloat(req.UsdToIdr, 'f', -1, 64)))

	writeJSON(w, http.StatusOK, req)
}
