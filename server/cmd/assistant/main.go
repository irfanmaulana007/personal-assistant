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

	"github.com/irfanmaulana007/personal-assistant/server/internal/api"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/calendar"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/email"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/knowledge"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/reminder"
	"github.com/irfanmaulana007/personal-assistant/server/internal/config"
	googleint "github.com/irfanmaulana007/personal-assistant/server/internal/integration/google"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
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

	// Initialize Google auth
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

	// Initialize intent parser
	parser := intent.NewRegexParser()

	// Initialize integration clients
	timezone := cfg.Owner.Location()
	calendarClient := googleint.NewCalendarClient(googleAuth, timezone, log)
	gmailClient := googleint.NewGmailClient(googleAuth, log)

	// Initialize capability handlers
	calendarHandler := calendar.New(calendarClient, timezone, cfg.Capabilities.Calendar.DefaultDuration, cfg.Capabilities.Calendar.MaxResults)
	emailHandler := email.New(gmailClient)
	reminderHandler := reminder.New(db, timezone, cfg.Capabilities.Reminders.CheckIntervalDuration(), cfg.Owner.WhatsAppJID, log)
	knowledgeHandler := knowledge.New(db, cfg.Capabilities.Knowledge.MaxNoteLength)

	// Build capability router
	var handlers []capability.Handler
	if cfg.Capabilities.Calendar.Enabled {
		handlers = append(handlers, calendarHandler)
	}
	if cfg.Capabilities.Email.Enabled {
		handlers = append(handlers, emailHandler)
	}
	if cfg.Capabilities.Reminders.Enabled {
		handlers = append(handlers, reminderHandler)
	}
	if cfg.Capabilities.Knowledge.Enabled {
		handlers = append(handlers, knowledgeHandler)
	}

	router := capability.NewRouter(log, handlers...)

	// Initialize WhatsApp transport
	var wa *whatsapp.Transport
	if cfg.WhatsApp.Enabled {
		wa = whatsapp.New(cfg.WhatsApp.Database, cfg.Owner.WhatsAppJID, log)
		wa.SetMessageHandler(func(msg *transport.Message) {
			// Log incoming message
			_ = db.LogMessage(ctx, &store.MessageLog{
				Platform:  msg.Platform,
				Direction: "in",
				Sender:    msg.From,
				Body:      msg.Text,
			})

			// Parse intent
			result := parser.Parse(msg.Text)

			// Route to handler
			response := router.Route(ctx, result)

			// Send response
			if err := wa.SendMessage(ctx, msg.From, response); err != nil {
				log.Error("failed to send response",
					"to", msg.From,
					"error", err,
				)
				return
			}

			// Log outgoing message
			_ = db.LogMessage(ctx, &store.MessageLog{
				Platform:  msg.Platform,
				Direction: "out",
				Sender:    "assistant",
				Body:      response,
				Intent:    string(result.Capability),
				Action:    string(result.Action),
			})
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
			parser,
			router,
			db,
			cfg.Web.Password,
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
