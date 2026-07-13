package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/irfanmaulana007/personal-assistant/server/internal/transport"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver for whatsmeow's sqlstore
)

// Connection statuses reported to the UI.
const (
	StatusDisconnected = "disconnected"
	StatusPairing      = "pairing"
	StatusConnected    = "connected"
)

// Transport implements the transport.Transport interface for WhatsApp using
// whatsmeow (a linked-device connection to a personal WhatsApp account).
// Pairing is driven from the web UI: BeginPairing exposes a live QR code and
// the connection is established without blocking server startup.
type Transport struct {
	dsn       string
	log       *slog.Logger
	container *sqlstore.Container
	appCtx    context.Context

	mu       sync.RWMutex
	client   *whatsmeow.Client
	status   string
	qr       string
	ownerJID string
	allowed     map[string]bool // senders permitted to talk to the assistant
	allowAll    bool            // when true, answer every sender (allowed is ignored)
	handler     transport.MessageHandler
	groupBypass func(text string) bool // when set, an un-addressed group message whose text satisfies this is still delivered (e.g. "/t" translator commands)
}

// New creates a new WhatsApp transport. dsn is the PostgreSQL DSN whatsmeow
// uses to persist its session/device state (in its own whatsmeow_* tables,
// sharing the app's Postgres database). The owner JID is derived from the
// paired device, so it no longer needs to be configured.
func New(dsn string, log *slog.Logger) *Transport {
	return &Transport{dsn: dsn, log: log.With("transport", "whatsapp"), status: StatusDisconnected}
}

func (t *Transport) Name() string { return "whatsapp" }

// SetAllowedSenders restricts which sender JIDs the assistant responds to. When
// non-empty, only these numbers are answered (e.g. your personal + work
// numbers, while the assistant runs on a separate paired account). When empty,
// it falls back to the paired account's own messages (the "message yourself"
// mode).
func (t *Transport) SetAllowedSenders(jids []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(jids) == 0 {
		t.allowed = nil
		t.log.Info("whatsapp reply mode: paired account only (no OWNER_JID allowlist)")
		return
	}
	t.allowed = make(map[string]bool, len(jids))
	for _, j := range jids {
		t.allowed[j] = true
	}
	t.log.Info("whatsapp allowlist configured", "senders", jids)
}

// SetAllowAll toggles "answer every number" mode. When enabled the assistant
// responds to any sender and the allowlist is ignored; when disabled it falls
// back to the allowlist (or "message yourself" mode when that is empty).
func (t *Transport) SetAllowAll(allowAll bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.allowAll = allowAll
	if allowAll {
		t.log.Info("whatsapp reply mode: all numbers (allowlist ignored)")
	}
}

func (t *Transport) SetMessageHandler(handler transport.MessageHandler) {
	t.mu.Lock()
	t.handler = handler
	t.mu.Unlock()
}

// SetGroupBypass installs a predicate that lets certain un-addressed group
// messages through the "must @mention the assistant" gate. It exists so a
// self-contained "/t" translator command works in a group without mentioning
// the assistant, while ordinary prompts still require a mention.
func (t *Transport) SetGroupBypass(fn func(text string) bool) {
	t.mu.Lock()
	t.groupBypass = fn
	t.mu.Unlock()
}

// Init opens the whatsmeow store (backed by PostgreSQL via the pgx driver) and
// remembers the long-lived app context used for connections (which outlive the
// request that triggers them).
func (t *Transport) Init(ctx context.Context) error {
	t.appCtx = ctx
	container, err := sqlstore.New(ctx, "pgx", t.dsn, waLog.Noop)
	if err != nil {
		return fmt.Errorf("create whatsmeow store: %w", err)
	}
	t.container = container
	return nil
}

// Connect reconnects an existing paired session without blocking. It is a no-op
// (status stays disconnected) if no device has been paired yet.
func (t *Transport) Connect(ctx context.Context) error {
	device, err := t.container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}
	if device.ID == nil {
		t.setStatus(StatusDisconnected, "")
		return nil
	}
	client := whatsmeow.NewClient(device, waLog.Noop)
	client.AddEventHandler(t.eventHandler)
	t.mu.Lock()
	t.client = client
	t.mu.Unlock()
	if err := client.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	t.onConnected()
	return nil
}

// BeginPairing starts a QR pairing flow (if not already connected/pairing).
// The live QR code is exposed via Status().
func (t *Transport) BeginPairing() error {
	t.mu.RLock()
	st := t.status
	t.mu.RUnlock()
	if st == StatusConnected || st == StatusPairing {
		return nil
	}

	device, err := t.container.GetFirstDevice(t.appCtx)
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}
	if device.ID != nil {
		return t.Connect(t.appCtx)
	}

	client := whatsmeow.NewClient(device, waLog.Noop)
	client.AddEventHandler(t.eventHandler)
	qrChan, err := client.GetQRChannel(t.appCtx)
	if err != nil {
		return fmt.Errorf("qr channel: %w", err)
	}
	if err := client.Connect(); err != nil {
		return fmt.Errorf("connect for QR: %w", err)
	}
	t.mu.Lock()
	t.client = client
	t.status = StatusPairing
	t.mu.Unlock()

	go func() {
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				t.setStatus(StatusPairing, evt.Code)
			case "success", "login":
				t.onConnected()
			case "timeout":
				t.setStatus(StatusDisconnected, "")
				t.log.Warn("WhatsApp QR pairing timed out")
			}
		}
	}()
	return nil
}

// Logout unpairs the device and clears the session.
func (t *Transport) Logout(ctx context.Context) error {
	t.mu.RLock()
	client := t.client
	t.mu.RUnlock()
	if client != nil {
		_ = client.Logout(ctx)
		client.Disconnect()
	}
	t.mu.Lock()
	t.client = nil
	t.status = StatusDisconnected
	t.qr = ""
	t.ownerJID = ""
	t.mu.Unlock()
	return nil
}

// Status returns the current connection status and the pending QR code (if any).
func (t *Transport) Status() (status, qr string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status, t.qr
}

// OwnerJID returns the paired account's JID (empty until connected).
func (t *Transport) OwnerJID() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.ownerJID
}

func (t *Transport) Stop() error {
	t.mu.RLock()
	client := t.client
	t.mu.RUnlock()
	if client != nil {
		client.Disconnect()
	}
	return nil
}

func (t *Transport) SendMessage(ctx context.Context, to, text string) error {
	t.mu.RLock()
	client := t.client
	t.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("whatsapp not connected")
	}

	jid, err := types.ParseJID(to)
	if err != nil {
		return fmt.Errorf("parse JID %q: %w", to, err)
	}

	// The reply is authored in Markdown (for the web channel). Rewrite it to
	// WhatsApp's native markup so the formatting renders instead of leaking
	// literal "*", "#" and "-" characters into the chat.
	msg := &waE2E.Message{Conversation: proto.String(toWhatsAppMarkup(text))}
	if _, err := client.SendMessage(ctx, jid, msg); err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	return nil
}

// SendImage uploads image bytes and sends them as an image message, with an
// optional caption (rendered in WhatsApp markup like text replies).
func (t *Transport) SendImage(ctx context.Context, to string, data []byte, mimeType, caption string) error {
	t.mu.RLock()
	client := t.client
	t.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("whatsapp not connected")
	}

	jid, err := types.ParseJID(to)
	if err != nil {
		return fmt.Errorf("parse JID %q: %w", to, err)
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload image: %w", err)
	}
	if mimeType == "" {
		mimeType = "image/png"
	}
	img := &waE2E.ImageMessage{
		Mimetype:      proto.String(mimeType),
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    proto.Uint64(uploaded.FileLength),
	}
	if caption != "" {
		img.Caption = proto.String(toWhatsAppMarkup(caption))
	}
	if _, err := client.SendMessage(ctx, jid, &waE2E.Message{ImageMessage: img}); err != nil {
		return fmt.Errorf("send image: %w", err)
	}
	return nil
}

func (t *Transport) onConnected() {
	t.mu.Lock()
	if t.client != nil && t.client.Store.ID != nil {
		t.ownerJID = t.client.Store.ID.ToNonAD().String()
	}
	t.status = StatusConnected
	t.qr = ""
	jid := t.ownerJID
	t.mu.Unlock()
	t.log.Info("WhatsApp connected", "jid", jid)
}

func (t *Transport) setStatus(status, qr string) {
	t.mu.Lock()
	t.status = status
	t.qr = qr
	t.mu.Unlock()
}

func (t *Transport) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		t.handleMessage(v)
	case *events.Connected:
		t.log.Info("WhatsApp connected")
	case *events.Disconnected:
		t.log.Warn("WhatsApp disconnected")
	case *events.LoggedOut:
		t.log.Error("WhatsApp logged out — re-pair from the Integrations page")
		t.setStatus(StatusDisconnected, "")
	}
}

func (t *Transport) handleMessage(evt *events.Message) {
	// Accept plain text, extended text, and image messages (a photo may carry
	// a caption or come on its own). Anything else — stickers, audio, etc. — is
	// ignored below once we find neither text nor an image to act on.
	img := evt.Message.GetImageMessage()
	if evt.Message.GetConversation() == "" && evt.Message.GetExtendedTextMessage() == nil && img == nil {
		return
	}

	senderJID := evt.Info.Sender.ToNonAD().String()

	t.mu.RLock()
	ownerJID := t.ownerJID
	allowed := t.allowed
	allowAll := t.allowAll
	handler := t.handler
	client := t.client
	groupBypass := t.groupBypass
	t.mu.RUnlock()

	text := evt.Message.GetConversation()
	if text == "" && evt.Message.GetExtendedTextMessage() != nil {
		text = evt.Message.GetExtendedTextMessage().GetText()
	}

	// A self-contained "/t" translator command in a group is open to every
	// participant: it neither requires the assistant to be @mentioned nor the
	// sender to be in the allowlist, so a foreign friend in the group can use it
	// too. This bypass applies only to group "/t" commands — ordinary group
	// prompts and all 1:1 messages still go through the allowlist and mention
	// gates below.
	groupCmdBypass := evt.Info.IsGroup && groupBypass != nil && groupBypass(text)

	// WhatsApp may address the sender by LID (e.g. "…@lid") rather than their
	// phone number. Collect every known identity so an OWNER_JID allowlist of
	// phone JIDs still matches: the sender itself, the alternate address the
	// event carries, and the LID→phone mapping from the store.
	candidates := []string{senderJID}
	if alt := evt.Info.SenderAlt.ToNonAD(); alt.User != "" {
		candidates = append(candidates, alt.String())
	}
	if evt.Info.Sender.Server == types.HiddenUserServer && client != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(context.Background(), evt.Info.Sender.ToNonAD()); err == nil && pn.User != "" {
			candidates = append(candidates, pn.ToNonAD().String())
		}
	}

	// Decide who the assistant answers:
	//  - allow-all mode → every sender is permitted (allowlist ignored);
	//  - allowlist configured → any of the sender's identities must be listed;
	//  - otherwise → the paired account's own messages ("message yourself" mode).
	permitted := false
	switch {
	case allowAll:
		permitted = true
	case len(allowed) > 0:
		for _, c := range candidates {
			if allowed[c] {
				permitted = true
				break
			}
		}
	default:
		permitted = ownerJID == "" || senderJID == ownerJID
	}
	if !permitted && !groupCmdBypass {
		t.log.Info("ignoring WhatsApp message: sender not in OWNER_JID allowlist",
			"sender", senderJID, "candidates", candidates, "allowlist_size", len(allowed))
		return
	}

	// Group chats are noisy, so the assistant only speaks up when it is directly
	// addressed: either @mentioned, or someone replies to one of its own
	// messages. The one exception is a self-contained "/t" translator command,
	// which groupCmdBypass recognises and lets through without a mention. Any
	// other group chatter is ignored. 1:1 chats always pass.
	if evt.Info.IsGroup {
		ctxInfo := messageContextInfo(evt.Message)
		if !groupCmdBypass && !t.botAddressed(ctxInfo, client) {
			t.log.Info("ignoring WhatsApp group message: assistant not mentioned or replied to",
				"chat", evt.Info.Chat.String(), "sender", senderJID)
			return
		}
	}

	// For an image message, download the media and encode it as a data: URL so
	// the agent can pass it to a vision model (mirrors the web chat path). The
	// caption, if any, becomes the message text.
	imageDataURL := ""
	if img != nil {
		if text == "" {
			text = img.GetCaption()
		}
		if client != nil {
			data, err := client.Download(context.Background(), img)
			if err != nil {
				t.log.Error("failed to download WhatsApp image", "error", err)
			} else {
				mime := img.GetMimetype()
				if mime == "" {
					mime = "image/jpeg"
				}
				imageDataURL = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
			}
		}
	}

	msg := &transport.Message{
		ID:        evt.Info.ID,
		From:      senderJID,
		Chat:      evt.Info.Chat.ToNonAD().String(),
		IsGroup:   evt.Info.IsGroup,
		Text:      text,
		Image:     imageDataURL,
		Platform:  "whatsapp",
		Timestamp: evt.Info.Timestamp.Unix(),
		Raw:       evt,
	}

	if handler != nil {
		handler(msg)
	}
}

// messageContextInfo returns the ContextInfo carried by a text or image
// message, if any. ContextInfo holds mentions and quoted-message ("reply to")
// data used to decide whether the assistant was addressed in a group.
func messageContextInfo(m *waE2E.Message) *waE2E.ContextInfo {
	if ext := m.GetExtendedTextMessage(); ext != nil {
		return ext.GetContextInfo()
	}
	if img := m.GetImageMessage(); img != nil {
		return img.GetContextInfo()
	}
	return nil
}

// botAddressed reports whether a group message directly targets the assistant:
// either the paired account is @mentioned, or the message quotes (replies to) a
// message the assistant itself sent. Returns false when the assistant's own
// identity is unknown (client not paired).
func (t *Transport) botAddressed(ci *waE2E.ContextInfo, client *whatsmeow.Client) bool {
	if ci == nil || client == nil || client.Store == nil {
		return false
	}

	// Collect the assistant's own identities (phone JID and LID) as user parts,
	// so a mention/quote by either form is recognized.
	self := map[string]bool{}
	if client.Store.ID != nil {
		self[client.Store.ID.ToNonAD().User] = true
	}
	if lid := client.Store.LID; lid.User != "" {
		self[lid.ToNonAD().User] = true
	}
	return contextAddressesSelf(ci, self)
}

// contextAddressesSelf reports whether a message's ContextInfo targets one of
// the given identities (user parts), by @mention or by quoting one of their
// messages. Pure so it can be unit-tested without a paired client.
func contextAddressesSelf(ci *waE2E.ContextInfo, self map[string]bool) bool {
	if ci == nil || len(self) == 0 {
		return false
	}

	// @mention: any mentioned JID whose user part is the assistant's.
	for _, m := range ci.GetMentionedJID() {
		if jid, err := types.ParseJID(m); err == nil && self[jid.ToNonAD().User] {
			return true
		}
	}

	// Reply-to: the quoted message's author (Participant) is the assistant.
	if p := ci.GetParticipant(); p != "" {
		if jid, err := types.ParseJID(p); err == nil && self[jid.ToNonAD().User] {
			return true
		}
	}

	return false
}
