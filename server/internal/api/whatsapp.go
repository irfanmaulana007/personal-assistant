package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

type whatsappStatusResp struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"` // disconnected, pairing, connected
	QR      string `json:"qr"`     // data:image/png;base64,... when pairing
}

// qrDataURL renders a QR payload string as a PNG data URL for the browser.
func qrDataURL(code string) string {
	if code == "" {
		return ""
	}
	png, err := qrcode.Encode(code, qrcode.Medium, 320)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}

func (s *Server) writeWhatsAppStatus(w http.ResponseWriter) {
	if s.whatsapp == nil {
		writeJSON(w, http.StatusOK, whatsappStatusResp{Enabled: false, Status: "disabled"})
		return
	}
	status, qr := s.whatsapp.Status()
	writeJSON(w, http.StatusOK, whatsappStatusResp{Enabled: true, Status: status, QR: qrDataURL(qr)})
}

// handleWhatsAppStatus reports connection status and the current QR (if pairing).
func (s *Server) handleWhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	s.writeWhatsAppStatus(w)
}

// handleWhatsAppConnect starts pairing (shows a QR) or reconnects.
func (s *Server) handleWhatsAppConnect(w http.ResponseWriter, r *http.Request) {
	if s.whatsapp == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "WhatsApp is disabled"})
		return
	}
	if err := s.whatsapp.BeginPairing(); err != nil {
		s.log.Error("whatsapp begin pairing", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.writeWhatsAppStatus(w)
}

// handleWhatsAppDisconnect unpairs the device.
func (s *Server) handleWhatsAppDisconnect(w http.ResponseWriter, r *http.Request) {
	if s.whatsapp == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "WhatsApp is disabled"})
		return
	}
	if err := s.whatsapp.Logout(r.Context()); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.writeWhatsAppStatus(w)
}

type whatsappAllowlistResp struct {
	Allowlist []string `json:"allowlist"`
	// AllowAll, when true, makes the assistant answer every number and the
	// allowlist is ignored. Pointer on the request so an omitted field leaves
	// the stored value unchanged.
	AllowAll *bool `json:"allow_all,omitempty"`
}

// handleGetWhatsAppAllowlist returns the numbers allowed to chat with the assistant.
func (s *Server) handleGetWhatsAppAllowlist(w http.ResponseWriter, r *http.Request) {
	list := s.settings.WhatsAppAllowedJIDs(r.Context())
	if list == nil {
		list = []string{}
	}
	allowAll := s.settings.WhatsAppAllowAll(r.Context())
	writeJSON(w, http.StatusOK, whatsappAllowlistResp{Allowlist: list, AllowAll: &allowAll})
}

// handleSetWhatsAppAllowlist saves the allowlist and refreshes the live transport.
func (s *Server) handleSetWhatsAppAllowlist(w http.ResponseWriter, r *http.Request) {
	var req whatsappAllowlistResp
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	seen := map[string]bool{}
	list := make([]string, 0, len(req.Allowlist))
	for _, raw := range req.Allowlist {
		jid := normalizeWhatsAppJID(raw)
		if jid == "" || seen[jid] {
			continue
		}
		seen[jid] = true
		list = append(list, jid)
	}
	if err := s.settings.SetWhatsAppAllowedJIDs(r.Context(), list); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save"})
		return
	}
	allowAll := s.settings.WhatsAppAllowAll(r.Context())
	if req.AllowAll != nil {
		allowAll = *req.AllowAll
		if err := s.settings.SetWhatsAppAllowAll(r.Context(), allowAll); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save"})
			return
		}
	}
	if s.whatsapp != nil {
		s.whatsapp.SetAllowedSenders(list)
		s.whatsapp.SetAllowAll(allowAll)
	}
	writeJSON(w, http.StatusOK, whatsappAllowlistResp{Allowlist: list, AllowAll: &allowAll})
}

// normalizeWhatsAppJID accepts a full JID ("628…@s.whatsapp.net") or a bare
// number ("+62 851-2150-3971") and returns a canonical user JID, or "" if empty.
func normalizeWhatsAppJID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "@") {
		return s
	}
	var digits strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	if digits.Len() == 0 {
		return ""
	}
	return digits.String() + "@s.whatsapp.net"
}
