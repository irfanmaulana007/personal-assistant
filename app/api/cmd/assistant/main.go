package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/agent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/api"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	calendarsvc "github.com/irfanmaulana007/personal-assistant/app/api/internal/calendar"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/activity"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/autotriage"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/bucketlist"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/calendar"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/contacts"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/email"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/event"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/hiking"
	imagegencap "github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/imagegen"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/knowledge"
	memorycap "github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/memory"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/reminder"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/selftune"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/travel"
	trellocap "github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/trello"
	websearchcap "github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/websearch"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/capability/wishlist"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/composio"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/composiotools"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/config"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/crypto"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/eval"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/groupproject"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/imagegen"
	googleint "github.com/irfanmaulana007/personal-assistant/app/api/internal/integration/google"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/mailer"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/mcp"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/mcptools"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/memory"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/observability"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/persona"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/routine"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/skills"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/translate"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/transport"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/transport/whatsapp"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/trello"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/websearch"
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

	// Wire error + performance monitoring (Sentry). No-op when no DSN is set, so
	// local development is unaffected. Flush buffered events on shutdown. The
	// release is taken from the APP_VERSION env var when the deployment sets it.
	flushSentry := observability.InitSentry(
		cfg.Sentry.DSN,
		cfg.SentryEnvironment(),
		os.Getenv("APP_VERSION"),
		cfg.Sentry.TracesSampleRate,
		cfg.Sentry.Debug,
		log,
	)
	defer flushSentry()

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
	handlers = append(handlers, wishlist.New(db, log))
	handlers = append(handlers, activity.New(db, timezone, log))
	handlers = append(handlers, travel.New(db, timezone, log))
	handlers = append(handlers, hiking.New(db, timezone, log))
	handlers = append(handlers, websearchcap.New(websearch.New(), settingsSvc, log))
	handlers = append(handlers, imagegencap.New(imagegen.NewClient(), settingsSvc, log))
	handlers = append(handlers, trellocap.New(trello.New(), db, settingsSvc, log))
	handlers = append(handlers, selftune.New(db, log))
	handlers = append(handlers, autotriage.New(db, trello.New(), settingsSvc, log))

	router := capability.NewRouter(log, handlers...)

	// Composio-backed tools for the user's connected apps (optional).
	composioTools := composiotools.New(composioClient, settingsSvc, log)

	// MCP-backed tools for the project's enabled MCP servers (Cloudflare, Railway,
	// Notion), each per-project in read-only or read & write mode.
	mcpTools := mcptools.New(mcp.NewClient(), settingsSvc, db, log)

	// LLM tool-calling agent (replaces the regex parser). Composio + MCP tools are
	// combined behind the single ToolProvider seam.
	toolProvider := agent.CombineProviders(composioTools, mcpTools)
	assistant := agent.New(llmClient, settingsSvc, skillsSvc, memSvc, personaSvc, router, cfg.Owner, toolProvider, log)

	// LLM-as-judge that scores the assistant's own replies inline (async, one
	// judgement per reply). Shared by the web and WhatsApp ingress paths.
	evalJudge := eval.NewJudge(llmClient, settingsSvc, db, log)

	// Group Translator skill: handles the `/t` command in WhatsApp groups
	// (translate between a group's two configured languages), short-circuiting
	// the agent for those messages. Each translation is logged to /logs (tagged
	// with the translator skill) and judged out of band, so it shares the store
	// and the LLM-as-judge above.
	groupTranslator := translate.NewGroup(translator, settingsSvc, db, db, evalJudge, log)

	// Group → project binding: lets a WhatsApp group's owner self-assign which
	// app project the assistant acts as in that group (one project per group). An
	// unbound group is inert until assigned. Runs before the agent, like the
	// translator.
	groupProjectSvc := groupproject.New(db, log)

	// Daily routines ("scheduled skills"): editable start-of-day / end-of-day
	// prompts run through the agent and delivered over WhatsApp. Supersedes the
	// old reminder digest — carry its configured time over on first boot.
	routineSvc := routine.New(settingsSvc, db, assistant, timezone, cfg.Owner.WhatsAppJID, log)
	routineSvc.MigrateFromDigest(ctx)
	// Routines are now enabled/scheduled per project; move any legacy global
	// routine settings onto the default project so a previously-enabled routine
	// keeps running there rather than silently switching off.
	routineSvc.MigrateToDefaultProject(ctx)

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
			// The transport invokes this on its own goroutine; a panic here would
			// otherwise take the process down without a trace. Report it to Sentry.
			defer observability.Recover()
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
			uctx, userID, assigned := resolveWhatsAppScope(ctx, db, owner, msg, log)

			// For a group, capture who actually sent the message (display name /
			// phone) so the incoming log and the run trace attribute the turn to the
			// real participant — not the generic owner the run is scoped to. Empty for
			// 1:1 chats, where the sender already is the scoped user.
			senderLabel := ""
			if msg.IsGroup {
				senderLabel = groupSenderLabel(msg)
			}

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

			// Group → project binding. Runs before the agent and handles owner-only
			// binding commands (assign / unassign / list). An unbound group now
			// defaults to the General project (scoped above), so ordinary chat
			// falls through to the agent; only explicit binding commands are
			// intercepted here. Like the translator, those config exchanges bypass
			// the agent and stay out of the conversation history.
			if msg.IsGroup {
				isOwner := isOwnerSender(uctx, settingsSvc, wa, msg)
				if reply, handled := groupProjectSvc.Handle(uctx, msg.Chat, msg.Text, owner.ID, assigned, isOwner); handled {
					replyTo := msg.Chat
					if replyTo == "" {
						replyTo = msg.From
					}
					if err := wa.SendMessage(ctx, replyTo, reply); err != nil {
						log.Error("failed to send group-project response", "to", replyTo, "error", err)
					}
					return
				}
			}

			// Recent conversation history for context (before logging this message).
			history := recentAgentHistory(uctx, db, userID, msg.Platform, 20)

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
			inSender := msg.From
			if senderLabel != "" {
				inSender = senderLabel
			}
			_ = db.LogMessage(uctx, &store.MessageLog{
				UserID:    userID,
				Platform:  msg.Platform,
				Direction: "in",
				Sender:    inSender,
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
					observability.CaptureError(err, map[string]string{"component": "agent", "platform": msg.Platform})
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
			_ = db.LogMessage(uctx, &store.MessageLog{
				UserID:    userID,
				Platform:  msg.Platform,
				Direction: "out",
				Sender:    "assistant",
				Body:      response,
				Intent:    "agent",
			})

			// Record the trace (dashboard + logs). Use the scoped context so the
			// trace is attributed to the WhatsApp run's project; otherwise it lands
			// under project 0 and is filtered out of the project-scoped Logs page.
			trace := &store.Trace{UserID: userID, Platform: msg.Platform, Sender: senderLabel, Input: msg.Text, LatencyMs: latencyMs}
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
					_ = db.LogToolUsage(uctx, userID, tool.Name, msg.Platform)
				}
				for _, st := range res.Steps {
					trace.Steps = append(trace.Steps, store.LLMCall{
						Step: st.Step, Model: st.Model, PromptTokens: st.PromptTokens,
						CompletionTokens: st.CompletionTokens, TotalTokens: st.TotalTokens,
						LatencyMs: st.LatencyMs, FinishReason: st.FinishReason, ToolCalls: st.ToolCalls,
					})
				}
			}
			traceID, _ := db.CreateTrace(uctx, trace)
			// Judge a sampled fraction of live replies out of band.
			evalJudge.JudgeInline(uctx, traceID)
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

		mailerSvc := mailer.New(mailer.Config{
			Host:     cfg.SMTP.Host,
			Port:     cfg.SMTP.Port,
			Username: cfg.SMTP.Username,
			Password: cfg.SMTP.Password,
			From:     cfg.SMTP.From,
			FromName: cfg.SMTP.FromName,
		})
		if mailerSvc.Enabled() {
			log.Info("email transport enabled", "host", cfg.SMTP.Host, "port", cfg.SMTP.Port)
		} else {
			log.Warn("SMTP not configured — password reset by email is disabled")
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
			mailerSvc,
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
// for an inbound WhatsApp message, and returns the context carrying that scope,
// the effective user id (used for history/logging), and whether the chat has an
// explicit project binding.
//
//   - An explicit whatsapp_mapping wins. Groups are keyed by the group JID;
//     personal chats are matched against every resolved identity of the sender
//     (phone form and LID→phone form) so a chat that arrives addressed by LID
//     still finds its phone-form mapping row. A group mapping never confers
//     superadmin; a personal mapping may, and attributes the named user. A found
//     mapping returns assigned=true and surfaces the project name to the agent.
//   - With no mapping, both groups and personal chats fall back to the General
//     default project (store.EnsureDefaultProject) and return assigned=false. The
//     agent then acts as the General assistant; for a group the owner can still
//     bind a specific project via the groupproject flow, which overrides General.
//     Routing to a real project — never the unscoped "project 0" — keeps traces
//     on the per-project dashboards and stops reads leaking across projects via
//     the "project 0 matches any row" predicate.
func resolveWhatsAppScope(ctx context.Context, db store.Store, owner *store.User, msg *transport.Message, log *slog.Logger) (context.Context, int64, bool) {
	userID := owner.ID

	// In a group, attribute the turn to the participant who actually sent it when
	// a personal mapping identifies them, so each sender's group mentions are
	// logged under their own user instead of all stacking under the owner. Only
	// the user id is taken here; the project and role still come from the group
	// mapping resolved below (a group never adopts a personal chat's project).
	if msg.IsGroup {
		if uid := senderUserID(ctx, db, log, msg.Candidates); uid != 0 {
			userID = uid
		}
	}

	// Resolve an explicit mapping. A group is keyed by its group JID; a personal
	// chat tries every known identity of the sender so a LID-addressed message
	// still matches its phone-form mapping row.
	var m *store.WhatsAppMapping
	if msg.IsGroup {
		m = lookupWhatsAppMapping(ctx, db, log, msg.Chat)
	} else {
		keys := msg.Candidates
		if len(keys) == 0 {
			keys = []string{msg.From}
		}
		for _, k := range keys {
			if hit := lookupWhatsAppMapping(ctx, db, log, k); hit != nil {
				m = hit
				break
			}
		}
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
		// Surface the bound project's name to the agent (group chats only) so it can
		// state which project it is acting as when asked.
		if msg.IsGroup {
			if p, _ := db.GetProject(ctx, m.ProjectID); p != nil {
				ctx = authctx.WithProjectName(ctx, p.Name)
			}
		}
		return ctx, userID, true
	}

	ctx = authctx.WithUserID(ctx, userID)

	// No explicit mapping → the General default project. Both unmapped groups and
	// unmapped personal chats act as the General assistant, scoped to a real
	// project (never the unscoped project 0). assigned=false so a group's owner
	// can still bind a specific project via chat (which overrides General).
	gen, err := db.EnsureDefaultProject(ctx, owner.ID)
	if err != nil || gen == nil {
		log.Error("resolve general default project", "error", err)
		return ctx, userID, false // fail closed: leave unscoped rather than pick a wrong project
	}
	ctx = authctx.WithProjectID(ctx, gen.ID)
	ctx = authctx.WithProjectRole(ctx, store.ProjectRoleAdmin)
	if msg.IsGroup {
		ctx = authctx.WithProjectName(ctx, gen.Name)
	}
	return ctx, userID, false
}

// lookupWhatsAppMapping fetches a mapping by JID, logging (but not failing on) a
// lookup error. Returns nil when the JID is empty or has no mapping.
func lookupWhatsAppMapping(ctx context.Context, db store.Store, log *slog.Logger, jid string) *store.WhatsAppMapping {
	if jid == "" {
		return nil
	}
	m, err := db.GetWhatsAppMapping(ctx, jid)
	if err != nil {
		log.Error("whatsapp mapping lookup", "error", err, "jid", jid)
	}
	return m
}

// senderUserID resolves a WhatsApp sender to the user id a personal mapping
// attributes them to, trying every one of the sender's identity candidates (so a
// LID-addressed sender still matches their phone-form mapping row). Returns 0
// when no candidate maps to a real user — the caller then keeps the owner id.
// Used to attribute a group participant's turn to their own user.
func senderUserID(ctx context.Context, db store.Store, log *slog.Logger, candidates []string) int64 {
	for _, k := range candidates {
		if hit := lookupWhatsAppMapping(ctx, db, log, k); hit != nil && hit.UserID != 0 {
			return hit.UserID
		}
	}
	return 0
}

// groupSenderLabel builds a human-readable identity for a WhatsApp group sender —
// "<display name> (+<phone>)" when both are known, falling back to whichever is
// available, and finally the raw sender JID. It labels who actually said a group
// message so the logs attribute it to the real participant rather than the
// generic owner the run is scoped to.
func groupSenderLabel(msg *transport.Message) string {
	name := strings.TrimSpace(msg.SenderName)
	phone := senderPhone(msg)
	switch {
	case name != "" && phone != "":
		return fmt.Sprintf("%s (%s)", name, phone)
	case name != "":
		return name
	case phone != "":
		return phone
	default:
		return msg.From
	}
}

// senderPhone returns the sender's phone number in +E.164 form when one of the
// identity candidates is a phone-form JID (…@s.whatsapp.net); "" otherwise (e.g.
// a LID-only sender whose phone the client hasn't resolved yet).
func senderPhone(msg *transport.Message) string {
	for _, c := range msg.Candidates {
		at := strings.IndexByte(c, '@')
		if at <= 0 {
			continue
		}
		if c[at+1:] == "s.whatsapp.net" {
			if user := c[:at]; user != "" {
				return "+" + user
			}
		}
	}
	return ""
}

// isOwnerSender reports whether a WhatsApp message came from the account owner —
// the only identity allowed to change a group's project binding. The owner is
// the paired account itself ("message yourself" mode) or any number on the
// settings allowlist (the owner's configured numbers). Matching spans the
// sender's identity candidates so it holds whether WhatsApp addressed them by
// phone number or by LID. It fails closed: an unrecognised sender is not treated
// as the owner.
func isOwnerSender(ctx context.Context, settingsSvc *settings.Service, wa *whatsapp.Transport, msg *transport.Message) bool {
	ids := msg.Candidates
	if len(ids) == 0 {
		ids = []string{msg.From}
	}
	if paired := wa.OwnerJID(); paired != "" {
		for _, c := range ids {
			if c == paired {
				return true
			}
		}
	}
	allow := settingsSvc.WhatsAppAllowedJIDs(ctx)
	if len(allow) == 0 {
		// "message yourself" mode: only the paired account (handled above) counts.
		return false
	}
	set := make(map[string]bool, len(allow))
	for _, j := range allow {
		set[j] = true
	}
	for _, c := range ids {
		if set[c] {
			return true
		}
	}
	return false
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
