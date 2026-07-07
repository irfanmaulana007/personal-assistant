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

// Transport implements the transport.Transport interface for WhatsApp.
type Transport struct {
	client   *whatsmeow.Client
	dbPath   string
	ownerJID string
	handler  transport.MessageHandler
	log      *slog.Logger
	mu       sync.RWMutex
}

// New creates a new WhatsApp transport.
func New(dbPath, ownerJID string, log *slog.Logger) *Transport {
	return &Transport{
		dbPath:   dbPath,
		ownerJID: ownerJID,
		log:      log.With("transport", "whatsapp"),
	}
}

func (t *Transport) Name() string { return "whatsapp" }

func (t *Transport) SetMessageHandler(handler transport.MessageHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handler = handler
}

func (t *Transport) Start(ctx context.Context) error {
	dbLog := waLog.Noop

	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", t.dbPath), dbLog)
	if err != nil {
		return fmt.Errorf("create whatsmeow store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}

	t.client = whatsmeow.NewClient(deviceStore, dbLog)
	t.client.AddEventHandler(t.eventHandler)

	if t.client.Store.ID == nil {
		// Need to pair via QR code
		qrChan, _ := t.client.GetQRChannel(ctx)
		if err := t.client.Connect(); err != nil {
			return fmt.Errorf("connect for QR: %w", err)
		}
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				t.log.Info("Scan QR code to pair WhatsApp", "qr", evt.Code)
				fmt.Println("\n=== WhatsApp QR Code ===")
				fmt.Println("Scan this QR code with your WhatsApp app:")
				fmt.Println(evt.Code)
				fmt.Println("========================")
			case "login":
				t.log.Info("WhatsApp paired successfully")
			case "timeout":
				return fmt.Errorf("QR code scan timed out")
			}
		}
	} else {
		if err := t.client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		t.log.Info("WhatsApp connected", "jid", t.client.Store.ID.String())
	}

	return nil
}

func (t *Transport) Stop() error {
	if t.client != nil {
		t.client.Disconnect()
	}
	return nil
}

func (t *Transport) SendMessage(ctx context.Context, to, text string) error {
	jid, err := types.ParseJID(to)
	if err != nil {
		return fmt.Errorf("parse JID %q: %w", to, err)
	}

	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}

	_, err = t.client.SendMessage(ctx, jid, msg)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	return nil
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
		t.log.Error("WhatsApp logged out — need to re-pair")
	}
}

func (t *Transport) handleMessage(evt *events.Message) {
	// Only process text messages
	if evt.Message.GetConversation() == "" && evt.Message.GetExtendedTextMessage() == nil {
		return
	}

	senderJID := evt.Info.Sender.ToNonAD().String()

	// Owner verification: only respond to the configured owner
	if senderJID != t.ownerJID {
		t.log.Debug("ignoring message from non-owner", "sender", senderJID)
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

	t.mu.RLock()
	handler := t.handler
	t.mu.RUnlock()

	if handler != nil {
		handler(msg)
	}
}
