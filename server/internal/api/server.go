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
	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Server is the HTTP API server for the web client.
type Server struct {
	agent      *agent.Agent
	settings   *settings.Service
	llmClient  *llm.Client
	store      store.Store
	password   string
	signingKey []byte
	staticDir  string
	port       int
	log        *slog.Logger
}

// NewServer creates a new API server.
func NewServer(
	agent *agent.Agent,
	settingsSvc *settings.Service,
	llmClient *llm.Client,
	store store.Store,
	password string,
	signingKey []byte,
	staticDir string,
	port int,
	log *slog.Logger,
) *Server {
	return &Server{
		agent:      agent,
		settings:   settingsSvc,
		llmClient:  llmClient,
		store:      store,
		password:   password,
		signingKey: signingKey,
		staticDir:  staticDir,
		port:       port,
		log:        log.With("component", "api"),
	}
}

// Start starts the HTTP server. It blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/health", s.handleHealth)

	// Protected routes
	protected := http.NewServeMux()
	protected.HandleFunc("/api/chat", s.handleChat)
	protected.HandleFunc("/api/chat/history", s.handleChatHistory)
	protected.HandleFunc("/api/settings", s.handleSettings)
	protected.HandleFunc("/api/settings/test", s.handleSettingsTest)
	protected.HandleFunc("/api/metrics/usage", s.handleMetricsUsage)
	for _, path := range []string{"/api/chat", "/api/chat/history", "/api/settings", "/api/settings/test", "/api/metrics/usage"} {
		mux.Handle(path, s.authMiddleware(protected))
	}

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
