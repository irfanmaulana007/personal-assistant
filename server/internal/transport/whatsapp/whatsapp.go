package whatsapp

import (
	"context"
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

	_ "github.com/mattn/go-sqlite3"
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
	dbPath    string
	log       *slog.Logger
	container *sqlstore.Container
	appCtx    context.Context

	mu       sync.RWMutex
	client   *whatsmeow.Client
	status   string
	qr       string
	ownerJID string
	allowed  map[string]bool // senders permitted to talk to the assistant
	handler  transport.MessageHandler
}

// New creates a new WhatsApp transport. The owner JID is derived from the
// paired device, so it no longer needs to be configured.
func New(dbPath string, log *slog.Logger) *Transport {
	return &Transport{dbPath: dbPath, log: log.With("transport", "whatsapp"), status: StatusDisconnected}
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

func (t *Transport) SetMessageHandler(handler transport.MessageHandler) {
	t.mu.Lock()
	t.handler = handler
	t.mu.Unlock()
}

// Init opens the whatsmeow store and remembers the long-lived app context used
// for connections (which outlive the request that triggers them).
func (t *Transport) Init(ctx context.Context) error {
	t.appCtx = ctx
	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", t.dbPath), waLog.Noop)
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

	msg := &waE2E.Message{Conversation: proto.String(text)}
	if _, err := client.SendMessage(ctx, jid, msg); err != nil {
		return fmt.Errorf("send message: %w", err)
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
	if evt.Message.GetConversation() == "" && evt.Message.GetExtendedTextMessage() == nil {
		return
	}

	senderJID := evt.Info.Sender.ToNonAD().String()

	t.mu.RLock()
	ownerJID := t.ownerJID
	allowed := t.allowed
	handler := t.handler
	t.mu.RUnlock()

	// Decide who the assistant answers:
	//  - allowlist configured → only those numbers (assistant on its own account,
	//    you message it from your personal/work numbers);
	//  - otherwise → the paired account's own messages ("message yourself" mode).
	permitted := false
	if len(allowed) > 0 {
		permitted = allowed[senderJID]
	} else {
		permitted = ownerJID == "" || senderJID == ownerJID
	}
	if !permitted {
		// Logged at info so a mismatch is visible without debug logging — compare
		// this sender to your OWNER_JID (e.g. a "@lid" sender won't match a
		// "@s.whatsapp.net" allowlist entry).
		t.log.Info("ignoring WhatsApp message: sender not in OWNER_JID allowlist",
			"sender", senderJID, "allowlist_size", len(allowed), "paired", ownerJID)
		return
	}

	text := evt.Message.GetConversation()
	if text == "" && evt.Message.GetExtendedTextMessage() != nil {
		text = evt.Message.GetExtendedTextMessage().GetText()
	}

	msg := &transport.Message{
		ID:        evt.Info.ID,
		From:      senderJID,
		Text:      text,
		Platform:  "whatsapp",
		Timestamp: evt.Info.Timestamp.Unix(),
		Raw:       evt,
	}

	if handler != nil {
		handler(msg)
	}
}
