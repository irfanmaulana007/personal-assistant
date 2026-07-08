package api

import (
	"encoding/base64"
	"net/http"

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
