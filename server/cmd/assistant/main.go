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
	calendarsvc "github.com/irfanmaulana007/personal-assistant/server/internal/calendar"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/activity"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/autotriage"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/bucketlist"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/calendar"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/contacts"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/email"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/event"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/hiking"
	imagegencap "github.com/irfanmaulana007/personal-assistant/server/internal/capability/imagegen"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/knowledge"
	memorycap "github.com/irfanmaulana007/personal-assistant/server/internal/capability/memory"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/reminder"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/selftune"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability/travel"
	trellocap "github.com/irfanmaulana007/personal-assistant/server/internal/capability/trello"
	websearchcap "github.com/irfanmaulana007/personal-assistant/server/internal/capability/websearch"
	"github.com/irfanmaulana007/personal-assistant/server/internal/composio"
	"github.com/irfanmaulana007/personal-assistant/server/internal/composiotools"
	"github.com/irfanmaulana007/personal-assistant/server/internal/config"
	"github.com/irfanmaulana007/personal-assistant/server/internal/crypto"
	"github.com/irfanmaulana007/personal-assistant/server/internal/eval"
	"github.com/irfanmaulana007/personal-assistant/server/internal/imagegen"
	googleint "github.com/irfanmaulana007/personal-assistant/server/internal/integration/google"
	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/memory"
	"github.com/irfanmaulana007/personal-assistant/server/internal/persona"
	"github.com/irfanmaulana007/personal-assistant/server/internal/routine"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/skills"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
	"github.com/irfanmaulana007/personal-assistant/server/internal/translate"
	"github.com/irfanmaulana007/personal-assistant/server/internal/transport"
	"github.com/irfanmaulana007/personal-assistant/server/internal/transport/whatsapp"
	"github.com/irfanmaulana007/personal-assistant/server/internal/trello"
	"github.com/irfanmaulana007/personal-assistant/server/internal/websearch"
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

	// Initialize the hybrid store (PostgreSQL for data, MongoDB for logs).
	db, err := store.Open(ctx, cfg.Database)
	if err != nil {
		log.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	log.Info("database initialized", "backend", "hybrid (postgres + mongo)")

	// Decode the encryption key once for reuse (Google tokens + settings).
	encKey, err := crypto.DecodeKey(cfg.Security.EncryptionKey)
	if err != nil {
		log.Error("invalid encryption key", "error", err)
		os.Exit(1)
	}

	// Runtime LLM settings (provider/API key/model/base URL) — stored in and
	// resolved from the database (the single source of truth).
	settingsSvc := settings.New(db, encKey)
	skillsSvc := skills.New(db)
	memSvc := memory.New(db)
	personaSvc := persona.New(db)
	llmClient := llm.NewClient()
	composioClient := composio.NewClient()

	// Normalize reminder/bucket-list text to English before persisting, whatever
	// language the user typed (REST or chat). Fail-soft: stores as-is on error.
	translator := translate.New(settingsSvc, llmClient, log)
	db.SetTranslator(translator)

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

	// Calendar service over the user's Composio-connected Google Calendar(s).
	// Shared by the one-time-event handler and the reminder recap worker.
	calSvc := calendarsvc.New(composioClient, settingsSvc, timezone, log)

	reminderHandler := reminder.New(db, settingsSvc, calSvc, timezone, cfg.Capabilities.Reminders.CheckIntervalDuration(), cfg.Owner.WhatsAppJID, log)
	if cfg.Capabilities.Reminders.Enabled {
		handlers = append(handlers, reminderHandler)
	}
	if cfg.Capabilities.Knowledge.Enabled {
		handlers = append(handlers, knowledge.New(db, cfg.Capabilities.Knowledge.MaxNoteLength))
	}

	// Long-term memory is always on (remember/recall).
	handlers = append(handlers, memorycap.New(memSvc, log))

	// One-time events → the user's Composio-connected Google Calendar, with a
	// one-time-reminder fallback. Always registered (composio is optional).
	handlers = append(handlers, event.New(calSvc, db, timezone, log))

	// Skill capabilities (gated per user via the skills framework; always
	// registered so the router can serve them when the skill is enabled).
	handlers = append(handlers, contacts.New(db, log))
	handlers = append(handlers, bucketlist.New(db, log))
	handlers = append(handlers, activity.New(db, timezone, log))
	handlers = append(handlers, travel.New(db, timezone, log))
	handlers = append(handlers, hiking.New(db, timezone, log))
	handlers = append(handlers, websearchcap.New(websearch.New(), settingsSvc, log))
	handlers = append(handlers, imagegencap.New(imagegen.NewClient(), settingsSvc, log))
	handlers = append(handlers, trellocap.New(trello.New(), settingsSvc, log))
	handlers = append(handlers, selftune.New(db, log))
	handlers = append(handlers, autotriage.New(db, trello.New(), settingsSvc, log))

	router := capability.NewRouter(log, handlers...)

	// Composio-backed tools for the user's connected apps (optional).
	composioTools := composiotools.New(composioClient, settingsSvc, log)

	// LLM tool-calling agent (replaces the regex parser).
	assistant := agent.New(llmClient, settingsSvc, skillsSvc, memSvc, personaSvc, router, cfg.Owner, composioTools, log)

	// LLM-as-judge that scores the assistant's own replies inline (async, one
	// judgement per reply). Shared by the web and WhatsApp ingress paths.
	evalJudge := eval.NewJudge(llmClient, settingsSvc, db, log)

	// Group Translator skill: handles the `/t` command in WhatsApp groups
	// (translate between a group's two configured languages), short-circuiting
	// the agent for those messages. Each translation is logged to /logs (tagged
	// with the translator skill) and judged out of band, so it shares the store
	// and the LLM-as-judge above.
	groupTranslator := translate.NewGroup(translator, settingsSvc, db, db, evalJudge, log)

	// Daily routines ("scheduled skills"): editable start-of-day / end-of-day
	// prompts run through the agent and delivered over WhatsApp. Supersedes the
	// old reminder digest — carry its configured time over on first boot.
	routineSvc := routine.New(settingsSvc, db, assistant, timezone, cfg.Owner.WhatsAppJID, log)
	routineSvc.MigrateFromDigest(ctx)

	// Initialize WhatsApp transport
	var wa *whatsapp.Transport
	if cfg.WhatsApp.Enabled {
		wa = whatsapp.New(cfg.Database.PostgresDSN, log)
		// The allowlist lives in settings (editable at Settings → WhatsApp). Seed
		// it from OWNER_JID on first boot so existing deployments keep working.
		allow := settingsSvc.WhatsAppAllowedJIDs(ctx)
		if len(allow) == 0 {
			if seed := cfg.Owner.AllowedJIDs(); len(seed) > 0 {
				if err := settingsSvc.SetWhatsAppAllowedJIDs(ctx, seed); err != nil {
					log.Error("seed whatsapp allowlist", "error", err)
				}
				allow = seed
			}
		}
		wa.SetAllowedSenders(allow)
		wa.SetAllowAll(settingsSvc.WhatsAppAllowAll(ctx))
		// A "/t" translator command works in a group without @mentioning the
		// assistant; ordinary prompts still require a mention. General prompts
		// are addressed by mentioning the assistant.
		wa.SetGroupBypass(translate.IsCommand)
		wa.SetMessageHandler(func(msg *transport.Message) {
			// WhatsApp acts as the owner (first admin). Its data is scoped to
			// that user; if setup hasn't happened yet, ask the user to set up.
			owner, err := db.FirstAdmin(ctx)
			if err != nil || owner == nil {
				_ = wa.SendMessage(ctx, msg.From, "The assistant isn't set up yet. Open the web app to create an admin account first.")
				return
			}
			// Resolve which project (and role) the agent acts as from where the
			// message came from: a group JID → its mapped project (role clamped, no
			// superadmin from a group); a personal number → its mapped project + role
			// (superadmin allowed for 1:1 only), else the owner's personal project.
			uctx, userID := resolveWhatsAppScope(ctx, db, owner, msg, log)

			// Group Translator skill: a "/t" command in a group is a
			// self-contained translate/config request. Handle it directly and
			// return — it bypasses the agent and is intentionally kept out of the
			// conversation history so it never disturbs the assistant's context.
			if msg.IsGroup {
				if reply, handled := groupTranslator.Handle(uctx, userID, msg.Chat, msg.Text); handled {
					replyTo := msg.Chat
					if replyTo == "" {
						replyTo = msg.From
					}
					if err := wa.SendMessage(ctx, replyTo, reply); err != nil {
						log.Error("failed to send translator response", "to", replyTo, "error", err)
					}
					return
				}
			}

			// Recent conversation history for context (before logging this message).
			history := recentAgentHistory(ctx, db, userID, msg.Platform, 20)

			// Log incoming message. Note when a photo is attached so image-only
			// messages don't show up as empty in the logs.
			logBody := msg.Text
			if msg.Image != "" {
				if logBody == "" {
					logBody = "[image]"
				} else {
					logBody += " [image]"
				}
			}
			_ = db.LogMessage(ctx, &store.MessageLog{
				UserID:    userID,
				Platform:  msg.Platform,
				Direction: "in",
				Sender:    msg.From,
				Body:      logBody,
			})

			// Run the LLM agent.
			start := time.Now()
			res, err := assistant.Run(uctx, msg.Text, history, msg.Image)
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

			// Reply into the chat the message came from. For a group this is the
			// group JID; for a 1:1 chat it equals the sender. Fall back to the
			// sender if the chat is somehow unset.
			replyTo := msg.Chat
			if replyTo == "" {
				replyTo = msg.From
			}

			// Send response. When the English Tutor skill is active its reply
			// begins with a [[grammar]]…[[/grammar]] correction block; on WhatsApp
			// that's rendered as a readable "English check" card (original struck
			// through, corrected version with changed words bolded). The logged
			// body below keeps the raw markers so the web chat renders its own
			// correction view; only the WhatsApp-bound text is reformatted here.
			if err := wa.SendMessage(ctx, replyTo, whatsapp.FormatGrammarReply(msg.Text, response)); err != nil {
				log.Error("failed to send response",
					"to", replyTo,
					"error", err,
				)
				return
			}

			// Deliver any images the agent produced (e.g. Image Generator skill).
			if res != nil {
				for _, img := range res.Images {
					if err := wa.SendImage(ctx, replyTo, img.Data, img.MimeType, ""); err != nil {
						log.Error("failed to send image", "to", replyTo, "error", err)
					}
				}
			}

			// Log outgoing message (chat history)
			_ = db.LogMessage(ctx, &store.MessageLog{
				UserID:    userID,
				Platform:  msg.Platform,
				Direction: "out",
				Sender:    "assistant",
				Body:      response,
				Intent:    "agent",
			})

			// Record the trace (dashboard + logs).
			trace := &store.Trace{UserID: userID, Platform: msg.Platform, Input: msg.Text, LatencyMs: latencyMs}
			if err != nil {
				trace.Status = "error"
				trace.Error = err.Error()
			} else if res != nil {
				trace.Output = res.Reply
				trace.Model = res.Model
				trace.PromptTokens = res.Usage.PromptTokens
				trace.CompletionTokens = res.Usage.CompletionTokens
				trace.TotalTokens = res.Usage.TotalTokens
				trace.ToolCount = len(res.Tools)
				trace.Skills = res.Skills
				for _, tool := range res.Tools {
					trace.Tools = append(trace.Tools, store.ToolInvocation{Name: tool.Name, Arguments: tool.Arguments, Result: tool.Result, LatencyMs: tool.LatencyMs})
					_ = db.LogToolUsage(ctx, userID, tool.Name, msg.Platform)
				}
				for _, st := range res.Steps {
					trace.Steps = append(trace.Steps, store.LLMCall{
						Step: st.Step, Model: st.Model, PromptTokens: st.PromptTokens,
						CompletionTokens: st.CompletionTokens, TotalTokens: st.TotalTokens,
						LatencyMs: st.LatencyMs, FinishReason: st.FinishReason, ToolCalls: st.ToolCalls,
					})
				}
			}
			traceID, _ := db.CreateTrace(ctx, trace)
			// Judge a sampled fraction of live replies out of band.
			evalJudge.JudgeInline(ctx, traceID)
		})

		// Proactive messages (reminders + daily routines) are delivered to the
		// paired WhatsApp account (derived from pairing), regardless of any stored
		// recipient.
		deliver := func(ctx context.Context, _ string, text string) error {
			// Deliver to the primary (first) allowlisted number. Fall back to the
			// paired account itself ("message yourself" mode) when none is set.
			to := ""
			if list := settingsSvc.WhatsAppAllowedJIDs(ctx); len(list) > 0 {
				to = list[0]
			}
			if to == "" {
				to = wa.OwnerJID()
			}
			if to == "" {
				return fmt.Errorf("whatsapp not connected")
			}
			return wa.SendMessage(ctx, to, text)
		}
		reminderHandler.SetSendFunc(deliver)
		routineSvc.SetSendFunc(deliver)

		if err := wa.Init(ctx); err != nil {
			log.Error("failed to initialize WhatsApp", "error", err)
			os.Exit(1)
		}
		// Reconnect an existing session in the background so startup never
		// blocks on WhatsApp. Pairing is driven from the UI.
		go func() {
			if err := wa.Connect(ctx); err != nil {
				log.Error("WhatsApp reconnect failed", "error", err)
			}
		}()
		defer wa.Stop()
		log.Info("WhatsApp transport ready")
	}

	// Start HTTP API server for web client
	if cfg.Web.Enabled {
		// Derive signing key from password for JWT
		signingKey := sha256.Sum256([]byte(cfg.Web.Password))

		var waCtl api.WhatsAppController
		if wa != nil {
			waCtl = wa
		}

		apiServer := api.NewServer(
			assistant,
			settingsSvc,
			llmClient,
			evalJudge,
			routineSvc,
			composioClient,
			calSvc,
			waCtl,
			db,
			signingKey[:],
			cfg.Web.StaticDir,
			cfg.Web.Port,
			cfg.Environment,
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

	// Start the daily routine scheduler (start-of-day / end-of-day briefings).
	go routineSvc.StartScheduler(ctx)

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

// recentAgentHistory loads the most recent conversation turns for a platform and
// maps them to agent messages (oldest first), for use as agent context.
// resolveWhatsAppScope decides which project, role, and user the agent acts as
// for an inbound WhatsApp message, and returns the context carrying that scope
// plus the effective user id (used for history/logging).
//
//   - Group message: look up the group JID. A mapping scopes the run to its
//     project; the role is clamped so a group can never confer superadmin.
//   - Personal (1:1) message: look up the sender's number. A mapping scopes the
//     run to its project and role (superadmin honoured here only) and, if it
//     names a user, attributes the chat to that user. Unmapped personal chats
//     fall back to the owner's personal project so the owner's 1:1 keeps working.
func resolveWhatsAppScope(ctx context.Context, db store.Store, owner *store.User, msg *transport.Message, log *slog.Logger) (context.Context, int64) {
	userID := owner.ID
	jid := msg.From
	if msg.IsGroup {
		jid = msg.Chat
	}

	m, err := db.GetWhatsAppMapping(ctx, jid)
	if err != nil {
		log.Error("whatsapp mapping lookup", "error", err, "jid", jid)
	}
	if m != nil {
		role := m.Role
		if msg.IsGroup && role == store.GlobalRoleSuperadmin {
			role = store.ProjectRoleAdmin // a group never confers superadmin
		}
		if !msg.IsGroup && m.UserID != 0 {
			userID = m.UserID
		}
		ctx = authctx.WithUserID(ctx, userID)
		ctx = authctx.WithProjectID(ctx, m.ProjectID)
		ctx = authctx.WithProjectRole(ctx, role)
		return ctx, userID
	}

	ctx = authctx.WithUserID(ctx, userID)
	// Unmapped personal chat: default to the owner's first/personal project so the
	// owner's private channel still works. Unmapped groups get no project scope.
	if !msg.IsGroup {
		if summaries, err := db.ListProjectsForUser(ctx, owner.ID); err == nil && len(summaries) > 0 {
			ctx = authctx.WithProjectID(ctx, summaries[0].ID)
			ctx = authctx.WithProjectRole(ctx, summaries[0].Role)
		}
	}
	return ctx, userID
}

func recentAgentHistory(ctx context.Context, db store.Store, userID int64, platform string, limit int) []agent.Message {
	logs, err := db.GetMessageHistory(ctx, userID, platform, limit)
	if err != nil {
		return nil
	}
	out := make([]agent.Message, 0, len(logs))
	for _, l := range logs {
		if l.Body == "" {
			continue
		}
		role := "assistant"
		if l.Direction == "in" {
			role = "user"
		}
		out = append(out, agent.Message{Role: role, Content: l.Body})
	}
	return out
}
