package transport

import "context"

// Message represents a platform-agnostic incoming message.
type Message struct {
	ID string
	// From is the individual author of the message (used for allowlist checks
	// and logging). In a group this is the participant, not the group.
	From string
	// Candidates lists every known identity of the sender — the From JID plus any
	// alternate address the event carried and the LID→phone mapping — so an owner
	// or allowlist check matches whether WhatsApp addressed the sender by phone
	// number or by LID. Always includes From; may be nil on non-WhatsApp channels.
	Candidates []string
	// SenderName is the sender's human-readable display name as carried by the
	// platform (on WhatsApp, the message's PushName). Used to attribute a group
	// message to the real participant who sent it — "who said what" — rather than
	// stacking every group mention under one generic user. May be empty.
	SenderName string
	// Chat is the conversation the message belongs to and where a reply should
	// be sent. For a 1:1 chat it equals From; for a group it is the group JID.
	Chat string
	// IsGroup reports whether the message arrived in a group chat.
	IsGroup bool
	Text    string
	// Image, when non-empty, is a data: URL (base64) for an image attached to
	// the message. Requires a vision-capable model to be interpreted.
	Image     string
	Platform  string
	Timestamp int64
	Raw       any
}

// MessageHandler is called when a message is received.
type MessageHandler func(msg *Message)

// SendFunc sends a text reply to a recipient on a transport.
type SendFunc func(ctx context.Context, to, text string) error

// Transport defines the interface for messaging platforms.
type Transport interface {
	// Name returns the transport identifier (e.g., "whatsapp").
	Name() string

	// Start connects to the platform and begins listening for messages.
	Start(ctx context.Context) error

	// Stop gracefully disconnects from the platform.
	Stop() error

	// SetMessageHandler registers the callback for incoming messages.
	SetMessageHandler(handler MessageHandler)

	// SendMessage sends a text message to the given recipient.
	SendMessage(ctx context.Context, to, text string) error
}
