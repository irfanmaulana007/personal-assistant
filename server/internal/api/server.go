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
	calendarsvc "github.com/irfanmaulana007/personal-assistant/server/internal/calendar"
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
	SetAllowAll(allowAll bool)
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
	calendar   *calendarsvc.Service
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
	calendarSvc *calendarsvc.Service,
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
		calendar:    calendarSvc,
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

	// Route guards:
	//   protect      — any authenticated user
	//   superadmin   — global superadmin only (platform-wide surfaces)
	//   project      — authenticated + an active project resolved from the
	//                  X-Project-Id header (member+); scopes domain data
	//   projectAdmin — project + the project admin role (or superadmin)
	protect := func(h http.HandlerFunc) http.Handler { return s.authMiddleware(h) }
	superadmin := func(h http.HandlerFunc) http.Handler { return s.authMiddleware(s.requireSuperadmin(h)) }
	project := func(h http.HandlerFunc) http.Handler { return s.authMiddleware(s.withProject(h)) }
	projectAdmin := func(h http.HandlerFunc) http.Handler {
		return s.authMiddleware(s.withProject(s.requireProjectAdmin(h)))
	}

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
	mux.Handle("GET /api/preferences", protect(s.handleGetPreferences))
	mux.Handle("GET /api/persona", protect(s.handleGetPersona))
	mux.Handle("PUT /api/persona", protect(s.handleSetPersona))
	mux.Handle("GET /api/routines", protect(s.handleListRoutines))

	// Projects & RBAC (project-level RBAC enforced inside each handler via
	// projectAccess on the {id} path param).
	mux.Handle("GET /api/projects", protect(s.handleListProjects))
	mux.Handle("POST /api/projects", superadmin(s.handleCreateProject))
	mux.Handle("GET /api/projects/{id}", protect(s.handleGetProject))
	mux.Handle("PATCH /api/projects/{id}", protect(s.handleUpdateProject))
	mux.Handle("DELETE /api/projects/{id}", protect(s.handleDeleteProject))
	mux.Handle("GET /api/projects/{id}/members", protect(s.handleListMembers))
	mux.Handle("POST /api/projects/{id}/members", protect(s.handleAddMember))
	mux.Handle("PATCH /api/projects/{id}/members/{userId}", protect(s.handleUpdateMember))
	mux.Handle("DELETE /api/projects/{id}/members/{userId}", protect(s.handleRemoveMember))
	mux.Handle("GET /api/projects/{id}/audit", protect(s.handleListAudit))

	// Active-project scoped: skills + features
	mux.Handle("GET /api/skills", project(s.handleListSkills))
	mux.Handle("PUT /api/skills/{id}", projectAdmin(s.handleSetSkill))
	mux.Handle("GET /api/features", project(s.handleListFeatures))
	mux.Handle("PUT /api/features/{id}", projectAdmin(s.handleSetFeature))

	// Active-project scoped: chat + domain data
	mux.Handle("/api/chat", project(s.handleChat))
	mux.Handle("/api/chat/history", project(s.handleChatHistory))
	mux.Handle("GET /api/reminders", project(s.handleListReminders))
	mux.Handle("POST /api/reminders", project(s.handleCreateReminder))
	mux.Handle("GET /api/reminders/config", protect(s.handleGetRemindersConfig))
	mux.Handle("PUT /api/reminders/{id}", project(s.handleUpdateReminder))
	mux.Handle("PUT /api/reminders/{id}/enabled", project(s.handleSetReminderEnabled))
	mux.Handle("DELETE /api/reminders/{id}", project(s.handleDeleteReminder))
	mux.Handle("GET /api/bucket-list", project(s.handleListBucketItems))
	mux.Handle("POST /api/bucket-list", project(s.handleCreateBucketItem))
	mux.Handle("PUT /api/bucket-list/{id}", project(s.handleUpdateBucketItem))
	mux.Handle("PUT /api/bucket-list/{id}/done", project(s.handleSetBucketItemDone))
	mux.Handle("PUT /api/bucket-list/{id}/resolution", project(s.handleSetBucketItemResolution))
	mux.Handle("DELETE /api/bucket-list/{id}", project(s.handleDeleteBucketItem))

	// Superadmin only (platform-wide surfaces)
	mux.Handle("/api/settings", superadmin(s.handleSettings))
	mux.Handle("/api/settings/test", superadmin(s.handleSettingsTest))
	mux.Handle("PUT /api/preferences", superadmin(s.handleUpdatePreferences))
	mux.Handle("PUT /api/reminders/config", superadmin(s.handleSetRemindersConfig))
	mux.Handle("POST /api/skills/{id}/reset-prompt", superadmin(s.handleResetSkillPrompt))
	mux.Handle("PUT /api/skills/{id}/prompt", superadmin(s.handleSetSkillPrompt))
	mux.Handle("PUT /api/routines/{key}", superadmin(s.handleUpdateRoutine))
	mux.Handle("POST /api/routines/{key}/run", superadmin(s.handleRunRoutine))
	mux.Handle("GET /api/pricing", superadmin(s.handleListPricing))
	mux.Handle("PUT /api/pricing", superadmin(s.handleSetPricing))
	mux.Handle("DELETE /api/pricing/{model}", superadmin(s.handleDeletePricing))
	mux.Handle("/api/metrics/usage", superadmin(s.handleMetricsUsage))
	mux.Handle("GET /api/admin/overview", superadmin(s.handleAdminOverview))
	mux.Handle("GET /api/logs", superadmin(s.handleListLogs))
	mux.Handle("GET /api/logs/{id}", superadmin(s.handleGetLog))
	mux.Handle("GET /api/users", superadmin(s.handleListUsers))
	mux.Handle("POST /api/users", superadmin(s.handleCreateUser))
	mux.Handle("PATCH /api/users/{id}", superadmin(s.handleUpdateUser))
	mux.Handle("DELETE /api/users/{id}", superadmin(s.handleDeleteUser))
	mux.Handle("GET /api/integrations", superadmin(s.handleListIntegrations))
	mux.Handle("PUT /api/integrations/key", superadmin(s.handleSetComposioKey))
	mux.Handle("PUT /api/integrations/websearch-key", superadmin(s.handleSetWebSearchKey))
	mux.Handle("PUT /api/integrations/openai-key", superadmin(s.handleSetOpenAIKey))
	mux.Handle("PUT /api/integrations/trello-creds", superadmin(s.handleSetTrelloCreds))
	mux.Handle("POST /api/integrations/{toolkit}/connect", superadmin(s.handleConnectIntegration))
	mux.Handle("DELETE /api/integrations/{toolkit}", superadmin(s.handleDisconnectIntegration))
	mux.Handle("DELETE /api/calendar/events", superadmin(s.handleClearCalendarEvents))
	mux.Handle("GET /api/whatsapp", superadmin(s.handleWhatsAppStatus))
	mux.Handle("POST /api/whatsapp/connect", superadmin(s.handleWhatsAppConnect))
	mux.Handle("POST /api/whatsapp/disconnect", superadmin(s.handleWhatsAppDisconnect))
	mux.Handle("GET /api/whatsapp/allowlist", superadmin(s.handleGetWhatsAppAllowlist))
	mux.Handle("PUT /api/whatsapp/allowlist", superadmin(s.handleSetWhatsAppAllowlist))
	mux.Handle("GET /api/whatsapp/mappings", superadmin(s.handleListWhatsAppMappings))
	mux.Handle("POST /api/whatsapp/mappings", superadmin(s.handleCreateWhatsAppMapping))
	mux.Handle("PATCH /api/whatsapp/mappings/{id}", superadmin(s.handleUpdateWhatsAppMapping))
	mux.Handle("DELETE /api/whatsapp/mappings/{id}", superadmin(s.handleDeleteWhatsAppMapping))

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
