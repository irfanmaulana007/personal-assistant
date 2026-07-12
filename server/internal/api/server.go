package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/irfanmaulana007/personal-assistant/server/internal/agent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/composio"
	"github.com/irfanmaulana007/personal-assistant/server/internal/eval"
	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/pricing"
	"github.com/irfanmaulana007/personal-assistant/server/internal/routine"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// WhatsAppController controls the WhatsApp transport from the API (nil when the
// WhatsApp feature is disabled).
type WhatsAppController interface {
	Status() (status, qr string)
	BeginPairing() error
	Logout(ctx context.Context) error
	SetAllowedSenders(jids []string)
}

// Server is the HTTP API server for the web client.
type Server struct {
	agent      *agent.Agent
	settings   *settings.Service
	pricing    *pricing.Service
	llmClient  *llm.Client
	eval       *eval.Judge
	routines   *routine.Service
	composio   *composio.Client
	whatsapp   WhatsAppController
	store      store.Store
	signingKey []byte
	staticDir  string
	port       int
	// environment names the deployment (e.g. "local" / "production"); surfaced
	// on trace responses so the Logs run-detail copy shows which DB holds the run.
	environment string
	log         *slog.Logger
}

// NewServer creates a new API server. whatsapp may be nil.
func NewServer(
	agent *agent.Agent,
	settingsSvc *settings.Service,
	llmClient *llm.Client,
	judge *eval.Judge,
	routines *routine.Service,
	composioClient *composio.Client,
	whatsapp WhatsAppController,
	store store.Store,
	signingKey []byte,
	staticDir string,
	port int,
	environment string,
	log *slog.Logger,
) *Server {
	return &Server{
		agent:       agent,
		settings:    settingsSvc,
		pricing:     pricing.New(store),
		llmClient:   llmClient,
		eval:        judge,
		routines:    routines,
		composio:    composioClient,
		whatsapp:    whatsapp,
		store:       store,
		signingKey:  signingKey,
		staticDir:   staticDir,
		port:        port,
		environment: environment,
		log:         log.With("component", "api"),
	}
}

// Start starts the HTTP server. It blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// protect wraps a handler so it requires any authenticated user;
	// admin also requires the admin role.
	protect := func(h http.HandlerFunc) http.Handler { return s.authMiddleware(h) }
	admin := func(h http.HandlerFunc) http.Handler { return s.authMiddleware(s.requireAdmin(h)) }

	// Public routes
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/setup", s.handleSetup)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)

	// Authenticated (any role)
	mux.Handle("GET /api/auth/me", protect(s.handleMe))
	mux.Handle("PATCH /api/auth/me", protect(s.handleUpdateProfile))
	mux.Handle("GET /api/auth/me/stats", protect(s.handleMyStats))
	mux.Handle("POST /api/auth/password", protect(s.handleChangePassword))
	mux.Handle("/api/chat", protect(s.handleChat))
	mux.Handle("/api/chat/history", protect(s.handleChatHistory))
	mux.Handle("GET /api/skills", protect(s.handleListSkills))
	mux.Handle("PUT /api/skills/{id}", protect(s.handleSetSkill))
	mux.Handle("GET /api/preferences", protect(s.handleGetPreferences))
	mux.Handle("GET /api/persona", protect(s.handleGetPersona))
	mux.Handle("PUT /api/persona", protect(s.handleSetPersona))
	mux.Handle("GET /api/reminders", protect(s.handleListReminders))
	mux.Handle("POST /api/reminders", protect(s.handleCreateReminder))
	mux.Handle("GET /api/reminders/config", protect(s.handleGetRemindersConfig))
	mux.Handle("GET /api/routines", protect(s.handleListRoutines))
	mux.Handle("PUT /api/reminders/{id}", protect(s.handleUpdateReminder))
	mux.Handle("PUT /api/reminders/{id}/enabled", protect(s.handleSetReminderEnabled))
	mux.Handle("DELETE /api/reminders/{id}", protect(s.handleDeleteReminder))
	mux.Handle("GET /api/bucket-list", protect(s.handleListBucketItems))
	mux.Handle("POST /api/bucket-list", protect(s.handleCreateBucketItem))
	mux.Handle("PUT /api/bucket-list/{id}", protect(s.handleUpdateBucketItem))
	mux.Handle("PUT /api/bucket-list/{id}/done", protect(s.handleSetBucketItemDone))
	mux.Handle("PUT /api/bucket-list/{id}/resolution", protect(s.handleSetBucketItemResolution))
	mux.Handle("DELETE /api/bucket-list/{id}", protect(s.handleDeleteBucketItem))

	// Admin only
	mux.Handle("/api/settings", admin(s.handleSettings))
	mux.Handle("/api/settings/test", admin(s.handleSettingsTest))
	mux.Handle("PUT /api/preferences", admin(s.handleUpdatePreferences))
	mux.Handle("PUT /api/reminders/config", admin(s.handleSetRemindersConfig))
	mux.Handle("PUT /api/routines/{key}", admin(s.handleUpdateRoutine))
	mux.Handle("POST /api/routines/{key}/run", admin(s.handleRunRoutine))
	mux.Handle("GET /api/pricing", admin(s.handleListPricing))
	mux.Handle("PUT /api/pricing", admin(s.handleSetPricing))
	mux.Handle("DELETE /api/pricing/{model}", admin(s.handleDeletePricing))
	mux.Handle("/api/metrics/usage", admin(s.handleMetricsUsage))
	mux.Handle("GET /api/logs", admin(s.handleListLogs))
	mux.Handle("GET /api/logs/{id}", admin(s.handleGetLog))
	mux.Handle("GET /api/users", admin(s.handleListUsers))
	mux.Handle("POST /api/users", admin(s.handleCreateUser))
	mux.Handle("PATCH /api/users/{id}", admin(s.handleUpdateUser))
	mux.Handle("DELETE /api/users/{id}", admin(s.handleDeleteUser))
	mux.Handle("GET /api/integrations", admin(s.handleListIntegrations))
	mux.Handle("PUT /api/integrations/key", admin(s.handleSetComposioKey))
	mux.Handle("PUT /api/integrations/websearch-key", admin(s.handleSetWebSearchKey))
	mux.Handle("POST /api/integrations/{toolkit}/connect", admin(s.handleConnectIntegration))
	mux.Handle("DELETE /api/integrations/{toolkit}", admin(s.handleDisconnectIntegration))
	mux.Handle("GET /api/whatsapp", admin(s.handleWhatsAppStatus))
	mux.Handle("POST /api/whatsapp/connect", admin(s.handleWhatsAppConnect))
	mux.Handle("POST /api/whatsapp/disconnect", admin(s.handleWhatsAppDisconnect))
	mux.Handle("GET /api/whatsapp/allowlist", admin(s.handleGetWhatsAppAllowlist))
	mux.Handle("PUT /api/whatsapp/allowlist", admin(s.handleSetWhatsAppAllowlist))

	// Serve static files (SPA fallback)
	mux.Handle("/", s.spaHandler())

	// Apply global middleware
	handler := corsMiddleware(loggingMiddleware(s.log)(mux))

	addr := fmt.Sprintf(":%d", s.port)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		s.log.Info("shutting down HTTP server")
		server.Close()
	}()

	s.log.Info("HTTP server starting", "addr", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// spaHandler serves static files and falls back to index.html for SPA routing.
func (s *Server) spaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the requested file
		path := filepath.Join(s.staticDir, filepath.Clean(r.URL.Path))

		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			// Fall back to index.html for SPA routing
			http.ServeFile(w, r, filepath.Join(s.staticDir, "index.html"))
			return
		}

		http.ServeFile(w, r, path)
	})
}

// spaFS is for future use with embedded files.
type spaFS struct {
	fs fs.FS
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
