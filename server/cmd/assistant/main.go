package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/agent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/api"
	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/calendar"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/email"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/knowledge"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/reminder"
	"github.com/irfanmaulana007/personal-assistant/server/internal/composio"
	"github.com/irfanmaulana007/personal-assistant/server/internal/composiotools"
	"github.com/irfanmaulana007/personal-assistant/server/internal/config"
	"github.com/irfanmaulana007/personal-assistant/server/internal/crypto"
	googleint "github.com/irfanmaulana007/personal-assistant/server/internal/integration/google"
	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
	"github.com/irfanmaulana007/personal-assistant/server/internal/transport"
	"github.com/irfanmaulana007/personal-assistant/server/internal/transport/whatsapp"
)

func main() {
	configPath := flag.String("config", "server/config/config.yaml", "path to config file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Setup logger
	log := setupLogger(cfg.Logging)
	log.Info("starting personal assistant", "owner", cfg.Owner.Name)

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize store
	db, err := store.NewSQLite(cfg.Database.Path)
	if err != nil {
		log.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	log.Info("database initialized", "path", cfg.Database.Path)

	// Decode the encryption key once for reuse (Google tokens + settings).
	encKey, err := crypto.DecodeKey(cfg.Security.EncryptionKey)
	if err != nil {
		log.Error("invalid encryption key", "error", err)
		os.Exit(1)
	}

	// Runtime LLM settings (provider/API key/model/base URL) — stored in and
	// resolved from the database (the single source of truth).
	settingsSvc := settings.New(db, encKey)
	llmClient := llm.NewClient()
	composioClient := composio.NewClient()

	timezone := cfg.Owner.Location()

	// Build capability router
	var handlers []capability.Handler

	// Google auth is only required when a capability that depends on it
	// (calendar or email) is enabled. This lets the server run without
	// Google credentials for local/web-only development.
	if cfg.Capabilities.Calendar.Enabled || cfg.Capabilities.Email.Enabled {
		googleAuth, err := googleint.NewAuth(cfg.Google.CredentialsFile, db, cfg.Security.EncryptionKey, log)
		if err != nil {
			log.Error("failed to initialize Google auth", "error", err)
			os.Exit(1)
		}

		// Trigger initial Google authorization if needed
		if _, err := googleAuth.GetToken(ctx); err != nil {
			log.Error("Google authorization failed", "error", err)
			os.Exit(1)
		}
		log.Info("Google authorization ready")

		if cfg.Capabilities.Calendar.Enabled {
			calendarClient := googleint.NewCalendarClient(googleAuth, timezone, log)
			handlers = append(handlers, calendar.New(calendarClient, timezone, cfg.Capabilities.Calendar.DefaultDuration, cfg.Capabilities.Calendar.MaxResults))
		}
		if cfg.Capabilities.Email.Enabled {
			gmailClient := googleint.NewGmailClient(googleAuth, log)
			handlers = append(handlers, email.New(gmailClient))
		}
	}

	reminderHandler := reminder.New(db, timezone, cfg.Capabilities.Reminders.CheckIntervalDuration(), cfg.Owner.WhatsAppJID, log)
	if cfg.Capabilities.Reminders.Enabled {
		handlers = append(handlers, reminderHandler)
	}
	if cfg.Capabilities.Knowledge.Enabled {
		handlers = append(handlers, knowledge.New(db, cfg.Capabilities.Knowledge.MaxNoteLength))
	}

	router := capability.NewRouter(log, handlers...)

	// Composio-backed tools for the user's connected apps (optional).
	composioTools := composiotools.New(composioClient, settingsSvc, log)

	// LLM tool-calling agent (replaces the regex parser).
	assistant := agent.New(llmClient, settingsSvc, router, cfg.Owner, composioTools, log)

	// Initialize WhatsApp transport
	var wa *whatsapp.Transport
	if cfg.WhatsApp.Enabled {
		wa = whatsapp.New(cfg.WhatsApp.Database, cfg.Owner.WhatsAppJID, log)
		wa.SetMessageHandler(func(msg *transport.Message) {
			// WhatsApp acts as the owner (first admin). Its data is scoped to
			// that user; if setup hasn't happened yet, ask the user to set up.
			owner, err := db.FirstAdmin(ctx)
			if err != nil || owner == nil {
				_ = wa.SendMessage(ctx, msg.From, "The assistant isn't set up yet. Open the web app to create an admin account first.")
				return
			}
			userID := owner.ID
			uctx := authctx.WithUserID(ctx, userID)

			// Log incoming message
			_ = db.LogMessage(ctx, &store.MessageLog{
				UserID:    userID,
				Platform:  msg.Platform,
				Direction: "in",
				Sender:    msg.From,
				Body:      msg.Text,
			})

			// Run the LLM agent.
			start := time.Now()
			res, err := assistant.Run(uctx, msg.Text, nil)
			latencyMs := int(time.Since(start).Milliseconds())
			response := ""
			if err != nil {
				if err == agent.ErrNotConfigured {
					response = "The assistant isn't configured yet. Set the LLM API key in the web Settings page."
				} else {
					log.Error("agent run failed", "error", err)
					response = "Sorry, I ran into a problem. Please try again."
				}
			} else {
				response = res.Reply
			}

			// Send response
			if err := wa.SendMessage(ctx, msg.From, response); err != nil {
				log.Error("failed to send response",
					"to", msg.From,
					"error", err,
				)
				return
			}

			// Log outgoing message + usage
			_ = db.LogMessage(ctx, &store.MessageLog{
				UserID:    userID,
				Platform:  msg.Platform,
				Direction: "out",
				Sender:    "assistant",
				Body:      response,
				Intent:    "agent",
			})
			if res != nil {
				_ = db.LogUsage(ctx, &store.LLMUsage{
					UserID:           userID,
					Model:            res.Model,
					PromptTokens:     res.Usage.PromptTokens,
					CompletionTokens: res.Usage.CompletionTokens,
					TotalTokens:      res.Usage.TotalTokens,
					LatencyMs:        latencyMs,
					ToolCalls:        len(res.Tools),
					Platform:         msg.Platform,
				})
				for _, tool := range res.Tools {
					_ = db.LogToolUsage(ctx, userID, tool, msg.Platform)
				}
			}
		})

		// Set send function for reminders
		reminderHandler.SetSendFunc(wa.SendMessage)

		if err := wa.Start(ctx); err != nil {
			log.Error("failed to start WhatsApp", "error", err)
			os.Exit(1)
		}
		defer wa.Stop()
		log.Info("WhatsApp transport started")
	}

	// Start HTTP API server for web client
	if cfg.Web.Enabled {
		// Derive signing key from password for JWT
		signingKey := sha256.Sum256([]byte(cfg.Web.Password))

		apiServer := api.NewServer(
			assistant,
			settingsSvc,
			llmClient,
			composioClient,
			db,
			signingKey[:],
			cfg.Web.StaticDir,
			cfg.Web.Port,
			log,
		)

		go func() {
			if err := apiServer.Start(ctx); err != nil {
				log.Error("HTTP server failed", "error", err)
			}
		}()
		log.Info("web interface enabled", "port", cfg.Web.Port)
	}

	// Start reminder scheduler
	if cfg.Capabilities.Reminders.Enabled {
		go reminderHandler.StartScheduler(ctx)
		log.Info("reminder scheduler started")
	}

	log.Info("personal assistant is running — press Ctrl+C to stop")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("shutting down...")
	cancel()
}

func setupLogger(cfg config.LoggingConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
